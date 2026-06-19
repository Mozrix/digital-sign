import React, { useState, useEffect } from 'react';
import axios from 'axios';

// Tema Warna Konsisten
const theme = {
  primary: '#4F46E5',
  success: '#10B981',
  danger: '#EF4444',
  textMuted: '#6B7280',
  border: '#E5E7EB',
};

// Desain Style Objek agar tidak error undefined
const styles = {
  input: {
    width: '100%',
    padding: '10px',
    marginBottom: '12px',
    borderRadius: '6px',
    border: '1px solid #ccc',
    boxSizing: 'border-box',
    fontSize: '14px'
  },
  btn: {
    padding: '10px 20px',
    color: 'white',
    border: 'none',
    borderRadius: '6px',
    fontWeight: 'bold',
    cursor: 'pointer',
    fontSize: '14px'
  },
  btnOutline: {
    padding: '10px 20px',
    backgroundColor: 'transparent',
    border: '1px solid',
    borderRadius: '6px',
    fontWeight: 'bold',
    cursor: 'pointer',
    fontSize: '14px'
  }
};

const SettingDocument = ({ email, role, setAppState }) => {
  const [hasP12, setHasP12] = useState(false);
  const [reqData, setReqData] = useState({ name: '', passphrase: '' });
  const [extractData, setExtractData] = useState({ file: null, passphrase: '' });

  const PROTOCOL = window.location.protocol; 
  const API_BASE_URL = `${PROTOCOL}//dgsign.test:8081`;

  const isUnsecured = window.location.protocol === 'http:' || window.location.hostname === 'localhost';

  // Ambil status apakah user sudah punya P12 atau belum saat komponen dimuat
  useEffect(() => {
    if (email) {
      checkP12Status();
    }
  }, [email]);

  const checkP12Status = async () => {
    try {
      // Endpoint opsional untuk mengecek status p12 user aktif
      const res = await axios.get(`${API_BASE_URL}/user/status?email=${email}`);
      setHasP12(res.data.has_p12);
    } catch (err) {
      console.error("Gagal memuat status Digital ID:", err);
    }
  };

  // 1. FUNGSI UNTUK MAHASISWA / DOSEN MEMBUAT DIGITAL ID (.P12) Baru
  const handleRequestID = async (e) => {
    e.preventDefault();
    try {
      const res = await axios.post(`${API_BASE_URL}/request-id`, {
        email: email,
        name: reqData.name,
        passphrase: reqData.passphrase
      }, { responseType: 'blob' }); // Unduh file langsung sebagai blob

      // Logic otomatis download file .p12 ke komputer user
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `digital_id_${email}.p12`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);

      alert("Sertifikat Digital ID (.p12) Anda berhasil dibuat dan diunduh! Simpan file ini baik-baik.");
      setHasP12(true);
    } catch (err) {
      alert("Gagal membuat Digital ID baru.");
    }
  };

  // 2. FUNGSI EKSTRAK CERT .CRT
  const handleExtractCert = async (e) => {
    e.preventDefault();
    if (!extractData.file) return alert("Pilih file .p12 terlebih dahulu!");
    
    const formData = new FormData();
    formData.append('file', extractData.file);
    formData.append('passphrase', extractData.passphrase);

    try {
      const res = await axios.post(`${API_BASE_URL}/extract-cert`, formData, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'certificate.crt');
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      alert(".crt Berhasil diekstrak dan diunduh!");
    } catch (err) {
      alert("Gagal mengekstrak sertifikat. Pastikan passphrase benar.");
    }
  };

  // 3. FUNGSI RESET DIGITAL ID
  const handleResetP12 = async () => {
    if (window.confirm('Apakah Anda yakin ingin mereset Digital ID? Sertifikat lama Anda tidak akan bisa digunakan lagi.')) {
      try {
        await axios.post(`${API_BASE_URL}/reset-p12`, { email });
        alert("Digital ID berhasil direset. Silakan buat ulang sertifikat baru.");
        setHasP12(false);
      } catch (err) {
        alert("Gagal mereset Digital ID");
      }
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: '10px' }}>Pengaturan Identitas Digital</h2>
      <p style={{ color: theme.textMuted, marginBottom: '30px' }}>Kelola sertifikat dan otentikasi akun Anda.</p>
      
      <div style={{ marginBottom: '40px' }}>
        <h3 style={{ fontSize: '16px', marginBottom: '15px' }}>1. Kelola Sertifikat Digital (.p12)</h3>
        
        {hasP12 ? (
          /* TAMPILAN JIKA USER SUDAH PERNAH MEMBUAT .P12 */
          <div>
            <div style={{ padding: '15px', backgroundColor: '#ECFDF5', border: '1px solid #10B981', borderRadius: '8px', color: '#047857', marginBottom: '20px', fontSize: '14px' }}>
              ✅ Anda sudah memiliki Digital ID aktif. Untuk menjaga integritas tanda tangan, Anda hanya diizinkan memiliki 1 sertifikat aktif per akun.
            </div>

            {/* FITUR EKSTRAK .CRT */}
            <div style={{ backgroundColor: '#F9FAFB', padding: '20px', borderRadius: '8px', border: `1px solid ${theme.border}`, marginBottom: '20px' }}>
              <h4 style={{ marginTop: 0, marginBottom: '10px', fontSize: '15px' }}>🛠️ Ekstrak Sertifikat Publik (.crt)</h4>
              <p style={{ fontSize: '13px', color: theme.textMuted, marginBottom: '15px' }}>Unggah file .p12 Anda untuk mengambil sertifikat publiknya saja. File .crt ini aman dibagikan ke orang lain.</p>
              
              <form onSubmit={handleExtractCert}>
                <input type="file" accept=".p12,.pfx" required style={{ ...styles.input, backgroundColor: 'white' }} onChange={e => setExtractData({...extractData, file: e.target.files[0]})} />
                <input type="password" placeholder="Passphrase file .p12 Anda" required style={styles.input} onChange={e => setExtractData({...extractData, passphrase: e.target.value})} />
              <button 
                onClick={handleResetP12} 
                disabled={isUnsecured}
                style={{ 
                  ...styles.btn, 
                  backgroundColor: isUnsecured ? '#ccc' : theme.danger, 
                  width: 'auto', 
                  padding: '10px 25px', 
                  fontSize: '14px',
                  cursor: isUnsecured ? 'not-allowed' : 'pointer'
                }}
              >
                {isUnsecured ? '🔒 Terkunci' : 'Reset Digital ID Saya'}
              </button>              </form>
            </div>

            {/* TOMBOL RESET .P12 */}
            <div style={{ backgroundColor: '#FEF2F2', padding: '20px', borderRadius: '8px', border: '1px solid #FCA5A5' }}>
              <h4 style={{ marginTop: 0, marginBottom: '10px', color: '#991B1B', fontSize: '15px' }}>Reset Digital ID</h4>
              <p style={{ fontSize: '13px', color: '#7F1D1D', marginBottom: '15px' }}>Kehilangan file .p12 atau lupa passphrase Anda? Klik tombol di bawah untuk menghapus akses sertifikat lama dan membuat sertifikat yang baru.</p>
              <button 
                onClick={handleResetP12} 
                disabled={isUnsecured}
                style={{ 
                  ...styles.btn, 
                  backgroundColor: isUnsecured ? '#ccc' : theme.danger, 
                  width: 'auto', 
                  padding: '10px 25px', 
                  fontSize: '14px',
                  cursor: isUnsecured ? 'not-allowed' : 'pointer'
                }}
              >
                {isUnsecured ? '🔒 Terkunci' : 'Reset Digital ID Saya'}
              </button>
            </div>
          </div>
        ) : (
          /* TAMPILAN AWAL: JIKA MAHASISWA/USER BELUM PUNYA PASSPHRASE & SERTIFIKAT */
            <form onSubmit={handleRequestID}>
            <p style={{ fontSize: '14px', color: theme.textMuted, marginBottom: '15px' }}>
              Anda belum memiliki sertifikat. Silakan buat sertifikat Digital ID Anda terlebih dahulu dengan mengisi form passphrase di bawah ini.
            </p>
            <input type="text" placeholder="Nama Lengkap" required style={styles.input} value={reqData.name} onChange={e => setReqData({...reqData, name: e.target.value})} disabled={isUnsecured} />
            <input type="password" placeholder="Passphrase Baru untuk Kunci Digital ID Anda" required style={styles.input} value={reqData.passphrase} onChange={e => setReqData({...reqData, passphrase: e.target.value})} disabled={isUnsecured} />
            
            {/* TOMBOL GENERATE DIKUNCI JIKA HTTP */}
            <button 
              type="submit" 
              disabled={isUnsecured}
              style={{ 
                ...styles.btn, 
                backgroundColor: isUnsecured ? '#ccc' : theme.success, 
                width: 'auto', 
                padding: '10px 25px',
                cursor: isUnsecured ? 'not-allowed' : 'pointer'
              }}
            >
              {isUnsecured ? '🔒 Terkunci' : 'Request & Download .p12'}
            </button>
          </form>
        )}
      </div>

      {/* MENU 2FA AUTHENTICATOR */}
      <div style={{ borderTop: `1px solid ${theme.border}`, paddingTop: '30px' }}>
        <h3 style={{ fontSize: '16px', marginBottom: '15px' }}>2. Reset Perangkat Authenticator</h3>
        <p style={{ fontSize: '14px', color: theme.textMuted, marginBottom: '15px' }}>Gunakan ini jika Anda kehilangan akses ke perangkat <i>smartphone</i> Anda.</p>
        <button 
          style={{ ...styles.btnOutline, borderColor: '#E11D48', color: '#E11D48', width: 'auto', padding: '10px 25px', marginTop: 0 }} 
          onClick={async () => {
            if(window.confirm('Reset OTP sekarang?')) {
              await axios.post(`${API_BASE_URL}/reset-otp`, { email });
              if (setAppState) {
                setAppState('AUTH');
              }
              alert("OTP direset. Silakan login kembali untuk setup ulang perangkat.");
            }
          }}
        >
          Reset Keamanan 2FA
        </button>
      </div>
    </div>
  );
};

export default SettingDocument;