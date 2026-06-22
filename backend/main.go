package main

import (
	"bytes"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pquerna/otp/totp"
	"github.com/rs/cors"
	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"software.sslmate.com/src/go-pkcs12"
)

var db *sql.DB

// --- STRUKTUR DATA ---
type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignerInfo struct {
	Name            string    `json:"name"`
	Date            time.Time `json:"date"`
	IntegrityStatus string    `json:"integrity_status"`
}

type VerifyResponse struct {
	Message string       `json:"message"`
	IsValid bool         `json:"isValid"`
	Signers []SignerInfo `json:"signers"`
}

type BasicPayload struct {
	Email string `json:"email"`
}
type OTPVerifyPayload struct {
	Email   string `json:"email"`
	OTPCode string `json:"otpCode"`
}
type RequestIDPayload struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Passphrase string `json:"passphrase"`
}

type HistoryItem struct {
	ID           int    `json:"id"`
	Email        string `json:"email"`
	DocumentName string `json:"document_name"`
	SignerName   string `json:"signer_name"`
	SignedAt     string `json:"signed_at"`
	Status       string `json:"status"`
	FileToken    string `json:"file_token"`
}

type UpdateStatusPayload struct {
	RequestID int    `json:"request_id"`
	Field     string `json:"field"` // "is_approve", "is_ready", "is_sent", "is_revoke"
	Value     bool   `json:"value"`
}

type ApprovePayload struct {
	DocumentID int    `json:"document_id"`
	Status     string `json:"status"` // "disetujui" atau "ditolak"
}

func initDB() {
	var err error
	db, err = sql.Open("mysql", "root:@tcp(127.0.0.1:3306)/dgsign_db?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal("Gagal terhubung ke MySQL:", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			name VARCHAR(255),
			otp_secret VARCHAR(100),
			role VARCHAR(20) DEFAULT 'user',
			has_p12 BOOLEAN DEFAULT FALSE,
			p12_path VARCHAR(500),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Println("Gagal membuat tabel users:", err)
	}

	for _, stmt := range []string{
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR(255)",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS otp_secret VARCHAR(100)",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(20) DEFAULT 'user'",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS has_p12 BOOLEAN DEFAULT FALSE",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS p12_path VARCHAR(500)",
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS scan_count INT DEFAULT 0",
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS scan_limit INT DEFAULT 1",
	} {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("Gagal memastikan skema users (%s): %v", stmt, err)
		}
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS document_history (
			id INT AUTO_INCREMENT PRIMARY KEY,
			email VARCHAR(255) NOT NULL,
			document_name VARCHAR(255) NOT NULL,
			signer_name VARCHAR(255) NOT NULL,
			file_path VARCHAR(500) NOT NULL,
			signed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status VARCHAR(20) DEFAULT 'menunggu',
			file_token VARCHAR(255)
		)
	`)
	if err != nil {
		log.Println("Gagal membuat tabel document_history:", err)
	}

	for _, stmt := range []string{
		"ALTER TABLE document_history ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'menunggu'",
		"ALTER TABLE document_history ADD COLUMN IF NOT EXISTS file_token VARCHAR(255)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("Gagal memastikan skema document_history (%s): %v", stmt, err)
		}
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS signed_documents (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			uuid VARCHAR(128) NOT NULL UNIQUE,
			user_id BIGINT NOT NULL,
			filename VARCHAR(255) NOT NULL,
			file_path VARCHAR(500) NOT NULL,
			hash_sha256 VARCHAR(64) NOT NULL,
			certificate_serial VARCHAR(128),
			signer_name VARCHAR(255),
			signer_email VARCHAR(255),
			issuer_name VARCHAR(255),
			valid_until DATETIME,
			signed_at DATETIME NOT NULL,
			verification_token VARCHAR(128),
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		log.Println("Gagal membuat tabel signed_documents:", err)
	}

	for _, stmt := range []string{
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS signer_name VARCHAR(255)",
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS issuer_name VARCHAR(255)",
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS valid_until DATETIME",
		// Manajemen versi tanda tangan:
		// document_hash   -> identitas dokumen (SHA-256 PDF sumber). Dokumen yang
		//                    ditandatangani ulang akan menghasilkan document_hash yang
		//                    sama, sehingga versi lamanya dapat dikenali & dicabut.
		// signature_status-> "ACTIVE" (default) atau "REVOKED" bila telah digantikan
		//                    oleh penandatanganan yang lebih baru (signature revocation).
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS document_hash VARCHAR(64)",
		"ALTER TABLE signed_documents ADD COLUMN IF NOT EXISTS signature_status VARCHAR(20) DEFAULT 'ACTIVE'",
	} {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("Gagal memastikan skema signed_documents (%s): %v", stmt, err)
		}
	}

	// Index document_hash untuk mempercepat query revoke saat re-signing.
	// Idempoten: error (mis. index sudah ada) diabaikan agar aman dijalankan ulang.
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_signed_documents_document_hash ON signed_documents (document_hash)"); err != nil {
		log.Printf("Gagal membuat index document_hash (mungkin sudah ada): %v", err)
	}

	// --- TAMBAHAN UNTUK FITUR ALUR PENGAJUAN (WORKFLOW) ---
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS document_workflows (
			id INT AUTO_INCREMENT PRIMARY KEY,
			mahasiswa_email VARCHAR(255) NOT NULL,
			dosen_email VARCHAR(255) NOT NULL,
			document_name VARCHAR(255) NOT NULL,
			original_file_path VARCHAR(500) NOT NULL,
			signed_file_path VARCHAR(500),
			status VARCHAR(50) DEFAULT 'menunggu_dosen', -- menunggu_dosen, diterima_dosen, ditandatangani, selesai
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Println("Gagal membuat tabel document_workflows:", err)
	}
	// ------------------------------------------------------

	fmt.Println("Koneksi MySQL Berhasil!")
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func sanitizeString(value string) string {
	return strings.TrimSpace(value)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func drawTextOnImage(img *image.RGBA, x, y int, label string, col color.Color) {
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	drawer.DrawString(label)
}

func displayNameFromCertificate(cert *x509.Certificate, fallback string) string {
	if cert != nil && cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if cert != nil && len(cert.Subject.Organization) > 0 {
		return cert.Subject.Organization[0]
	}
	return fallback
}

func issuerNameFromCertificate(cert *x509.Certificate) string {
	if cert == nil {
		return "DGSign Certificate"
	}
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName
	}
	if len(cert.Issuer.Organization) > 0 {
		return cert.Issuer.Organization[0]
	}
	return "DGSign Certificate"
}

func createVisualSignatureImage(textLines []string, qrURL string, width int, height int) (string, error) {
	if width <= 0 {
		width = 280
	}
	if height <= 0 {
		height = 140
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

	qrSize := 90
	if width < 220 || height < 100 {
		qrSize = 60
	}

	// QR code (fallback to text-only block if QR generation fails)
	qrBounds := image.Rectangle{}
	if qr, err := qrcode.New(qrURL, qrcode.Medium); err == nil {
		qrImage := qr.Image(qrSize)
		qrBounds = qrImage.Bounds()
		qrX := 10
		qrY := (height - qrBounds.Dy()) / 2

		// [FIXED] Use draw.Draw instead of manually setting pixels
		qrRect := image.Rect(qrX, qrY, qrX+qrBounds.Dx(), qrY+qrBounds.Dy())
		draw.Draw(dst, qrRect, qrImage, image.Point{0, 0}, draw.Over)

	} else {
		log.Printf("QR generation failed for %s: %v", qrURL, err)
	}

	// Text panel
	x := 0
	if qrBounds.Dx() > 0 {
		x = qrBounds.Dx() + 24
	} else {
		x = 12
	}
	for i, line := range textLines {
		yPos := 22 + (i * 16)
		drawTextOnImage(dst, x, yPos, line, color.Black)
	}

	if err := os.MkdirAll("uploads", os.ModePerm); err != nil {
		return "", err
	}
	outPath := filepath.Join("uploads", fmt.Sprintf("visual_%d.png", time.Now().UnixNano()))
	file, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	if err := png.Encode(file, dst); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

// 1. Register
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var payload AuthPayload
	json.NewDecoder(r.Body).Decode(&payload)
	payload.Email = normalizeEmail(payload.Email)
	hashedPassword := hashPassword(payload.Password)
	_, err := db.Exec("INSERT INTO users (email, password) VALUES (?, ?)", payload.Email, hashedPassword)
	if err != nil {
		http.Error(w, "Email sudah terdaftar", http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "Registrasi berhasil, silakan login.")
}

// 2. Login (Sekarang mengecek apakah user butuh setup OTP)
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var payload AuthPayload
	json.NewDecoder(r.Body).Decode(&payload)
	payload.Email = normalizeEmail(payload.Email)

	hashedPassword := hashPassword(payload.Password)
	var otpSecret sql.NullString
	var role sql.NullString

	// 1. UBAH TIPE DATA MENJADI INT (Untuk mengakomodasi TINYINT MySQL)
	var hasP12Int sql.NullInt64

	err := db.QueryRow("SELECT otp_secret, role, has_p12 FROM users WHERE email = ? AND password = ?", payload.Email, hashedPassword).Scan(&otpSecret, &role, &hasP12Int)
	if err != nil {
		http.Error(w, "Email atau password salah", http.StatusUnauthorized)
		return
	}

	requireSetup := !otpSecret.Valid || otpSecret.String == ""

	userRole := "user"
	if role.Valid && role.String != "" {
		userRole = role.String
	}

	// 2. TERJEMAHKAN ANGKA 1 MENJADI TRUE, 0/NULL MENJADI FALSE
	isHasP12 := false
	if hasP12Int.Valid && hasP12Int.Int64 == 1 {
		isHasP12 = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Kredensial valid",
		"requireSetup": requireSetup,
		"role":         userRole,
		// 3. KIRIM NILAI BOOLEAN YANG SUDAH MATANG KE REACT
		"has_p12": isHasP12,
	})
}

// 3. Verifikasi OTP saat Login / Setup
func verifyOTPHandler(w http.ResponseWriter, r *http.Request) {
	var payload OTPVerifyPayload
	json.NewDecoder(r.Body).Decode(&payload)
	payload.Email = normalizeEmail(payload.Email)

	var secret sql.NullString
	err := db.QueryRow("SELECT otp_secret FROM users WHERE email = ?", payload.Email).Scan(&secret)

	if err != nil || !secret.Valid {
		http.Error(w, "Secret OTP tidak ditemukan", http.StatusNotFound)
		return
	}

	if !totp.Validate(payload.OTPCode, secret.String) {
		http.Error(w, "OTP Salah / Kedaluwarsa", http.StatusUnauthorized)
		return
	}
	fmt.Fprintf(w, "OTP Valid!")
}

// 4. Setup OTP & Generate QR
func generateOTPHandler(w http.ResponseWriter, r *http.Request) {
	var payload BasicPayload
	json.NewDecoder(r.Body).Decode(&payload)
	payload.Email = normalizeEmail(payload.Email)

	key, _ := totp.Generate(totp.GenerateOpts{Issuer: "Sistem-DGSign", AccountName: payload.Email})
	db.Exec("UPDATE users SET otp_secret = ? WHERE email = ?", key.Secret(), payload.Email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": key.URL()})
}

// 5. Reset OTP
func resetOTPHandler(w http.ResponseWriter, r *http.Request) {
	var payload BasicPayload
	json.NewDecoder(r.Body).Decode(&payload)
	payload.Email = normalizeEmail(payload.Email)
	db.Exec("UPDATE users SET otp_secret = NULL WHERE email = ?", payload.Email)
	fmt.Fprintf(w, "OTP direset.")
}

// 6. Request Digital ID (Dibatasi Hanya Bisa 1 Kali Set Passphrase)
func requestDigitalIDHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Data tidak valid", http.StatusBadRequest)
		return
	}
	payload.Email = normalizeEmail(payload.Email)

	// =================================================================
	// 1. CEK DULU DI DB: APAKAH DIA SUDAH PUNYA .P12?
	// =================================================================
	var hasP12 bool
	errCheck := db.QueryRow("SELECT has_p12 FROM users WHERE email = ?", payload.Email).Scan(&hasP12)
	if errCheck == nil && hasP12 {
		http.Error(w, "Anda sudah memiliki Digital ID aktif. Sistem hanya mengizinkan 1 identitas per akun.", http.StatusForbidden)
		return
	}
	// =================================================================

	// 2. Buat Kunci Privat RSA-2048
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		http.Error(w, "Gagal membuat kunci privat", http.StatusInternalServerError)
		return
	}

	// 3. Buat Nomor Seri Unik
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	// 4. TEMPLATE SERTIFIKAT END-ENTITY UNTUK PDF SIGNING
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   payload.Name,
			Organization: []string{"Muhammad Noval Rafief - Unila"},
			Locality:     []string{"Bandar Lampung"},
			Country:      []string{"ID"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(2, 0, 0), // Valid 2 Tahun

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning, x509.ExtKeyUsageEmailProtection},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// 5. Cetak Sertifikat Mentah (DER Bytes)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		http.Error(w, "Gagal mencetak sertifikat x509", http.StatusInternalServerError)
		return
	}

	// 6. Parsing derBytes menjadi objek *x509.Certificate
	cert, errParse := x509.ParseCertificate(derBytes)
	if errParse != nil {
		http.Error(w, "Gagal membaca struktur sertifikat", http.StatusInternalServerError)
		return
	}

	// 7. Bungkus menjadi .p12
	pfxBytes, err := pkcs12.Encode(rand.Reader, privateKey, cert, nil, payload.Passphrase)
	if err != nil {
		http.Error(w, "Gagal membungkus ke PKCS12", http.StatusInternalServerError)
		return
	}

	// Buat nama file aman berbasis hash email agar tidak mudah ditebak
	hashEmail := md5.Sum([]byte(payload.Email))
	safeFileName := fmt.Sprintf("%x.p12", hashEmail)
	keystorePath := filepath.Join("keystore", safeFileName)

	if err := os.MkdirAll("keystore", os.ModePerm); err != nil {
		http.Error(w, "Gagal menyiapkan folder penyimpanan sertifikat", http.StatusInternalServerError)
		return
	}

	// Tulis byte ke file lokal server
	errWrite := os.WriteFile(keystorePath, pfxBytes, 0600) // Akses strict (0600)
	if errWrite != nil {
		http.Error(w, "Gagal menyimpan sertifikat di brankas server", http.StatusInternalServerError)
		return
	}

	// =================================================================
	// 9. UPDATE DATABASE (HAS_P12 & PATH)
	// =================================================================
	_, errUpdate := db.Exec("UPDATE users SET has_p12 = TRUE, p12_path = ? WHERE email = ?", keystorePath, payload.Email)
	if errUpdate != nil {
		http.Error(w, "Gagal menyimpan status Digital ID ke database: "+errUpdate.Error(), http.StatusInternalServerError)
		return
	}
	// =================================================================

	// 9. Kirim file .p12 ke React
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Write(pfxBytes)
}

// =========================================================================
// FUNGSI BIKIN GAMBAR VISUAL (QR CODE + TEKS) OTOMATIS
// =========================================================================
func generateSignatureVisualBytes(textLines []string, qrContent string, width, height int) ([]byte, error) {
	qrSize := height
	if qrSize > width/3 {
		qrSize = width / 3
	}
	qrImg, err := qrcode.Encode(qrContent, qrcode.Medium, qrSize)
	if err != nil {
		return nil, err
	}
	qrImage, _, _ := image.Decode(bytes.NewReader(qrImg))

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)

	offset := image.Pt(10, (height-qrSize)/2)
	draw.Draw(img, qrImage.Bounds().Add(offset), qrImage, image.Point{}, draw.Over)

	textColor := color.RGBA{0, 0, 0, 255}
	textX := 10 + qrSize + 15
	textY := 20

	for _, line := range textLines {
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(textColor),
			Face: basicfont.Face7x13,
			Dot:  fixed.Point26_6{X: fixed.I(textX), Y: fixed.I(textY)},
		}
		d.DrawString(line)
		textY += 16
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// =========================================================================
// HANDLER TANDA TANGAN (webSignHandler) YANG SUDAH JADI
// =========================================================================
func webSignHandler(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		http.Error(w, "Koneksi tidak aman! Proses tanda tangan digagalkan karena SSL tidak aktif.", http.StatusForbidden)
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, "Ukuran file terlalu besar", http.StatusBadRequest)
		return
	}

	email := normalizeEmail(r.FormValue("email"))
	otpCode := sanitizeString(r.FormValue("otpCode"))
	passphrase := sanitizeString(r.FormValue("passphrase"))
	signerName := sanitizeString(r.FormValue("signerName")) // Dari React
	targetPage := sanitizeString(r.FormValue("page"))
	xCoord := sanitizeString(r.FormValue("x"))
	yCoord := sanitizeString(r.FormValue("y"))
	widthCoord := sanitizeString(r.FormValue("width"))
	heightCoord := sanitizeString(r.FormValue("height"))
	workflowID := r.FormValue("workflow_id")

	userSignatureImageFile, _, err := r.FormFile("signatureImage")
	var customImageBytes []byte
	if err == nil {
		defer userSignatureImageFile.Close()
		customImageBytes, _ = io.ReadAll(userSignatureImageFile)
	}

	if email == "" || passphrase == "" || otpCode == "" {
		http.Error(w, "Email, passphrase, dan OTP wajib diisi", http.StatusBadRequest)
		return
	}

	var userRole string
	var secret sql.NullString
	if err := db.QueryRow("SELECT otp_secret, role FROM users WHERE email = ?", email).Scan(&secret, &userRole); err != nil || !secret.Valid || secret.String == "" {
		http.Error(w, "Akun belum punya pengaturan OTP", http.StatusUnauthorized)
		return
	}

	if userRole == "mahasiswa" || userRole == "user" {
		http.Error(w, "Akses ditolak: Mahasiswa tidak diizinkan.", http.StatusForbidden)
		return
	}
	if !totp.Validate(otpCode, secret.String) {
		http.Error(w, "OTP Salah atau Kedaluwarsa", http.StatusUnauthorized)
		return
	}

	var p12Path sql.NullString
	if err := db.QueryRow("SELECT p12_path FROM users WHERE email = ? AND has_p12 = 1", email).Scan(&p12Path); err != nil || !p12Path.Valid {
		http.Error(w, "Digital ID belum dibuat", http.StatusBadRequest)
		return
	}

	p12Bytes, err := os.ReadFile(p12Path.String)
	if err != nil {
		http.Error(w, "File sertifikat tidak ditemukan di server", http.StatusInternalServerError)
		return
	}

	privateKey, cert, err := pkcs12.Decode(p12Bytes, passphrase)
	if err != nil {
		http.Error(w, "Passphrase salah atau sertifikat tidak valid", http.StatusUnauthorized)
		return
	}

	pdfFile, pdfHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Dokumen PDF wajib diunggah", http.StatusBadRequest)
		return
	}
	defer pdfFile.Close()

	os.MkdirAll("uploads", os.ModePerm)
	inputPdfPath := filepath.Join("uploads", fmt.Sprintf("%d_%s", time.Now().Unix(), pdfHeader.Filename))

	inputFile, err := os.Create(inputPdfPath)
	if err != nil {
		http.Error(w, "Gagal simpan file", http.StatusInternalServerError)
		return
	}
	io.Copy(inputFile, pdfFile)
	inputFile.Close()

	// document_hash = identitas dokumen berupa SHA-256 dari PDF sumber (sebelum
	// ditandatangani). Dipakai sebagai kunci pengelompokan versi tanda tangan:
	// dokumen yang ditandatangani ulang memiliki document_hash yang sama,
	// sehingga versi lamanya dapat dicabut (lihat signature revocation di bawah).
	// (Berbeda dari hash_sha256 yang merupakan hash *output* dan berubah setiap
	// penandatanganan, document_hash tetap stabil antar versi.)
	sourceHash := ""

	// Hitung hash file yang baru di-upload.
	if sourceBytes, err := os.ReadFile(inputPdfPath); err == nil {
		hashBytes := sha256.Sum256(sourceBytes)
		uploadHash := hex.EncodeToString(hashBytes[:])

		// Cek apakah file yang di-upload adalah output dari penandatanganan sebelumnya
		// (yaitu PDF yang sudah distamp). Jika ya, warisi document_hash dari
		// record sebelumnya agar chain revocation tetap terjaga lintas versi.
		var existingDocHash sql.NullString
		db.QueryRow(
			"SELECT document_hash FROM signed_documents WHERE hash_sha256 = ? AND document_hash IS NOT NULL LIMIT 1",
			uploadHash,
		).Scan(&existingDocHash)

		if existingDocHash.Valid && existingDocHash.String != "" {
			// File yang di-upload adalah PDF yang sudah ditandatangani sebelumnya.
			// Warisi document_hash agar versi sebelumnya bisa di-revoke.
			sourceHash = existingDocHash.String
		} else {
			// File sumber baru / belum pernah ditandatangani.
			// Hitung document_hash baru dari file ini.
			sourceHash = uploadHash
		}
	}

	inputPdf, _ := os.Open(inputPdfPath)
	defer inputPdf.Close()
	finfo, _ := inputPdf.Stat()
	pdfReader, _ := pdf.NewReader(inputPdf, finfo.Size())

	pageNo, _ := strconv.ParseUint(targetPage, 10, 32)
	if pageNo <= 0 {
		pageNo = 1
	}
	xPos, _ := strconv.ParseFloat(xCoord, 64)
	yPos, _ := strconv.ParseFloat(yCoord, 64)
	boxW, _ := strconv.ParseFloat(widthCoord, 64)
	boxH, _ := strconv.ParseFloat(heightCoord, 64)

	if yPos < 0 {
		yPos = 50
	}
	if xPos < 0 {
		xPos = 50
	}

	// Tentukan Informasi Sertifikat
	if signerName == "" {
		signerName = cert.Subject.CommonName
	}
	issuerName := cert.Issuer.CommonName
	if issuerName == "" {
		issuerName = "DGSign"
	}
	validUntil := cert.NotAfter.UTC().Format("2006-01-02 15:04:05")
	signedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	uuidRecord := fmt.Sprintf("%d-%x", time.Now().UnixNano(), email)

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	hostNameOnly := strings.Split(r.Host, ":")[0]

	// Gabungkan dengan port 3000
	verifyURL := fmt.Sprintf("%s://%s:3000/verify/%s", scheme, hostNameOnly, uuidRecord)

	// Generate Visual Signature (Custom PNG atau Otomatis QR)
	var visualBytes []byte
	if len(customImageBytes) > 0 {
		visualBytes = customImageBytes
	} else {
		textLines := []string{
			"Digitally Signed",
			fmt.Sprintf("Signer: %s", signerName),
			fmt.Sprintf("Email: %s", email),
			fmt.Sprintf("Date: %s", signedAt),
			fmt.Sprintf("Issuer: %s", issuerName),
		}
		visualBytes, _ = generateSignatureVisualBytes(textLines, verifyURL, int(boxW), int(boxH))
	}

	signedOutputPath := filepath.Join("uploads", fmt.Sprintf("signed_%d_%s", time.Now().UnixNano(), pdfHeader.Filename))
	signedOutput, _ := os.Create(signedOutputPath)

	signData := sign.SignData{
		Signature:   sign.SignDataSignature{CertType: sign.ApprovalSignature},
		Signer:      privateKey.(crypto.Signer),
		Certificate: cert,
		Appearance: sign.Appearance{
			Visible:    true,
			Page:       uint32(pageNo),
			LowerLeftX: xPos, LowerLeftY: yPos,
			UpperRightX: xPos + boxW, UpperRightY: yPos + boxH,
			Image:            visualBytes,
			ImageAsWatermark: false,
		},
	}

	if err := sign.Sign(inputPdf, signedOutput, pdfReader, finfo.Size(), signData); err != nil {
		http.Error(w, "Gagal tanda tangan", http.StatusInternalServerError)
		return
	}
	signedOutput.Close()

	// Update Database
	var userID int
	db.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&userID)

	fileHash := "hash_error"
	if signedBytes, err := os.ReadFile(signedOutputPath); err == nil {
		hash := sha256.Sum256(signedBytes)
		fileHash = hex.EncodeToString(hash[:])
	}

	certSerial := cert.SerialNumber.String()

	// Catat tanda tangan baru. Setiap penandatanganan menghasilkan Signature ID
	// (uuid) baru dengan status ACTIVE.
	db.Exec(`
		INSERT INTO signed_documents
		(uuid, user_id, filename, file_path, hash_sha256, certificate_serial, signer_email, signed_at, verification_token, created_at, updated_at, signer_name, issuer_name, valid_until, document_hash, signature_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ACTIVE')
	`, uuidRecord, userID, pdfHeader.Filename, signedOutputPath, fileHash, certSerial, email, signedAt, fileHash, signedAt, signedAt, signerName, issuerName, validUntil, sourceHash)

	// Signature revocation: ketika dokumen yang sama ditandatangani ulang,
	// semua tanda tangan sebelumnya untuk document_hash yang sama dicabut
	// (status -> REVOKED). Baris baru (uuidRecord) dikecualikan agar tetap ACTIVE.
	// Dengan demikian hanya satu versi tanda tangan yang berlaku (latest wins),
	// dan versi lama yang sudah tidak relevan otomatis dianggap tidak valid.
	db.Exec(`
		UPDATE signed_documents
		SET signature_status = 'REVOKED'
		WHERE document_hash = ? AND uuid <> ?
	`, sourceHash, uuidRecord)

	// Update Workflow (PENTING: pakai file_path agar kebaca di React)
	historyStatus := "menunggu"
	if workflowID != "" && workflowID != "0" && workflowID != "null" && workflowID != "undefined" {
		historyStatus = "ditandatangani"
		db.Exec("UPDATE document_workflows SET status = 'ditandatangani', file_path = ? WHERE id = ?", signedOutputPath, workflowID)
	}

	// Insert ke history
	db.Exec(`
		INSERT INTO document_history 
		(email, document_name, signer_name, file_path, signed_at, status, file_token) 
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, email, pdfHeader.Filename, signerName, signedOutputPath, signedAt, historyStatus, uuidRecord)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Dokumen berhasil ditandatangani!",
	})
}

// 8. Ambil Riwayat Dokumen
func historyHandler(w http.ResponseWriter, r *http.Request) {
	// Pastikan CORS dan Header aman
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	email := normalizeEmail(r.URL.Query().Get("email"))

	// Gunakan COALESCE agar jika ada data NULL, diubah jadi string kosong "" (mencegah error scan)
	query := `
		SELECT id, document_name, COALESCE(signer_name, ''), signed_at, COALESCE(file_token, ''), COALESCE(status, '') 
		FROM document_history 
		WHERE email = ? 
		ORDER BY id DESC
	`

	// ORDER BY id DESC lebih aman daripada signed_at DESC kalau format waktunya kadang beda
	rows, err := db.Query(query, email)
	if err != nil {
		fmt.Println("Error query history:", err)
		http.Error(w, "Gagal mengambil data", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var histories []HistoryItem = []HistoryItem{} // Inisialisasi array kosong, jangan biarkan nil
	for rows.Next() {
		var h HistoryItem
		err := rows.Scan(&h.ID, &h.DocumentName, &h.SignerName, &h.SignedAt, &h.FileToken, &h.Status)
		if err != nil {
			fmt.Println("Error scan baris history:", err) // Biar kelihatan di terminal kalau ada error
			continue
		}
		histories = append(histories, h)
	}

	json.NewEncoder(w).Encode(histories)
}

// 9. Download Dokumen dari Riwayat
func downloadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	// Ambil TOKEN, bukan ID lagi
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Akses ditolak. Token tidak valid.", http.StatusForbidden)
		return
	}

	var filePath, docName, status string
	// Cari berdasarkan file_token
	err := db.QueryRow("SELECT file_path, document_name, status FROM document_history WHERE file_token = ?", token).Scan(&filePath, &docName, &status)
	if err != nil {
		http.Error(w, "Dokumen tidak ditemukan di server", http.StatusNotFound)
		return
	}

	if strings.ToLower(strings.TrimSpace(status)) != "disetujui" {
		http.Error(w, "Dokumen belum disetujui", http.StatusForbidden)
		return
	}

	cleanPath := filepath.Clean(filePath)

	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		http.Error(w, "File fisik tidak ditemukan", http.StatusNotFound)
		return
	}

	// Sembunyikan path asli di browser, hanya kirim nama dokumen
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", docName))
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, cleanPath)
}

func downloadSignedDocumentHandler(w http.ResponseWriter, r *http.Request) {
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		http.Error(w, "UUID dokumen wajib diisi", http.StatusBadRequest)
		return
	}

	var fileName, filePath string
	err := db.QueryRow("SELECT filename, file_path FROM signed_documents WHERE uuid = ?", uuid).Scan(&fileName, &filePath)
	if err != nil {
		http.Error(w, "Dokumen tidak ditemukan", http.StatusNotFound)
		return
	}

	cleanPath := filepath.Clean(filePath)
	if _, err := os.Stat(cleanPath); err != nil {
		http.Error(w, "File hasil tanda tangan tidak ditemukan", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, cleanPath)
}

func verifySignedDocumentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// =========================================================
	// GEMBOK API (IMPLEMENTASI OPSI 1)
	// Pastikan ada email yang dikirim dari React.
	// Kalau kosong, berarti hacker nembak URL tanpa login.
	// =========================================================
	// Ambil requester
	requesterEmail := r.URL.Query().Get("requester")
	uuid := strings.TrimPrefix(r.URL.Path, "/verify/")

	// --- LOGIKA "MENIPU" PENGGUNA ---
	// Jika tidak ada email (belum login) ATAU UUID tidak valid,
	// JANGAN KASIH TAU ALASANNYA. Cukup bilang dokumen tidak ada.
	if requesterEmail == "" || uuid == "" || uuid == "/verify" {
		http.Error(w, `{"error": "Dokumen tidak ditemukan."}`, http.StatusNotFound)
		return
	}

	var record struct {
		UUID              string
		Filename          string
		FilePath          string
		HashSHA256        string
		CertificateSerial string
		SignerName        string
		SignerEmail       string
		IssuerName        string
		ValidUntil        time.Time
		SignedAt          time.Time
		SignatureStatus   string
		ScanCount         int
		ScanLimit         int
		UserID            int
		DocumentHash      string
	}

	selectColumns := `
		SELECT uuid, filename, file_path, hash_sha256, certificate_serial, signer_name, signer_email, issuer_name, valid_until, signed_at,
		       COALESCE(signature_status, 'ACTIVE'),
		       COALESCE(scan_count, 0),
		       COALESCE(scan_limit, 1),
		       user_id,
		       COALESCE(document_hash, '')
	`
	scanFields := func() []interface{} {
		return []interface{}{
			&record.UUID, &record.Filename, &record.FilePath, &record.HashSHA256, &record.CertificateSerial, &record.SignerName, &record.SignerEmail, &record.IssuerName, &record.ValidUntil, &record.SignedAt, &record.SignatureStatus, &record.ScanCount, &record.ScanLimit, &record.UserID, &record.DocumentHash,
		}
	}

	err := db.QueryRow(selectColumns+" FROM signed_documents WHERE uuid = ?", uuid).Scan(scanFields()...)

	if err != nil {
		fmt.Println("[ERROR VERIFY DB]:", err)
		http.Error(w, `{"error": "Dokumen tidak ditemukan."}`, http.StatusNotFound)
		return
	}

	// Tahap 1 — Signature revocation + scan-limit rotation (looping).
	// Jika signature yang dipindai (QR) sudah REVOKED, ikuti rantai ke record
	// ACTIVE terbaru dengan document_hash yang sama. Karena QR di PDF bersifat
	// statis (UUID lama), scan berikutnya harus tetap bisa "diteruskan" ke versi
	// terbaru agar logika scan-limit terus berputar (looping) setiap kali batas
	// tercapai.
	if strings.EqualFold(strings.TrimSpace(record.SignatureStatus), "REVOKED") {
		if record.DocumentHash == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"isValid": false,
				"status":  "REVOKED",
				"message": "Dokumen ini telah digantikan oleh versi tanda tangan yang lebih baru. Silakan gunakan dokumen terbaru.",
			})
			return
		}
		err = db.QueryRow(selectColumns+`
			FROM signed_documents
			WHERE document_hash = ? AND COALESCE(signature_status, 'ACTIVE') = 'ACTIVE'
			ORDER BY created_at DESC LIMIT 1
		`, record.DocumentHash).Scan(scanFields()...)

		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"isValid": false,
				"status":  "REVOKED",
				"message": "Dokumen ini telah digantikan oleh versi tanda tangan yang lebih baru. Silakan gunakan dokumen terbaru.",
			})
			return
		}
	}

	// Tahap 1b — Scan limit enforcement (inti dari fitur looping).
	// Setiap QR Code memiliki batas jumlah pemindaian (scan_limit). Jika batas
	// tercapai, signature ini di-revoke dan dibuatkan record baru dengan UUID
	// baru (menunjuk ke file & document_hash yang sama), sehingga QR ini tidak
	// lagi VALID. Karena record baru juga ACTIVE, scan QR lama selanjutnya akan
	// diteruskan ke record baru (Tahap 1) dan siklus berulang.
	scanLimit := record.ScanLimit
	if scanLimit <= 0 {
		scanLimit = 1 // default
	}

	if record.ScanCount >= scanLimit {
		// Revoke signature ini.
		db.Exec("UPDATE signed_documents SET signature_status = 'REVOKED' WHERE uuid = ?", record.UUID)

		// Generate record baru dengan UUID baru (file & data sama, document_hash diwarisi).
		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		newUUID := fmt.Sprintf("%d-%x", time.Now().UnixNano(), record.SignerEmail)
		newTokenBytes := sha256.Sum256([]byte(newUUID))
		newToken := hex.EncodeToString(newTokenBytes[:])
		db.Exec(`
			INSERT INTO signed_documents
			(uuid, user_id, filename, file_path, hash_sha256, certificate_serial, signer_name, signer_email, issuer_name, valid_until, signed_at, verification_token, created_at, updated_at, document_hash, signature_status, scan_count, scan_limit)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ACTIVE', 0, ?)
		`, newUUID, record.UserID, record.Filename, record.FilePath, record.HashSHA256, record.CertificateSerial, record.SignerName, record.SignerEmail, record.IssuerName, record.ValidUntil, now, newToken, now, now, record.DocumentHash, scanLimit)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"status":  "SCAN_LIMIT_REACHED",
			"message": "Batas verifikasi QR Code untuk dokumen ini telah tercapai. QR Code ini tidak lagi valid.",
		})
		return
	}

	// Increment scan count pada record ACTIVE yang sedang berlaku.
	db.Exec("UPDATE signed_documents SET scan_count = scan_count + 1 WHERE uuid = ?", record.UUID)

	// Tahap 2 — Document integrity verification (tamper detection).
	// Bandingkan hash SHA-256 file PDF yang tersimpan di server dengan hash yang
	// tercatat saat penandatanganan. Jika berbeda, berarti file fisik telah
	// dimodifikasi setelah ditandatangani -> dokumen tidak valid.
	cleanPath := filepath.Clean(record.FilePath)
	isValid := false
	if data, err := os.ReadFile(cleanPath); err == nil {
		hash := sha256.Sum256(data)
		isValid = hex.EncodeToString(hash[:]) == record.HashSHA256
	}

	// Kirim data ke React
	json.NewEncoder(w).Encode(map[string]interface{}{
		"isValid":           isValid,
		"status":            "ACTIVE",
		"signerName":        record.SignerName,
		"signerEmail":       record.SignerEmail,
		"issuerName":        record.IssuerName,
		"certificateSerial": record.CertificateSerial,
		"signedAt":          record.SignedAt.Format("02 Jan 2006 15:04:05"),
		"validUntil":        record.ValidUntil.Format("02 Jan 2006 15:04:05"),
		"filename":          record.Filename,
	})
}

func verifyDocumentHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Baca File dari Upload
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Gagal membaca form", http.StatusBadRequest)
		return
	}

	pdfFile, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File PDF tidak ditemukan", http.StatusBadRequest)
		return
	}
	defer pdfFile.Close()

	// 2. HITUNG HASH FILE YANG DIUPLOAD (Ini kuncinya!)
	hasher := sha256.New()
	if _, err := io.Copy(hasher, pdfFile); err != nil {
		http.Error(w, "Gagal memproses file", http.StatusInternalServerError)
		return
	}
	uploadedHash := hex.EncodeToString(hasher.Sum(nil))

	// 3. CARI DI DB BERDASARKAN HASH (Bukan Nama File)
	var record struct {
		UUID            string
		SignerName      string
		SignerEmail     string
		SignedAt        time.Time
		ValidUntil      time.Time
		IssuerName      string
		SignatureStatus string
	}

	// Cari apakah hash ini ada di database kita
	err = db.QueryRow(`
        SELECT uuid, signer_name, signer_email, signed_at, valid_until, issuer_name, COALESCE(signature_status, 'ACTIVE') 
        FROM signed_documents 
        WHERE hash_sha256 = ?`, uploadedHash).Scan(
		&record.UUID, &record.SignerName, &record.SignerEmail, &record.SignedAt, &record.ValidUntil, &record.IssuerName, &record.SignatureStatus,
	)

	// 4. JIKA TIDAK DITEMUKAN BERARTI DOKUMEN PALSU/DIMODIFIKASI
	if err != nil {
		// Kalau hash-nya beda, berarti isinya sudah diubah atau bukan dokumen asli
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "Dokumen tidak valid atau telah dimodifikasi.",
		})
		return
	}

	// --- FITUR VERSION-BASED SIGNATURE VALIDATION ---
	// Jika hash output PDF cocok tapi status signature REVOKED, berarti ini versi
	// tanda tangan lama yang sudah digantikan. Konsisten dengan endpoint QR.
	if strings.EqualFold(strings.TrimSpace(record.SignatureStatus), "REVOKED") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"status":  "REVOKED",
			"message": "Dokumen ini telah digantikan oleh versi tanda tangan yang lebih baru. Silakan gunakan dokumen terbaru.",
		})
		return
	}

	// 5. JIKA KETEMU, BERARTI ASLI
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"isValid": true,
		"status":  "ACTIVE",
		"message": "Dokumen Asli & Tanda Tangan Valid",
		"certificate": map[string]string{
			"signerName":  record.SignerName,
			"signerEmail": record.SignerEmail,
			"signedAt":    record.SignedAt.Format("02 Jan 2006 15:04:05"),
			"validUntil":  record.ValidUntil.Format("02 Jan 2006 15:04:05"),
			"issuerName":  record.IssuerName,
		},
	})
}

// verifyFileHandler: pengecekan integritas file lebih lanjut dari halaman verifikasi QR.
// User upload PDF fisik. Sistem membandingkan hash file dengan hash record UUID dari
// URL QR. HANYA PDF yang cocok dengan record UUID itu (bukan versi/chain lain) yang
// dianggap ASLI. Ini mendeteksi: PDF dipalsukan, ditukar, atau QR dipindah dari dokumen lain.
func verifyFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "Metode tidak diizinkan.",
		})
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "Gagal membaca file. Ukuran mungkin melebihi batas.",
		})
		return
	}

	// UUID dari URL QR (halaman verifikasi tempat user berada).
	uuid := sanitizeString(r.FormValue("uuid"))
	if uuid == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "QR Code tidak teridentifikasi.",
		})
		return
	}

	pdfFile, _, err := r.FormFile("file")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "File PDF tidak ditemukan pada permintaan.",
		})
		return
	}
	defer pdfFile.Close()

	// Hitung hash PDF yang di-upload.
	hasher := sha256.New()
	if _, err := io.Copy(hasher, pdfFile); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "Gagal memproses file.",
		})
		return
	}
	uploadedHash := hex.EncodeToString(hasher.Sum(nil))

	// Ambil record spesifik untuk UUID ini.
	var record struct {
		HashSHA256        string
		SignatureStatus   string
		SignerName        string
		SignerEmail       string
		IssuerName        string
		CertificateSerial string
		Filename          string
		SignedAt          time.Time
		ValidUntil        time.Time
	}
	err = db.QueryRow(`
		SELECT hash_sha256, COALESCE(signature_status, 'ACTIVE'), signer_name, signer_email, issuer_name, certificate_serial, filename, signed_at, valid_until
		FROM signed_documents WHERE uuid = ?
	`, uuid).Scan(&record.HashSHA256, &record.SignatureStatus, &record.SignerName, &record.SignerEmail, &record.IssuerName, &record.CertificateSerial, &record.Filename, &record.SignedAt, &record.ValidUntil)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "QR Code tidak terdaftar di sistem.",
		})
		return
	}

	// FILTER UTAMA: hash PDF upload harus SAMA PERSIS dengan hash record UUID ini.
	// Bukan chain/versi lain — hanya dokumen asli untuk UUID ini yang valid.
	if uploadedHash != record.HashSHA256 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid": false,
			"message": "Dokumen mungkin telah dipalsukan, ditukar, atau QR dipindahkan dari dokumen lain.",
		})
		return
	}

	// Hash cocok. Sampaikan status versi (REVOKED tetap disebut, tapi asli).
	if strings.EqualFold(strings.TrimSpace(record.SignatureStatus), "REVOKED") {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isValid":     false,
			"status":      "REVOKED",
			"isAuthentic": true,
			"message":     "File PDF cocok dengan QR Code ini, namun tanda tangan sudah tidak berlaku karena ada versi yang lebih baru.",
		})
		return
	}

	// Hash cocok & ACTIVE => dokumen asli untuk UUID ini.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"isValid":           true,
		"isAuthentic":       true,
		"status":            "ACTIVE",
		"message":           "File PDF terverifikasi ASLI sesuai QR Code ini.",
		"filename":          record.Filename,
		"signerName":        record.SignerName,
		"signerEmail":       record.SignerEmail,
		"issuerName":        record.IssuerName,
		"certificateSerial": record.CertificateSerial,
		"signedAt":          record.SignedAt.Format("02 Jan 2006 15:04:05"),
		"validUntil":        record.ValidUntil.Format("02 Jan 2006 15:04:05"),
	})
}

// 11. Pengguna mengajukan pembuatan Digital ID (.p12)
func requestDigitalIDLifecycleHandler(w http.ResponseWriter, r *http.Request) {
	var payload RequestIDPayload
	json.NewDecoder(r.Body).Decode(&payload)

	// Ambil role pengguna dari database
	var userRole string
	db.QueryRow("SELECT role FROM users WHERE email = ?", payload.Email).Scan(&userRole)

	expiredDate := time.Now().AddDate(2, 0, 0) // Berlaku 2 Tahun

	_, err := db.Exec(`INSERT INTO digital_id_requests 
		(email, owner_name, role, expired_date, is_approve, is_ready, is_sent, is_revoke) 
		VALUES (?, ?, ?, ?, FALSE, FALSE, FALSE, FALSE)`,
		payload.Email, payload.Name, userRole, expiredDate)

	if err != nil {
		http.Error(w, "Gagal mengirimkan pengajuan", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Pengajuan Digital ID berhasil dikirim ke Admin.")
}

// 12. Kendali Admin untuk mengubah flag (isApprove, isReady, isSent, isRevoke)
func adminUpdateLifecycleHandler(w http.ResponseWriter, r *http.Request) {
	var payload UpdateStatusPayload
	json.NewDecoder(r.Body).Decode(&payload)

	// Validasi kolom keamanan agar menghindari SQL Injection dinamis
	allowedFields := map[string]bool{"is_approve": true, "is_ready": true, "is_sent": true, "is_revoke": true}
	if !allowedFields[payload.Field] {
		http.Error(w, "Field tidak valid", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("UPDATE digital_id_requests SET %s = ? WHERE id = ?", payload.Field)
	_, err := db.Exec(query, payload.Value, payload.RequestID)
	if err != nil {
		http.Error(w, "Gagal memperbarui status", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Status %s berhasil diubah menjadi %v", payload.Field, payload.Value)
}

// 12 Menampilkan semua dokumen dari semua user untuk Admin
func adminHistoryHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, email, document_name, signer_name, signed_at, status FROM document_history ORDER BY id DESC")
	if err != nil {
		http.Error(w, "Gagal mengambil data admin", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var histories []HistoryItem
	for rows.Next() {
		var h HistoryItem
		rows.Scan(&h.ID, &h.Email, &h.DocumentName, &h.SignerName, &h.SignedAt, &h.Status)
		histories = append(histories, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(histories)
}

// Fungsi untuk Admin menyetujui atau menolak dokumen
func adminApproveDocumentHandler(w http.ResponseWriter, r *http.Request) {
	var payload ApprovePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		fmt.Println("[ERROR ADMIN] Gagal membaca data dari React:", err)
		http.Error(w, "Data tidak valid", http.StatusBadRequest)
		return
	}

	fmt.Printf("[INFO ADMIN] Memproses persetujuan... ID Dokumen: %d, Status Baru: %s\n", payload.DocumentID, payload.Status)

	// Coba update status dokumen
	_, err := db.Exec("UPDATE document_history SET status = ? WHERE id = ?", payload.Status, payload.DocumentID)

	if err != nil {
		fmt.Println("[ERROR DATABASE]", err) // CETAK ERROR ASLI DI TERMINAL

		// Jika error karena kolom 'status' belum ada
		if strings.Contains(err.Error(), "Unknown column") {
			fmt.Println("[INFO ADMIN] Kolom 'status' tidak ditemukan. Membuat kolom otomatis...")
			_, errAlter := db.Exec("ALTER TABLE document_history ADD COLUMN status VARCHAR(20) DEFAULT 'menunggu'")

			if errAlter == nil {
				db.Exec("UPDATE document_history SET status = ? WHERE id = ?", payload.Status, payload.DocumentID)
				fmt.Println("[INFO ADMIN] Sukses membuat kolom dan update status!")
				fmt.Fprintf(w, "Dokumen berhasil %s!", payload.Status)
				return
			} else {
				fmt.Println("[ERROR ALTER TABLE]", errAlter)
			}
		}

		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("[INFO ADMIN] Status dokumen berhasil diupdate ke MySQL!")
	fmt.Fprintf(w, "Dokumen berhasil %s!", payload.Status)
}

// Endpoint khusus untuk mencetak akun Admin secara otomatis
func setupAdminHandler(w http.ResponseWriter, r *http.Request) {
	email := "admin@unila.ac.id"
	pass := "admin123"
	hashed := hashPassword(pass)

	// 1. Pastikan kolom role benar-benar ada di tabel (Abaikan jika sudah ada)
	db.Exec("ALTER TABLE users ADD COLUMN role VARCHAR(20) DEFAULT 'user'")

	// 2. Hapus admin lama jika kebetulan nyangkut
	db.Exec("DELETE FROM users WHERE email = ?", email)

	// 3. Masukkan data Admin Baru
	_, err := db.Exec("INSERT INTO users (email, password, role) VALUES (?, ?, 'admin')", email, hashed)

	if err != nil {
		fmt.Fprintf(w, "Gagal membuat admin. Cek koneksi database Anda: %v", err)
		return
	}

	fmt.Fprintf(w, "SUKSES!\nAkun Admin berhasil dibuat.\n\nSilakan login ke Web Portal dengan:\nEmail: %s\nPassword: %s", email, pass)
}

func resetP12Handler(w http.ResponseWriter, r *http.Request) {
	// Pastikan hanya menerima method POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method tidak diizinkan", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Data tidak valid", http.StatusBadRequest)
		return
	}

	// Update kolom has_p12 menjadi FALSE di database
	_, err := db.Exec("UPDATE users SET has_p12 = FALSE WHERE email = ?", payload.Email)
	if err != nil {
		fmt.Println("[ERROR] Gagal mereset status p12:", err)
		http.Error(w, "Gagal melakukan reset sertifikat di sistem", http.StatusInternalServerError)
		return
	}

	fmt.Printf("[RESET] Sertifikat milik %s telah di-reset.\n", payload.Email)
	w.Write([]byte("Digital ID (.p12) berhasil di-reset. Anda sekarang dapat membuat sertifikat baru."))
}

func extractCertHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Ambil file .p12 dari form
	p12File, header, err := r.FormFile("p12_file")
	if err != nil {
		http.Error(w, "File .p12 tidak ditemukan", http.StatusBadRequest)
		return
	}
	defer p12File.Close()

	// 2. Baca file ke dalam bytes
	p12Bytes, err := io.ReadAll(p12File)
	if err != nil {
		http.Error(w, "Gagal membaca file", http.StatusInternalServerError)
		return
	}

	// 3. Ambil passphrase
	passphrase := r.FormValue("passphrase")

	// 4. Dekripsi dan Ekstrak PKCS#12
	// Kita mengabaikan privateKey (pakai garis bawah _) karena kita hanya butuh cert (Sertifikat Publik)
	_, cert, err := pkcs12.Decode(p12Bytes, passphrase)
	if err != nil {
		http.Error(w, "Passphrase salah atau file rusak", http.StatusUnauthorized)
		return
	}

	// 5. Ubah format sertifikat (DER) menjadi format PEM (crt/cer standard)
	pemBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	// 6. Kirim sebagai file .crt yang bisa didownload
	// Ganti nama file sesuai nama aslinya ditambah .crt
	outFilename := header.Filename + "_Public.crt"
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", outFilename))
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")

	w.Write(pemBytes)
}

// --- API BARU UNTUK WORKFLOW ---

// A. Mahasiswa Mengajukan Dokumen
func workflowCreateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == "OPTIONS" {
		return
	}

	err := r.ParseMultipartForm(20 << 20) // Maksimal 20MB
	if err != nil {
		http.Error(w, "Gagal memproses form", http.StatusBadRequest)
		return
	}

	mahasiswaEmail := r.FormValue("mahasiswa_email")
	dosenEmail := r.FormValue("dosen_email")

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File PDF wajib diunggah", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 1. BUAT FOLDER JIKA BELUM ADA
	os.MkdirAll("workflows", os.ModePerm)

	// 2. SIMPAN FILE FISIK KE FOLDER WORKFLOWS
	// Bikin nama unik biar nggak bentrok kalau ada file yang namanya sama
	hashName := fmt.Sprintf("wf_%d_%s", time.Now().Unix(), header.Filename)
	savePath := filepath.Join("workflows", hashName)

	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Gagal menyimpan file ke server", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	// 3. SIMPAN KE DATABASE (PENTING: savePath dimasukkan ke kolom file_path)
	_, err = db.Exec(`
		INSERT INTO document_workflows 
		(mahasiswa_email, dosen_email, document_name, file_path, status, created_at) 
		VALUES (?, ?, ?, ?, 'menunggu_dosen', NOW())
	`, mahasiswaEmail, dosenEmail, header.Filename, savePath)

	if err != nil {
		fmt.Println("Gagal insert ke database:", err)
		http.Error(w, "Gagal menyimpan data ke database", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Berhasil upload"}`))
}

type Workflow struct {
	ID             int    `json:"id"`
	MahasiswaEmail string `json:"mahasiswa_email"`
	DosenEmail     string `json:"dosen_email"`
	DocumentName   string `json:"document_name"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	FilePath       string `json:"file_path"`
}

// 1. HANDLER LIST WORKFLOW (Mengirim data lengkap ke semua role)
func workflowListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	email := r.URL.Query().Get("email")
	role := r.URL.Query().Get("role")

	var rows *sql.Rows
	var err error

	// Menggunakan COALESCE agar jika file_path NULL di database, Golang membacanya sebagai string kosong ""
	if role == "admin" {
		rows, err = db.Query("SELECT id, mahasiswa_email, dosen_email, document_name, status, created_at, COALESCE(file_path, '') FROM document_workflows WHERE status IN ('ditandatangani', 'selesai', 'diterima_dosen') ORDER BY id DESC")
	} else if role == "dosen" {
		rows, err = db.Query("SELECT id, mahasiswa_email, dosen_email, document_name, status, created_at, COALESCE(file_path, '') FROM document_workflows WHERE dosen_email = ? ORDER BY id DESC", email)
	} else {
		rows, err = db.Query("SELECT id, mahasiswa_email, dosen_email, document_name, status, created_at, COALESCE(file_path, '') FROM document_workflows WHERE mahasiswa_email = ? ORDER BY id DESC", email)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var workflows []Workflow = []Workflow{}
	for rows.Next() {
		var wf Workflow
		err := rows.Scan(&wf.ID, &wf.MahasiswaEmail, &wf.DosenEmail, &wf.DocumentName, &wf.Status, &wf.CreatedAt, &wf.FilePath)
		if err != nil {
			fmt.Println("Error scan baris:", err)
			continue
		}
		workflows = append(workflows, wf)
	}

	json.NewEncoder(w).Encode(workflows)
}

// 2. HANDLER UPDATE STATUS (Untuk aksi Dosen Terima dan Admin Setuju)
func workflowActionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		return
	}

	var req struct {
		ID     int    `json:"id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var targetStatus string
	if req.Action == "diterima_dosen" {
		targetStatus = "diterima_dosen"
	} else if req.Action == "selesai" {
		targetStatus = "selesai"
	}

	if targetStatus != "" {
		_, err := db.Exec("UPDATE document_workflows SET status = ? WHERE id = ?", targetStatus, req.ID)
		if err != nil {
			http.Error(w, "Gagal update status database", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// 3. HANDLER GET FILE (Mengirimkan file fisik PDF ke React)
func getFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		return
	}

	filePath := r.URL.Query().Get("path")
	safePath := filepath.Clean(filePath)

	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		http.Error(w, "File fisik tidak ditemukan di server", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, safePath)
}

// Tambahkan fungsi ini di main.go
func getDosenListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT email, name FROM users WHERE role = 'dosen'")
	if err != nil {
		http.Error(w, "Gagal mengambil data dosen", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var dosens []map[string]string
	for rows.Next() {
		var email, name string
		rows.Scan(&email, &name)
		dosens = append(dosens, map[string]string{"email": email, "name": name})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dosens)
}

func viewDocumentHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Ambil UUID dari URL
	uuid := strings.TrimPrefix(r.URL.Path, "/view/")

	// 2. Ambil path file dari DB
	var filePath string
	err := db.QueryRow("SELECT file_path FROM signed_documents WHERE uuid = ?", uuid).Scan(&filePath)
	if err != nil {
		http.Error(w, "Dokumen tidak ditemukan", http.StatusNotFound)
		return
	}

	// 3. Serve file PDF ke browser
	// Set header agar browser langsung menampilkan PDF (bukan download otomatis)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=document.pdf")

	http.ServeFile(w, r, filePath)
}

func getUserStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Set header CORS dan JSON
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Email tidak diberikan", http.StatusBadRequest)
		return
	}

	var hasP12 bool
	// Ambil status has_p12 dari tabel users berdasarkan email
	err := db.QueryRow("SELECT has_p12 FROM users WHERE email = ?", email).Scan(&hasP12)
	if err != nil {
		fmt.Println("[ERROR DB] Gagal mengambil status user:", err)
		http.Error(w, "Gagal mengambil status pengguna", http.StatusInternalServerError)
		return
	}

	// Kirim balik statusnya ke React
	json.NewEncoder(w).Encode(map[string]interface{}{
		"has_p12": hasP12,
	})
}

func main() {
	initDB()
	mux := http.NewServeMux()
	mux.HandleFunc("/register", registerHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/verify-otp", verifyOTPHandler)
	mux.HandleFunc("/generate", generateOTPHandler)
	mux.HandleFunc("/reset-otp", resetOTPHandler)
	mux.HandleFunc("/request-id", requestDigitalIDHandler)
	mux.HandleFunc("/web-sign", webSignHandler)
	mux.HandleFunc("/history", historyHandler)
	mux.HandleFunc("/download", downloadDocumentHandler)
	mux.HandleFunc("/download-signed", downloadSignedDocumentHandler)
	mux.HandleFunc("/verify-pdf", verifyDocumentHandler)
	mux.HandleFunc("/verify-file", verifyFileHandler)
	mux.HandleFunc("/verify/", verifySignedDocumentHandler)
	mux.HandleFunc("/request-id-lifecycle", requestDigitalIDLifecycleHandler)
	mux.HandleFunc("/admin/update-status", adminUpdateLifecycleHandler)
	mux.HandleFunc("/admin/history", adminHistoryHandler)
	mux.HandleFunc("/admin/approve", adminApproveDocumentHandler)
	mux.HandleFunc("/setup-admin", setupAdminHandler)
	mux.HandleFunc("/reset-p12", resetP12Handler)
	mux.HandleFunc("/extract-cert", extractCertHandler)
	mux.HandleFunc("/workflow/create", workflowCreateHandler)
	mux.HandleFunc("/workflow/list", workflowListHandler)
	mux.HandleFunc("/workflow/action", workflowActionHandler)
	mux.HandleFunc("/dosen/list", getDosenListHandler)
	mux.HandleFunc("/get-file", getFileHandler)
	mux.HandleFunc("/view/", viewDocumentHandler)
	mux.HandleFunc("/user/status", getUserStatusHandler)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"https://localhost:3000", // <-- INI YANG KURANG!
			"http://dgsign.test:3000",
			"https://dgsign.test:3000",
		},
		AllowedMethods: []string{"GET", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"}, // <-- 'Authorization' WAJIB ADA
	})
	fmt.Println("Backend HTTPS berjalan di port 8081...")

	fmt.Println("==================================================")
	fmt.Println("🚀 SERVER SIAP TEMPUR!")
	fmt.Println("⚠️  JALUR MERAH : HTTP (Tanpa SSL) aktif di port 8080")
	fmt.Println("🔒 JALUR AMAN  : HTTPS (Dengan SSL) aktif di port 8081")
	go func() {
		err := http.ListenAndServe(":8080", c.Handler(mux))
		if err != nil {
			log.Fatal("Gagal menjalankan HTTP:", err)
		}
	}()

	// 2. Jalankan HTTPS (SSL) sebagai proses utama
	errTls := http.ListenAndServeTLS(":8081", "cert.pem", "key.pem", c.Handler(mux))
	if errTls != nil {
		log.Fatal("Gagal menjalankan HTTPS (Pastikan cert.pem & key.pem ada):", errTls)
	}
}
