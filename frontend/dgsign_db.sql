-- =====================================================
-- DATABASE
-- =====================================================

DROP DATABASE IF EXISTS dgsign_db;
CREATE DATABASE dgsign_db CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
USE dgsign_db;

-- =====================================================
-- USERS
-- =====================================================

CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255) DEFAULT NULL,
    otp_secret VARCHAR(100) DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    role VARCHAR(20) DEFAULT 'user',
    has_p12 TINYINT(1) DEFAULT 0,
    p12_path VARCHAR(255) NOT NULL
) ENGINE=InnoDB;

-- =====================================================
-- DOCUMENT WORKFLOW
-- =====================================================

CREATE TABLE document_workflows (
    id INT AUTO_INCREMENT PRIMARY KEY,
    mahasiswa_email VARCHAR(255) NOT NULL,
    dosen_email VARCHAR(255) NOT NULL,
    document_name VARCHAR(255) NOT NULL,
    original_file_path VARCHAR(500) NOT NULL,
    signed_file_path VARCHAR(500) DEFAULT NULL,
    status VARCHAR(50) DEFAULT 'menunggu_dosen',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    file_path VARCHAR(255) DEFAULT NULL
) ENGINE=InnoDB;

-- =====================================================
-- DOCUMENT HISTORY
-- =====================================================

CREATE TABLE document_history (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    document_name VARCHAR(255) NOT NULL,
    signer_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(255) NOT NULL,
    signed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) DEFAULT 'menunggu',
    file_token VARCHAR(64),
    is_active TINYINT(1) DEFAULT 1
) ENGINE=InnoDB;

-- =====================================================
-- SIGNED DOCUMENTS
-- =====================================================

CREATE TABLE signed_documents (

    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    uuid VARCHAR(128) NOT NULL UNIQUE,

    user_id BIGINT NOT NULL,

    filename VARCHAR(255) NOT NULL,

    file_path VARCHAR(500) NOT NULL,

    hash_sha256 VARCHAR(64) NOT NULL,

    certificate_serial VARCHAR(128),

    signer_email VARCHAR(255),

    signed_at DATETIME NOT NULL,

    verification_token VARCHAR(128),

    created_at DATETIME NOT NULL,

    updated_at DATETIME NOT NULL,

    signer_name VARCHAR(255),

    issuer_name VARCHAR(255),

    valid_until DATETIME,

    scan_count INT DEFAULT 0,

    ------------------------------------------------------
    -- FITUR BARU
    ------------------------------------------------------

    workflow_id INT DEFAULT NULL,

    signature_version INT DEFAULT 1,

    signature_status ENUM('ACTIVE','REVOKED')
    DEFAULT 'ACTIVE',

    previous_signature_uuid VARCHAR(128) DEFAULT NULL,

    INDEX(user_id),
    INDEX(workflow_id),
    INDEX(signature_status),
    INDEX(verification_token),

    CONSTRAINT fk_user
    FOREIGN KEY(user_id)
    REFERENCES users(id)
    ON DELETE CASCADE,

    CONSTRAINT fk_workflow
    FOREIGN KEY(workflow_id)
    REFERENCES document_workflows(id)
    ON DELETE SET NULL

) ENGINE=InnoDB;