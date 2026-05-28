# DGSign - Sistem Tanda Tangan Digital Terpadu

DGSign adalah aplikasi web berbasis **React.js** dan **Golang** yang mengintegrasikan keamanan kriptografi dan otentikasi lapis ganda (2FA) untuk mengamankan proses penandatanganan dokumen PDF. Aplikasi ini mencegah manipulasi dan duplikasi dokumen dengan menerapkan *watermark* grafis dan validasi terpusat.

## 🚀 Fitur Utama

- **Keamanan 2FA (Two-Factor Authentication)**: Menggunakan TOTP (Google Authenticator) sebagai gerbang otentikasi utama dan otorisasi tahap akhir saat menandatangani dokumen.
- **Injeksi Sertifikat & Watermark**: Menyuntikkan metadata, teks nama penandatangan, dan gambar spesimen tanda tangan (PNG/JPG) ke dalam file PDF menggunakan pustaka `pdfcpu`.
- **Anti-Duplikasi**: Mencegah dokumen yang sama (berdasarkan nama *file*) ditandatangani berulang kali. Jika dokumen diunggah ulang, sistem akan mendeteksi dan menampilkan daftar penduplikat serta pemilik aslinya.
- **Riwayat Dokumen**: Menyimpan jejak rekam penandatanganan dan memungkinkan pengguna mengunduh kembali dokumen yang telah ditandatangani.

## 🛠️ Teknologi yang Digunakan

- **Frontend**: React.js (Vite), Axios, QRCode.react
- **Backend**: Golang (net/http, Go-SQL-Driver)
- **Manipulasi PDF**: [pdfcpu](https://github.com/pdfcpu/pdfcpu)
- **Keamanan**: SHA-256 Hashing, RSA-2048 PKI, TOTP
- **Database**: MySQL

## 📚 Referensi Akademik

Pengembangan sistem keamanan *Two-Factor Authentication* (2FA) dan implementasi *Public Key Infrastructure* (PKI) pada aplikasi ini merujuk pada pedoman dan literatur berikut:
* **Institut Teknologi Bandung (ITB)**: *[Masukkan judul spesifik paper/jurnal/tesis ITB di sini, misal: "Analisis dan Implementasi Tanda Tangan Digital Berbasis Algoritma RSA", STEI ITB]*
* Dokumentasi resmi [pdfcpu](https://pdfcpu.io/) untuk pemrosesan struktur PDF.
* Standar kriptografi publik untuk *hashing* (SHA-256) dan enkripsi asimetris.

## ⚙️ Panduan Instalasi dan Menjalankan Proyek

### 1. Persiapan Database
1. Buka MySQL/phpMyAdmin.
2. Buat database baru atau langsung *import* file `dgsign_db.sql` yang tersedia di repositori ini.

### 2. Menjalankan Backend (Golang)
Buka terminal dan arahkan ke folder `backend`:
```bash
cd backend
# Unduh semua dependensi library
go mod tidy
# Jalankan server
go run main.go
```


3. Menjalankan Frontend (React)
Buka terminal baru, arahkan ke folder frontend, lalu jalankan perintah berikut:
```bash
cd frontend
npm install
npm run dev
```

🔒 Alur Penggunaan
Registrasi & Login: Daftar akun baru menggunakan email.

Setup 2FA: Pindai QR Code di aplikasi untuk mengaktifkan Google Authenticator.

Request Digital ID: Lakukan pengajuan identitas digital untuk mendapatkan file .p12.

Sign Document: Unggah PDF, masukkan passphrase, dan lampirkan gambar tanda tangan.

Verifikasi: Unggah dokumen untuk memastikan keaslian dan mendeteksi apakah dokumen pernah diduplikasi.
