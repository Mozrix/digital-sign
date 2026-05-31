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
	"regexp"
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

type UpdateStatusPayload struct {
	RequestID int    `json:"request_id"`
	Field     string `json:"field"` // "is_approve", "is_ready", "is_sent", "is_revoke"
	Value     bool   `json:"value"`
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

// 6. Request Digital ID (Dibatasi Hanya Bisa 1 Kali Set Passphrase)
func requestDigitalIDHandler(w http.ResponseWriter, r *http.Request) {
	var payload RequestIDPayload
	json.NewDecoder(r.Body).Decode(&payload)

	// --- LOGIKA BARU: VALIDASI PENGATURAN 1 KALI ---
	var existingName sql.NullString
	db.QueryRow("SELECT name FROM users WHERE email = ?", payload.Email).Scan(&existingName)
	if existingName.Valid && existingName.String != "" {
		http.Error(w, "Gagal: Akun Anda sudah pernah mengatur Passphrase Identitas Digital. Pembuatan ulang tidak diizinkan.", http.StatusBadRequest)
		return
	}
	// -----------------------------------------------

	// Jika belum pernah buat, update nama dan generate .p12
	db.Exec("UPDATE users SET name = ? WHERE email = ?", payload.Name, payload.Email)

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      pkix.Name{CommonName: payload.Name},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(2, 0, 0), // Expired Date otomatis 2 tahun standar global
	}
	certBytes, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	cert, _ := x509.ParseCertificate(certBytes)

	// Encode sertifikat menjadi identitas digital (.p12) yang kompatibel penuh dengan Adobe Acrobat
	pfxData, _ := pkcs12.Encode(rand.Reader, privateKey, cert, nil, payload.Passphrase)

	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", `attachment; filename="digital_id.p12"`)
	w.Write(pfxData)
}

// 7. Web Sign (Dengan Pencegahan Duplikat, Injeksi Gambar, & Injeksi Nama)
// 7. Web Sign (Dengan Koordinat Dinamis & Halaman Manual)
func webSignHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Ukuran file terlalu besar", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	otpCode := r.FormValue("otpCode")
	signerName := r.FormValue("signerName")

	// --- TANGKAP INPUT POSISI MANUAL DARI FRONTEND ---
	targetPage := r.FormValue("page") // Contoh: "1", "2", "3"
	xCoord := r.FormValue("x")        // Koordinat X dari klik mouse
	yCoord := r.FormValue("y")        // Koordinat Y dari klik mouse

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

	os.MkdirAll("uploads", os.ModePerm)
	uniqueFileName := fmt.Sprintf("%d_%s", time.Now().Unix(), pdfHeader.Filename)
	finalPdfPath := filepath.Join("uploads", uniqueFileName)

	tempID := time.Now().UnixNano()
	currentPdfPath := filepath.Join("uploads", fmt.Sprintf("temp_base_%d.pdf", tempID))
	tempPdf, _ := os.Create(currentPdfPath)
	io.Copy(tempPdf, pdfFile)
	tempPdf.Close()
	defer os.Remove(currentPdfPath)

	imgFile, _, errImg := r.FormFile("signatureImage")
	if errImg == nil {
		defer imgFile.Close()
		tempImgPath := filepath.Join("uploads", fmt.Sprintf("temp_img_%d.png", tempID))
		tempImg, _ := os.Create(tempImgPath)
		io.Copy(tempImg, imgFile)
		tempImg.Close()
		defer os.Remove(tempImgPath)

		// POSISI DAN KOORDINAT SEKARANG BERSIFAT DINAMIS
		// Menggunakan kombinasi xCoord dan yCoord dari parameter Frontend
		imgConfig := fmt.Sprintf("pos:bl, scale:0.35, offset:%s %s, rot:0", xCoord, yCoord)
		wmImg, errWmImg := api.ImageWatermark(tempImgPath, imgConfig, true, false, types.POINTS)

		if errWmImg == nil {
			nextPdfPath := filepath.Join("uploads", fmt.Sprintf("temp_withimg_%d.pdf", tempID))
			// Menggunakan targetPage secara dinamis, bukan cuma halaman "1" lagi
			err = api.AddWatermarksFile(currentPdfPath, nextPdfPath, []string{targetPage}, wmImg, nil)
			if err == nil {
				currentPdfPath = nextPdfPath
				defer os.Remove(currentPdfPath)
			}
		}
	}

	// Cetak Teks nama penandatangan tepat di bawah koordinat gambar tanda tangan
	// Mengurangi nilai Y agar posisi teks berada sedikit di bawah gambar
	var yText int
	fmt.Sscanf(yCoord, "%d", &yText)
	textConfig := fmt.Sprintf("font:Helvetica, points:12, pos:bl, offset:%s %d, rot:0", xCoord, yText-35)
	wmText, errWmText := api.TextWatermark(signerName, textConfig, true, false, types.POINTS)

	if errWmText == nil {
		err = api.AddWatermarksFile(currentPdfPath, finalPdfPath, []string{targetPage}, wmText, nil)
		if err != nil {
			os.Rename(currentPdfPath, finalPdfPath) // Fallback ke file sebelumnya (tanpa teks)
		}
	} else {
		os.Rename(currentPdfPath, finalPdfPath) // Fallback ke file sebelumnya (tanpa teks)
	}

	os.Rename(currentPdfPath, finalPdfPath)

	db.Exec("INSERT INTO document_history (email, document_name, signer_name, file_path) VALUES (?, ?, ?, ?)",
		email, pdfHeader.Filename, signerName, finalPdfPath)
	fmt.Fprintf(w, "Dokumen berhasil ditandatangani di halaman %s!", targetPage)
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
// 10. Verifikasi Keaslian Dokumen (Mendukung File Hasil Unduhan "Signed_")
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

	// --- LOGIKA BARU: PEMBERSIHAN NAMA FILE HASIL UNDUHAN ---
	filename := pdfHeader.Filename

	// 1. Hapus prefix "Signed_" dari hasil unduhan
	filename = strings.TrimPrefix(filename, "Signed_")

	// 2. Hapus angka duplikasi dari browser seperti " (1)", " (7)", dsb.
	// Ini akan mengubah "nama file (7).pdf" kembali menjadi "nama file.pdf"
	re := regexp.MustCompile(` \(\d+\)\.pdf$`)
	filename = re.ReplaceAllString(filename, ".pdf")
	// ----------------------------------------------------------------------

	// Ambil SEMUA data riwayat berdasarkan nama file asli yang sudah dibersihkan total
	rows, err := db.Query("SELECT signer_name, signed_at FROM document_history WHERE document_name = ? ORDER BY signed_at ASC", filename)
	if err != nil {
		http.Error(w, "Terjadi kesalahan database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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

	if len(signers) == 0 {
		http.Error(w, "Tanda tangan digital tidak ditemukan atau tidak valid pada sistem kami.", http.StatusNotFound)
		return
	}

	if len(signers) > 1 {
		originalSigner := signers[0].Name
		var duplicators []string
		for i := 1; i < len(signers); i++ {
			duplicators = append(duplicators, signers[i].Name)
		}
		duplicatorNames := strings.Join(duplicators, ", ")
		errMsg := fmt.Sprintf("PERINGATAN: Dokumen Terduplikasi!\n\n✅ Dokumen asli pertama kali ditandatangani oleh:\n- %s\n\n⚠️ Dokumen ini kemudian diduplikasi/ditandatangani ulang oleh:\n- %s", originalSigner, duplicatorNames)

		http.Error(w, errMsg, http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "VALID",
		"signer":  signers[0].Name,
		"date":    signers[0].Date,
		"message": "Dokumen ini valid dan memiliki tanda tangan digital yang sah.",
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
	mux.HandleFunc("/request-id-lifecycle", requestDigitalIDLifecycleHandler)
	mux.HandleFunc("/admin/update-status", adminUpdateLifecycleHandler)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST"}, AllowedHeaders: []string{"Content-Type"},
	})
	fmt.Println("Backend berjalan di port 8081...")
	log.Fatal(http.ListenAndServe(":8081", c.Handler(mux)))
}
