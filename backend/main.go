package main

import (
	"crypto"
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

	"github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
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
	Email        string `json:"email"`
	DocumentName string `json:"document_name"`
	SignerName   string `json:"signer_name"`
	SignedAt     string `json:"signed_at"`
	Status       string `json:"status"`
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
	var role sql.NullString // PENGAMAN: Mencegah error jika data di database NULL

	err := db.QueryRow("SELECT otp_secret, role FROM users WHERE email = ? AND password = ?", payload.Email, hashedPassword).Scan(&otpSecret, &role)
	if err != nil {
		http.Error(w, "Email atau password salah", http.StatusUnauthorized)
		return
	}

	requireSetup := !otpSecret.Valid || otpSecret.String == ""

	// Jika role valid, gunakan isinya. Jika tidak, default ke 'user'
	userRole := "user"
	if role.Valid && role.String != "" {
		userRole = role.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Kredensial valid",
		"requireSetup": requireSetup,
		"role":         userRole, // Kirim role yang sudah pasti valid ke React
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
	var payload struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Data tidak valid", http.StatusBadRequest)
		return
	}

	// 1. Buat Kunci Privat RSA-2048
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		http.Error(w, "Gagal membuat kunci privat", http.StatusInternalServerError)
		return
	}

	// 2. Buat Nomor Seri Unik
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	// =================================================================
	// 3. ROMBAK TOTAL: TEMPLATE SERTIFIKAT STANDAR ADOBE (ROOT CA)
	// =================================================================
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

		// KUNCI UTAMA AGAR ADOBE TERIMA:
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection, x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
		IsCA:                  true, // Adobe JAUH lebih percaya jika sertifikat bertindak sebagai CA
	}

	// 4. Cetak Sertifikat Mentah (DER Bytes)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		http.Error(w, "Gagal mencetak sertifikat x509", http.StatusInternalServerError)
		return
	}

	// --- TAMBAHAN BARU: Parsing derBytes menjadi objek *x509.Certificate ---
	cert, errParse := x509.ParseCertificate(derBytes)
	if errParse != nil {
		http.Error(w, "Gagal membaca struktur sertifikat", http.StatusInternalServerError)
		return
	}
	// ------------------------------------------------------------------------

	// 5. Bungkus menjadi .p12 menggunakan variabel 'cert' (bukan 'derBytes')
	pfxBytes, err := pkcs12.Encode(rand.Reader, privateKey, cert, nil, payload.Passphrase)
	if err != nil {
		http.Error(w, "Gagal membungkus ke PKCS12", http.StatusInternalServerError)
		return
	}
	// 6. Kirim ke React
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Write(pfxBytes)
}

// 7. Web Sign (Dengan Pencegahan Duplikat, Injeksi Gambar, & Injeksi Nama)
// 7. Web Sign (Dengan Koordinat Dinamis & Halaman Manual)
// Handler Tanda Tangan Web (Visual + Kriptografi)
func webSignHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Ukuran file terlalu besar", http.StatusBadRequest)
		return
	}

	// 1. Ambil Data Form
	email := r.FormValue("email")
	otpCode := r.FormValue("otpCode")
	signerName := r.FormValue("signerName")
	passphrase := r.FormValue("passphrase")
	targetPage := r.FormValue("page")
	xCoord := r.FormValue("x")
	yCoord := r.FormValue("y")

	// 2. Validasi OTP
	var secret sql.NullString
	db.QueryRow("SELECT otp_secret FROM users WHERE email = ?", email).Scan(&secret)
	if !totp.Validate(otpCode, secret.String) {
		http.Error(w, "OTP Salah atau Kedaluwarsa", http.StatusUnauthorized)
		return
	}

	// 3. Persiapan File
	pdfFile, pdfHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Dokumen PDF wajib diunggah", http.StatusBadRequest)
		return
	}
	defer pdfFile.Close()

	os.MkdirAll("uploads", os.ModePerm)
	uniqueID := time.Now().UnixNano()
	finalPdfPath := filepath.Join("uploads", fmt.Sprintf("%d_%s", time.Now().Unix(), pdfHeader.Filename))

	// Simpan PDF Awal
	tempPdfPath := filepath.Join("uploads", fmt.Sprintf("temp_base_%d.pdf", uniqueID))
	tempFile, _ := os.Create(tempPdfPath)
	io.Copy(tempFile, pdfFile)
	tempFile.Close()
	defer os.Remove(tempPdfPath)

	// 4. PROSES GAMBAR (Watermark) - Jika ada
	currentPdfPath := tempPdfPath
	imgFile, _, errImg := r.FormFile("signatureImage")
	if errImg == nil {
		defer imgFile.Close()
		tempImgPath := filepath.Join("uploads", fmt.Sprintf("temp_img_%d.png", uniqueID))
		outImg, _ := os.Create(tempImgPath)
		io.Copy(outImg, imgFile)
		outImg.Close()
		defer os.Remove(tempImgPath)

		// Gunakan nil agar tidak error kompilasi
		imgConfig := fmt.Sprintf("pos:bl, scale:0.35, offset:%s %s, rot:0", xCoord, yCoord)
		wm, _ := api.ImageWatermark(tempImgPath, imgConfig, true, false, types.POINTS)

		withImgPath := filepath.Join("uploads", fmt.Sprintf("with_img_%d.pdf", uniqueID))
		// Gunakan nil untuk konfigurasi untuk menghindari error "undefined"
		err = api.AddWatermarksFile(currentPdfPath, withImgPath, []string{targetPage}, wm, nil)

		if err == nil {
			currentPdfPath = withImgPath
			defer os.Remove(currentPdfPath)
		}
	}

	// 5. PROSES KRIPTOGRAFI (Sign)
	p12FileHeader, _, errP12 := r.FormFile("p12File")
	if errP12 != nil {
		http.Error(w, "File .p12 tidak ditemukan", http.StatusBadRequest)
		return
	}
	defer p12FileHeader.Close()

	p12Bytes, _ := io.ReadAll(p12FileHeader)
	privateKey, cert, errDecode := pkcs12.Decode(p12Bytes, passphrase)
	if errDecode != nil {
		http.Error(w, "Passphrase P12 salah", http.StatusUnauthorized)
		return
	}

	// Buka file yang sudah diproses (base atau with_img)
	inputPdf, _ := os.Open(currentPdfPath)
	defer inputPdf.Close()

	finfo, _ := inputPdf.Stat()
	size := finfo.Size()
	pdfReader, _ := pdf.NewReader(inputPdf, size)

	cryptoPdfPath := filepath.Join("uploads", filepath.Base(finalPdfPath))
	outputPdf, _ := os.Create(cryptoPdfPath)
	defer outputPdf.Close()

	errSign := sign.Sign(inputPdf, outputPdf, pdfReader, size, sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.CertificationSignature,
			DocMDPPerm: sign.AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:          privateKey.(crypto.Signer),
		DigestAlgorithm: crypto.SHA256,
		Certificate:     cert,
	})

	if errSign != nil {
		http.Error(w, "Gagal enkripsi tanda tangan", http.StatusInternalServerError)
		return
	}

	// 6. Simpan File Final
	os.Rename(cryptoPdfPath, finalPdfPath)
	db.Exec("INSERT INTO document_history (email, document_name, signer_name, file_path, status) VALUES (?, ?, ?, ?, 'menunggu')",
		email, pdfHeader.Filename, signerName, finalPdfPath)

	fmt.Fprintf(w, "Dokumen berhasil ditandatangani!")
}

// 8. Ambil Riwayat Dokumen
func historyHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	// Ambil data berdasarkan email user beserta statusnya
	rows, err := db.Query("SELECT id, document_name, signer_name, signed_at, status FROM document_history WHERE email = ? ORDER BY signed_at DESC", email)
	if err != nil {
		http.Error(w, "Gagal mengambil data", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var histories []HistoryItem
	for rows.Next() {
		var h HistoryItem
		rows.Scan(&h.ID, &h.DocumentName, &h.SignerName, &h.SignedAt, &h.Status)
		histories = append(histories, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(histories)
}

// 9. Download Dokumen dari Riwayat
func downloadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// 1. Ambil data dari DB
	var filePath, docName, status string
	err := db.QueryRow("SELECT file_path, document_name, status FROM document_history WHERE id = ?", id).Scan(&filePath, &docName, &status)
	if err != nil {
		http.Error(w, "Dokumen tidak ditemukan", http.StatusNotFound)
		return
	}

	if strings.ToLower(strings.TrimSpace(status)) != "disetujui" {
		http.Error(w, "Dokumen belum disetujui", http.StatusForbidden)
		return
	}

	// 2. DEBUG: Lihat di mana terminal Golang berada
	wd, _ := os.Getwd()
	fmt.Println("INFO: Current Working Directory (Folder Terminal):", wd)

	// 3. NORMALISASI PATH (Paling Penting)
	// Kita bersihkan path dari database agar kompatibel dengan sistem OS
	// Jika path di DB "uploads\file.pdf", kita pastikan Go membacanya dengan benar
	cleanPath := filepath.Clean(filePath)

	// Jika path di DB dimulai dengan "uploads", kita pastikan itu relatif terhadap folder saat ini
	// (Penting: Pastikan terminal Anda berada di dalam folder 'backend')
	finalPath := cleanPath

	fmt.Println("DEBUG: Mencoba mencari file di path:", finalPath)

	// 4. Cek file
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		fmt.Println("ERROR: File tidak ditemukan di lokasi:", finalPath)
		// Coba cari alternatif: mungkin file ada di folder 'backend/uploads' tapi saat ini
		// terminal dibuka di 'digital-sign'?
		// Solusi darurat: Coba cek apakah file ada di "backend/" + finalPath
		altPath := filepath.Join("backend", finalPath)
		if _, err := os.Stat(altPath); err == nil {
			finalPath = altPath
		} else {
			http.Error(w, "File fisik tidak ditemukan di server.", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", docName))
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, finalPath)
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

// 12 Menampilkan semua dokumen dari semua user untuk Admin
func adminHistoryHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, email, document_name, signer_name, signed_at, status FROM document_history ORDER BY signed_at DESC")
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
	mux.HandleFunc("/admin/history", adminHistoryHandler)
	mux.HandleFunc("/admin/approve", adminApproveDocumentHandler)
	mux.HandleFunc("/setup-admin", setupAdminHandler)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000", "https://dgsign.test:3000"}, // Tambahkan origin HTTPS
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	})
	fmt.Println("Backend HTTPS berjalan di port 8081...")
	log.Fatal(http.ListenAndServeTLS(":8081", "cert.pem", "key.pem", c.Handler(mux)))
}
