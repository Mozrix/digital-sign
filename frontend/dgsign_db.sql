CREATE DATABASE IF NOT EXISTS dgsign_db;
USE dgsign_db;

-- Tabel untuk menyimpan data pengguna dan otentikasi (Termasuk OTP)
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    otp_secret VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tabel untuk menyimpan riwayat dokumen yang telah ditandatangani
CREATE TABLE IF NOT EXISTS document_history (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    document_name VARCHAR(255) NOT NULL,
    signer_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(255) NOT NULL,
    signed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);