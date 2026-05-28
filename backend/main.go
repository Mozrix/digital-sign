package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/pquerna/otp/totp"
	"github.com/rs/cors"
	"software.sslmate.com/src/go-pkcs12"
)

var db *sql.DB

// --- STRUKTUR DATA ---
type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
	DocumentName string `json:"document_name"`
	SignerName   string `json:"signer_name"`
	SignedAt     string `json:"signed_at"`
}

func initDB() {
	var err error
	db, err = sql.Open("mysql", "root:@tcp(127.0.0.1:3306)/dgsign_db")
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal("Gagal terhubung ke MySQL:", err)
	}
	fmt.Println("Koneksi MySQL Berhasil!")
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// 1. Register
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var payload AuthPayload
	json.NewDecoder(r.Body).Decode(&payload)
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

	hashedPassword := hashPassword(payload.Password)
	var otpSecret sql.NullString

	err := db.QueryRow("SELECT otp_secret FROM users WHERE email = ? AND password = ?", payload.Email, hashedPassword).Scan(&otpSecret)
	if err != nil {
		http.Error(w, "Email atau password salah", http.StatusUnauthorized)
		return
	}

	// Jika otp_secret kosong/NULL, user harus setup dulu
	requireSetup := !otpSecret.Valid || otpSecret.String == ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Kredensial valid",
		"requireSetup": requireSetup,
	})
}

// 3. Verifikasi OTP saat Login / Setup
func verifyOTPHandler(w http.ResponseWriter, r *http.Request) {
	var payload OTPVerifyPayload
	json.NewDecoder(r.Body).Decode(&payload)

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

	key, _ := totp.Generate(totp.GenerateOpts{Issuer: "Sistem-DGSign", AccountName: payload.Email})
	db.Exec("UPDATE users SET otp_secret = ? WHERE email = ?", key.Secret(), payload.Email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": key.URL()})
}

// 5. Reset OTP
func resetOTPHandler(w http.ResponseWriter, r *http.Request) {
	var payload BasicPayload
	json.NewDecoder(r.Body).Decode(&payload)
	db.Exec("UPDATE users SET otp_secret = NULL WHERE email = ?", payload.Email)
	fmt.Fprintf(w, "OTP direset.")
}

// 6. Request Digital ID
func requestDigitalIDHandler(w http.ResponseWriter, r *http.Request) {
	var payload RequestIDPayload
	json.NewDecoder(r.Body).Decode(&payload)
	db.Exec("UPDATE users SET name = ? WHERE email = ?", payload.Name, payload.Email)

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      pkix.Name{CommonName: payload.Name},
		NotBefore:    time.Now(), NotAfter: time.Now().AddDate(2, 0, 0),
	}
	certBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	cert, _ := x509.ParseCertificate(certBytes)
	pfxData, _ := pkcs12.Encode(rand.Reader, privateKey, cert, nil, payload.Passphrase)

	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", `attachment; filename="digital_id.p12"`)
	w.Write(pfxData)
}

// 7. Web Sign (Dengan Pencegahan Duplikat, Injeksi Gambar, & Injeksi Nama)
func webSignHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Ukuran file terlalu besar", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	otpCode := r.FormValue("otpCode")
	signerName := r.FormValue("signerName")

	var secret sql.NullString
	db.QueryRow("SELECT otp_secret FROM users WHERE email = ?", email).Scan(&secret)
	if !totp.Validate(otpCode, secret.String) {
		http.Error(w, "OTP Salah atau Kedaluwarsa", http.StatusUnauthorized)
		return
	}

	pdfFile, pdfHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Dokumen PDF wajib diunggah", http.StatusBadRequest)
		return
	}
	defer pdfFile.Close()

	// --- PENGECEKAN DUPLIKASI SAAT UPLOAD ---
	var existing int
	errCheck := db.QueryRow("SELECT id FROM document_history WHERE document_name = ?", pdfHeader.Filename).Scan(&existing)
	if errCheck == nil {
		http.Error(w, "Gagal: Dokumen dengan nama ini sudah pernah ditandatangani. Harap ubah nama file Anda (misal: dokumen_v2.pdf).", http.StatusConflict)
		return
	}

	os.MkdirAll("uploads", os.ModePerm)
	uniqueFileName := fmt.Sprintf("%d_%s", time.Now().Unix(), pdfHeader.Filename)
	finalPdfPath := filepath.Join("uploads", uniqueFileName)

	// Buat ID unik untuk penamaan file sementara (temp file)
	tempID := time.Now().UnixNano()

	// Simpan PDF asli sebagai file kerja pertama
	currentPdfPath := filepath.Join("uploads", fmt.Sprintf("temp_base_%d.pdf", tempID))
	tempPdf, _ := os.Create(currentPdfPath)
	io.Copy(tempPdf, pdfFile)
	tempPdf.Close()
	defer os.Remove(currentPdfPath) // Akan otomatis dihapus saat fungsi selesai

	// --- 1. PROSES INJEKSI GAMBAR (OPSIONAL) ---
	imgFile, _, errImg := r.FormFile("signatureImage")
	if errImg == nil {
		defer imgFile.Close()
		tempImgPath := filepath.Join("uploads", fmt.Sprintf("temp_img_%d.png", tempID))
		tempImg, _ := os.Create(tempImgPath)
		io.Copy(tempImg, imgFile)
		tempImg.Close()
		defer os.Remove(tempImgPath)

		// Koordinat Gambar (Y = 135) diletakkan agak ke atas agar ada ruang untuk teks
		wmImg, errWmImg := api.ImageWatermark(tempImgPath, "pos:br, scale:0.35, offset:-100 135, rot:0", true, false, types.POINTS)

		if errWmImg == nil {
			nextPdfPath := filepath.Join("uploads", fmt.Sprintf("temp_withimg_%d.pdf", tempID))
			err = api.AddWatermarksFile(currentPdfPath, nextPdfPath, []string{"1"}, wmImg, nil)
			if err == nil {
				// Pindahkan file kerja ke PDF yang sudah berisi gambar
				currentPdfPath = nextPdfPath
				defer os.Remove(currentPdfPath)
			}
		}
	}

	// --- 2. PROSES INJEKSI NAMA PENANDATANGAN (TEKS) ---
	// Koordinat Teks (Y = 90) diletakkan di bawah gambar (Y = 135)
	textConfig := "font:Helvetica, points:12, pos:br, offset:-100 90, rot:0"
	wmText, errWmText := api.TextWatermark(signerName, textConfig, true, false, types.POINTS)

	if errWmText == nil {
		// Suntikkan teks ke file kerja terakhir dan simpan sebagai Hasil Final
		err = api.AddWatermarksFile(currentPdfPath, finalPdfPath, []string{"1"}, wmText, nil)
		if err != nil {
			os.Rename(currentPdfPath, finalPdfPath) // Fallback jika teks gagal diinjeksi
		}
	} else {
		os.Rename(currentPdfPath, finalPdfPath)
	}

	// 3. Simpan Riwayat ke Database
	_, err = db.Exec("INSERT INTO document_history (email, document_name, signer_name, file_path) VALUES (?, ?, ?, ?)",
		email, pdfHeader.Filename, signerName, finalPdfPath)

	fmt.Fprintf(w, "Dokumen atas nama %s berhasil ditandatangani!", signerName)
}

// 8. Ambil Riwayat Dokumen
func historyHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")

	rows, err := db.Query("SELECT id, document_name, signer_name, signed_at FROM document_history WHERE email = ? ORDER BY signed_at DESC", email)
	if err != nil {
		http.Error(w, "Gagal mengambil data", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var histories []HistoryItem
	for rows.Next() {
		var h HistoryItem
		rows.Scan(&h.ID, &h.DocumentName, &h.SignerName, &h.SignedAt)
		histories = append(histories, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(histories)
}

// 9. Download Dokumen dari Riwayat
func downloadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	var filePath, docName string
	err := db.QueryRow("SELECT file_path, document_name FROM document_history WHERE id = ?", id).Scan(&filePath, &docName)
	if err != nil {
		http.Error(w, "Dokumen tidak ditemukan", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"Signed_%s\"", docName))
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, filePath)
}

// 10. Verifikasi Keaslian Dokumen (Dengan Cek Detail Duplikat)
func verifyDocumentHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Gagal membaca file", http.StatusBadRequest)
		return
	}

	pdfFile, pdfHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File PDF tidak ditemukan", http.StatusBadRequest)
		return
	}
	defer pdfFile.Close()

	// 1. Ambil SEMUA data riwayat berdasarkan nama file, URUTKAN dari yang paling awal (ASC)
	rows, err := db.Query("SELECT signer_name, signed_at FROM document_history WHERE document_name = ? ORDER BY signed_at ASC", pdfHeader.Filename)
	if err != nil {
		http.Error(w, "Terjadi kesalahan database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Struct sementara untuk menyimpan nama dan waktu
	type SignerInfo struct {
		Name string
		Date string
	}
	var signers []SignerInfo

	for rows.Next() {
		var sName, sDate string
		rows.Scan(&sName, &sDate)
		signers = append(signers, SignerInfo{Name: sName, Date: sDate})
	}

	// 2. Jika tidak ada di database sama sekali
	if len(signers) == 0 {
		http.Error(w, "Tanda tangan digital tidak ditemukan atau tidak valid pada sistem kami.", http.StatusNotFound)
		return
	}

	// 3. Jika dokumen ditemukan lebih dari 1 kali (Terduplikasi)
	if len(signers) > 1 {
		// Index 0 adalah orang pertama (Asli)
		originalSigner := signers[0].Name

		// Kumpulkan sisa nama yang melakukan duplikasi
		var duplicators []string
		for i := 1; i < len(signers); i++ {
			duplicators = append(duplicators, signers[i].Name)
		}

		// Gabungkan nama-nama penduplikat menggunakan koma
		duplicatorNames := strings.Join(duplicators, ", ")

		// Susun pesan Alert (menggunakan \n untuk enter/baris baru di alert browser)
		errMsg := fmt.Sprintf("PERINGATAN: Dokumen Terduplikasi!\n\n✅ Dokumen asli pertama kali ditandatangani oleh:\n- %s\n\n⚠️ Dokumen ini kemudian diduplikasi/ditandatangani ulang oleh:\n- %s", originalSigner, duplicatorNames)

		http.Error(w, errMsg, http.StatusConflict)
		return
	}

	// 4. Jika datanya valid dan tepat hanya ada 1 (Tidak duplikat)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "VALID",
		"signer":  signers[0].Name,
		"date":    signers[0].Date,
		"message": "Dokumen ini valid dan memiliki tanda tangan digital yang sah.",
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
	mux.HandleFunc("/verify-pdf", verifyDocumentHandler)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST"}, AllowedHeaders: []string{"Content-Type"},
	})
	fmt.Println("Backend berjalan di port 8081...")
	log.Fatal(http.ListenAndServe(":8081", c.Handler(mux)))
}
