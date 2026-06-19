import React, { useState, useEffect } from 'react';
import { QRCodeCanvas } from 'qrcode.react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import axios from 'axios';
import SignDocument from './SignDocument';
import AdminDashboard from './AdminDasboard'; // IMPORT KOMPONEN ADMIN
import VerifyDocument from './VerifyDocument';
import Workflow from './Workflow';
import HistoryDocument from './HistoyDoc';
import SettingDocument from './SettingDocument';
// --- TEMA & STYLING MINIMALIS ---
const theme = {
  bg: '#f4f7f6',
  card: '#ffffff',
  primary: '#4F46E5', // Indigo modern
  text: '#1F2937',
  textMuted: '#6B7280',
  border: '#E5E7EB',
  radius: '12px',
  shadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)'
};

const styles = {
  container: { minHeight: '100vh', backgroundColor: theme.bg, display: 'flex', color: theme.text, fontFamily: '"Inter", sans-serif' },
  centerWrap: { flex: 1, display: 'flex', justifyContent: 'center', alignItems: 'center', padding: '20px' },
  card: { backgroundColor: theme.card, padding: '40px', borderRadius: theme.radius, boxShadow: theme.shadow, width: '100%', maxWidth: '400px' },
  input: { width: '100%', padding: '12px 16px', marginBottom: '15px', borderRadius: '8px', border: `1px solid ${theme.border}`, boxSizing: 'border-box', outline: 'none', fontSize: '15px' },
  btn: { width: '100%', padding: '12px', backgroundColor: theme.primary, color: 'white', border: 'none', borderRadius: '8px', fontSize: '16px', fontWeight: '600', cursor: 'pointer', transition: '0.2s' },
  btnOutline: { width: '100%', padding: '12px', backgroundColor: 'transparent', color: theme.text, border: `1px solid ${theme.border}`, borderRadius: '8px', cursor: 'pointer', marginTop: '10px' },
  sidebar: { width: '250px', backgroundColor: theme.card, borderRight: `1px solid ${theme.border}`, padding: '20px', display: 'flex', flexDirection: 'column' },
  content: { flex: 1, padding: '40px', overflowY: 'auto' },
  menuItem: (active) => ({ padding: '12px 16px', marginBottom: '8px', borderRadius: '8px', cursor: 'pointer', backgroundColor: active ? '#EEF2FF' : 'transparent', color: active ? theme.primary : theme.text, fontWeight: active ? '600' : '400' })
};

function MainApp() {
  // appState: 'AUTH' | 'SETUP_OTP' | 'VERIFY_OTP' | 'DASHBOARD'
  const [appState, setAppState] = useState('AUTH');
  const [authMode, setAuthMode] = useState('login');
  
  const [hasP12, setHasP12] = useState(false); // State untuk mengunci tombol
  const [extractData, setExtractData] = useState({ file: null, passphrase: '' });
  // Data Sesi
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [otpCode, setOtpCode] = useState('');
  const [qrUrl, setQrUrl] = useState('');
  const [role, setRole] = useState('user'); // STATE BARU: Menyimpan role pengguna
  
  // Data Dashboard
  const [activeMenu, setActiveMenu] = useState('upload');
  const [notification, setNotification] = useState('');
  const [historyList, setHistoryList] = useState([]);

  // State untuk Verifikasi Dokumen
  const [verifyFile, setVerifyFile] = useState(null);
  const [verifyResult, setVerifyResult] = useState(null);

  const [signaturePos, setSignaturePos] = useState({ page: 1, x: 100, y: 150 });

  const [docToSign, setDocToSign] = useState(null); // State untuk simpan data dokumen

  const PROTOCOL = window.location.protocol; 
  const API_BASE_URL = `${PROTOCOL}//dgsign.test:8081`;

  const isUnsecured = window.location.protocol === 'http:' || window.location.hostname === 'localhost';

// Fungsi ini akan dipanggil oleh Workflow.jsx
  const prepareSign = (wf) => {
    setDocToSign(wf); // Simpan data dokumen
    setActiveMenu('upload'); // Pindah tab
  };

  const handlePdfClick = (e) => {
    const rect = e.target.getBoundingClientRect();
    const clickX = e.clientX - rect.left; 
    const clickY = rect.bottom - e.clientY; 
    setSignaturePos({ ...signaturePos, x: Math.round(clickX), y: Math.round(clickY) });
  };

  const fetchHistory = async () => {
    try {
      const res = await axios.get(`${API_BASE_URL}/history?email=${email}`);
      setHistoryList(res.data || []);
    } catch (err) {
      console.error("Gagal mengambil riwayat:", err);
    }
  };

  const handleExtractCert = async (e) => {
    e.preventDefault();
    if (!extractData.file) return alert("Harap pilih file .p12!");
    const formData = new FormData();
    formData.append('p12_file', extractData.file);
    formData.append('passphrase', extractData.passphrase);
    try {
      const res = await axios.post(`${API_BASE_URL}extract-cert`, formData, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `${email}_Public.crt`);
      document.body.appendChild(link);
      link.click();
      alert("Sertifikat Publik (.crt) berhasil diekstrak!");
      setExtractData({ file: null, passphrase: '' });
      e.target.reset();
    } catch (err) {
      alert("Gagal mengekstrak sertifikat.");
    }
  };

  // FUNGSI 2: RESET
  const handleResetP12 = async () => {
    if (!window.confirm("Yakin ingin mereset Digital ID?")) return;
    try {
      const res = await axios.post(`${API_BASE_URL}/reset-p12`, { email });
      alert(res.data);
      setHasP12(false); 
    } catch (err) {
      alert("Gagal mereset: " + (err.response?.data || err.message));
    }
  };

  useEffect(() => {
    if (activeMenu === 'history' && email) {
      fetchHistory();
    }
  }, [activeMenu, email]);

  // Data Form 
  const [reqData, setReqData] = useState({ name: '', passphrase: '' });

// === 1. FUNGSI AUTENTIKASI ===
  const handleAuth = async (e) => {
    e.preventDefault();
    setNotification('');
    try {
      if (authMode === 'register') {
        await axios.post(`${API_BASE_URL}/register`, { email, password });
        setNotification('Registrasi sukses. Silakan masuk.');
        setAuthMode('login');
      } else {
        const res = await axios.post(`${API_BASE_URL}/login`, { email, password });

        console.log("DATA LOGIN DARI BACKEND:", res.data);
        
        // TANGKAP ROLE DARI BACKEND
        const userRole = res.data.role || 'user';
        setRole(userRole); 
        setHasP12(res.data.has_p12 || false);

        localStorage.setItem('email', email);      // Simpan email
        localStorage.setItem('role', userRole);    // Simpan role

        // ========================================================
        // LOGIKA BARU: BYPASS OTP KHUSUS UNTUK ADMIN
        // ========================================================
        if (userRole === 'admin') {
          setAppState('DASHBOARD'); // Langsung masuk sistem
          setActiveMenu('admin');
          return;                   // Hentikan proses agar tidak membaca OTP di bawahnya
        }

        // ========================================================

        // Jika dia adalah 'user' biasa, jalankan alur OTP seperti biasa
        if (res.data.requireSetup) {
          await generateQR();
          setAppState('SETUP_OTP');
        } else {
          setAppState('VERIFY_OTP');
        }
      }
    } catch (err) {
      setNotification(err.response?.data || "Terjadi kesalahan sistem.");
    }
  };

  // === 2. FUNGSI GATEWAY OTP ===
  const generateQR = async () => {
    const res = await axios.post(`${API_BASE_URL}/generate`, { email });
    setQrUrl(res.data.url);
  };

  const handleVerifyOTP = async (e) => {
    e.preventDefault();
    try {
      await axios.post(`${API_BASE_URL}/verify-otp`, { email, otpCode });
      setOtpCode('');
      setNotification('');
      setAppState('DASHBOARD'); // BERHASIL MASUK KE SISTEM UTAMA
    } catch (err) {
      setNotification("Kode OTP salah atau kedaluwarsa.");
    }
  };

  const [signData, setSignData] = useState({ 
    signerName: '', 
    passphrase: '', 
    otp: '', 
    file: null, 
    signatureImage: null 
  });

  const handlePdfUpload = (e) => {
    const selectedFile = e.target.files[0];
    if (selectedFile) {
      const fileSizeMB = selectedFile.size / (1024 * 1024);
      if (fileSizeMB > 5) {
        alert("Gagal: Ukuran dokumen PDF melebihi batas maksimal 5MB.");
        e.target.value = null; 
        setSignData({ ...signData, file: null });
      } else {
        setSignData({ ...signData, file: selectedFile });
      }
    }
  };

  const handleDownload = async (id, docName, status, token) => {
    // BLOKIR DOWNLOAD JIKA STATUS BELUM DISETUJUI
    if (status !== undefined && status !== 'disetujui') {
        alert(`Dokumen tidak dapat diunduh. Status saat ini: ${status.toUpperCase()}`);
        return;
    }

    try {
      const response = await axios.get(`${API_BASE_URL}/download?token=${token}`, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `${docName}`); 
      document.body.appendChild(link);
      link.click();
    } catch (err) {
      alert("Gagal mengunduh dokumen. File mungkin tidak ditemukan di server atau belum disetujui.");
    }
  };

  const handleVerifyDocument = async () => {
    if (!verifyFile) {
      alert("Harap pilih dokumen PDF terlebih dahulu!");
      return;
    }
    const formData = new FormData();
    formData.append('file', verifyFile);

    try {
      const res = await axios.post(`${API_BASE_URL}/verify-pdf`, formData);
      setVerifyResult(res.data);
    } catch (err) {
      alert(err.response?.data || "Dokumen tidak valid.");
      setVerifyResult(null); 
    }
  };

  const handleSignDocument = async (e) => {
    e.preventDefault();
    const token = localStorage.getItem('token');
    const formData = new FormData();
    formData.append('email', email);
    formData.append('otpCode', signData.otp);
    formData.append('passphrase', signData.passphrase);
    formData.append('signerName', signData.signerName);

    formData.append("page", signaturePos.page); 
    formData.append("x", signaturePos.x);
    formData.append("y", signaturePos.y);
    
    if (signData.file) formData.append('file', signData.file);
    if (signData.signatureImage) formData.append('signatureImage', signData.signatureImage);

    try {
      const res = await axios.post(`${API_BASE_URL}/web-sign`, formData, {
        headers: { 
          'Content-Type': 'multipart/form-data' ,
          'Authorization': `Bearer ${token}`
        }
      });
      alert(res.data);
      setSignData({ signerName: '', passphrase: '', otp: '', file: null, signatureImage: null });
      e.target.reset();
    } catch (err) { 
      alert(err.response?.data || "Gagal melakukan tanda tangan"); 
    }
  };

  const handleRequestID = async (e) => {
      e.preventDefault();
      try {
        const response = await axios.post(`${API_BASE_URL}/request-id`, { ...reqData, email }, { responseType: 'blob' });
        
        const link = document.createElement('a');
        link.href = window.URL.createObjectURL(new Blob([response.data]));
        link.setAttribute('download', `${email}_digital_id.p12`);
        document.body.appendChild(link); 
        link.click();
        
        alert("Digital ID (.p12) berhasil dibuat dan diunduh. Anda sekarang dapat menggunakannya di platform ini maupun di-import ke Adobe Acrobat.");
        setHasP12(true); // Set state untuk mengunci tombol setelah berhasil membuat ID
      } catch (err) { 
        const errorText = await err.response?.data.text();
        alert(errorText || "Gagal membuat ID."); 
      }
    };

  if (appState === 'AUTH') {
    return (
      <div style={styles.container}>
        <div style={styles.centerWrap}>
          <div style={styles.card}>
            <h2 style={{ textAlign: 'center', marginBottom: '10px'}}>DGSign Portal</h2>
            <p style={{ textAlign: 'center', color: theme.textMuted, marginBottom: '30px', fontSize: '14px' }}>Sistem Tanda Tangan Digital Terpadu</p>
            
            <form onSubmit={handleAuth}>
              <input style={styles.input} type="email" placeholder="Alamat Email" required value={email} onChange={e => setEmail(e.target.value)} />
              <input style={styles.input} type="password" placeholder="Kata Sandi" required value={password} onChange={e => setPassword(e.target.value)} />
              <button style={styles.btn} type="submit">{authMode === 'login' ? 'Masuk' : 'Buat Akun'}</button>
            </form>
            
            <div style={{ textAlign: 'center', marginTop: '15px', color: theme.textMuted, fontSize: '14px' }}>
              {authMode === 'login' ? (
                <span>Belum punya akun? <span style={{ cursor: 'pointer', color: theme.primary }} onClick={() => setAuthMode('register')}>Daftar</span></span>
              ) : (
                <span>Sudah punya akun? <span style={{ cursor: 'pointer', color: theme.primary }} onClick={() => setAuthMode('login')}>Masuk</span></span>
              )}
            </div>
            {notification && <p style={{ color: '#E11D48', textAlign: 'center', marginTop: '15px', fontSize: '14px' }}>{notification}</p>}
          </div>
        </div>
      </div>
    );
  }

  if (appState === 'SETUP_OTP') {
    return (
      <div style={styles.container}>
        <div style={styles.centerWrap}>
          <div style={{ ...styles.card, textAlign: 'center' }}>
            <h2>Setup Keamanan 2FA</h2>
            <p style={{ color: theme.textMuted, fontSize: '14px', marginBottom: '20px' }}>Pindai QR Code di bawah menggunakan aplikasi Google Authenticator untuk mengamankan akun Anda.</p>
            {qrUrl && <QRCodeCanvas value={qrUrl} size={180} style={{ padding: '10px', border: `1px solid ${theme.border}`, borderRadius: '10px', marginBottom: '20px' }} />}
            
            <form onSubmit={handleVerifyOTP}>
              <input style={{ ...styles.input, textAlign: 'center', letterSpacing: '4px', fontSize: '20px' }} type="text" maxLength="6" placeholder="------" required value={otpCode} onChange={e => setOtpCode(e.target.value)} />
              <button style={styles.btn} type="submit">Verifikasi & Lanjutkan</button>
            </form>
            {notification && <p style={{ color: '#E11D48', marginTop: '15px', fontSize: '14px' }}>{notification}</p>}
          </div>
        </div>
      </div>
    );
  }

  if (appState === 'VERIFY_OTP') {
    return (
      <div style={styles.container}>
        <div style={styles.centerWrap}>
          <div style={styles.card}>
            <h2 style={{ textAlign: 'center' }}>Verifikasi OTP</h2>
            <p style={{ textAlign: 'center', color: theme.textMuted, fontSize: '14px', marginBottom: '30px' }}>Masukkan 6 digit kode dari aplikasi Authenticator Anda.</p>
            <form onSubmit={handleVerifyOTP}>
              <input style={{ ...styles.input, textAlign: 'center', letterSpacing: '4px', fontSize: '24px' }} type="text" maxLength="6" placeholder="------" required value={otpCode} onChange={e => setOtpCode(e.target.value)} />
              <button style={styles.btn} type="submit">Buka Dashboard</button>
            </form>
            <button style={styles.btnOutline} onClick={() => setAppState('AUTH')}>Kembali ke Login</button>
            {notification && <p style={{ color: '#E11D48', textAlign: 'center', marginTop: '15px', fontSize: '14px' }}>{notification}</p>}
          </div>
        </div>
      </div>
    );
  }

  // TAMPILAN 4: MAIN DASHBOARD
  return (
    <div style={styles.container}>
{/* SIDEBAR */}
        <div style={styles.sidebar}>
          <h2 style={{ color: theme.primary, marginBottom: '30px', paddingLeft: '10px' }}>DGSign</h2>
          
          <div style={{ flex: 1 }}>
            {/* Menu untuk SEMUA (Mahasiswa, Dosen, Admin) */}
            <div style={styles.menuItem(activeMenu === 'upload')} onClick={() => setActiveMenu('upload')}>✍️ Digital Signature</div>
            
            {/* MENU DINAMIS BERDASARKAN ROLE */}
            {(role === 'user') && (
                <div style={styles.menuItem(activeMenu === 'workflow')} onClick={() => setActiveMenu('workflow')}>🔄 Pengajuan Saya</div>
            )}
            
            {(role === 'dosen' || role === 'admin') && (
                <div style={styles.menuItem(activeMenu === 'workflow')} onClick={() => setActiveMenu('workflow')}>📥 Dokumen Masuk</div>
            )}

            <div style={styles.menuItem(activeMenu === 'history')} onClick={() => setActiveMenu('history')}>🗂️ Riwayat</div>
            <div style={styles.menuItem(activeMenu === 'verify')} onClick={() => setActiveMenu('verify')}>✅ Verifikasi</div>
            
            <div style={{ margin: '20px 0', borderBottom: `1px solid ${theme.border}` }}></div>
            <div style={styles.menuItem(activeMenu === 'settings')} onClick={() => setActiveMenu('settings')}>⚙️ Pengaturan</div>

            {role === 'admin' && (
              <div style={{ ...styles.menuItem(activeMenu === 'admin'), color: '#E11D48' }} onClick={() => setActiveMenu('admin')}>🛡️ Admin Dashboard</div>
            )}
          </div>
        
        <div style={{ padding: '15px', backgroundColor: '#F9FAFB', borderRadius: '8px', fontSize: '13px', color: theme.textMuted }}>
          Login sebagai:<br/><b style={{ color: theme.text }}>{email}</b>
            <button style={{ ...styles.btnOutline, padding: '8px', marginTop: '15px' }} onClick={() => { 
              // 1. Kembalikan ke halaman login
              setAppState('AUTH'); 
              
              // 2. Bersihkan data sesi pengguna
              setEmail(''); 
              setPassword(''); 
              setRole('user'); 
              
              // 3. RESET STATE .P12 (Ini yang menghilangkan tombol ekstrak/reset)
              setHasP12(false); 
              setExtractData({ file: null, passphrase: '' }); 
              
              // 4. Kembalikan menu aktif ke default agar rapi
              setActiveMenu('upload'); 
              
              // (Opsional) Jika Anda menyimpan token di localStorage, hapus juga di sini:
              // localStorage.removeItem('token');
              localStorage.removeItem('email');
              localStorage.removeItem('role');
            }}>
              Keluar
            </button>        
          </div>
      </div>

      {/* CONTENT AREA */}
      <div style={styles.content}>
        <div style={{ minWidth: 'fit-content', width: '95%', backgroundColor: theme.card, padding: '40px', borderRadius: theme.radius, boxShadow: theme.shadow }}>
          
          {/* MENU 1: SIGN/UPLOAD DOCUMENT */}
          {activeMenu === 'upload' && role !== 'mahasiswa' && (
            <SignDocument userEmail={email} docToSign={docToSign} clearDoc={() => setDocToSign(null)} />
          )}

          {activeMenu === 'workflow' && <Workflow email={email} role={role} onPrepareSign={prepareSign} />}

          {/* MENU 2: RIWAYAT DOCUMENT */}
          {activeMenu === 'history' && (
            <HistoryDocument userEmail={email} />
          )}

          {/* MENU 3: VERIFIKASI DOCUMENT */}
          {activeMenu === 'verify' && (
            <div>
              <h2 style={{ marginBottom: '10px' }}>Verifikasi Document</h2>
              
              <div style={{ padding: '30px', backgroundColor: '#F9FAFB', borderRadius: '8px', border: `1px solid ${theme.border}` }}>
                <input 
                  type="file" 
                  accept="application/pdf" 
                  onChange={(e) => setVerifyFile(e.target.files[0])}
                  style={{ ...styles.input, padding: '8px', backgroundColor: 'white' }} 
                />
                <button 
                  onClick={handleVerifyDocument}
                  style={{ ...styles.btn, backgroundColor: theme.text, width: 'auto', padding: '10px 25px' }}
                >
                  Periksa Validitas
                </button>
              </div>
              {/* HASIL VERIFIKASI DOKUMEN */}
              {verifyResult && (
                <div style={{ marginTop: '30px' }}>
                  
                  {/* KOTAK STATUS */}
                  <div style={{ padding: '15px', backgroundColor: verifyResult.isValid ? '#ECFDF5' : '#FEF2F2', border: `1px solid ${verifyResult.isValid ? '#10B981' : '#EF4444'}`, borderRadius: '8px', marginBottom: '20px' }}>
                    <h3 style={{ color: verifyResult.isValid ? '#047857' : '#991B1B', margin: 0, display: 'flex', alignItems: 'center', gap: '10px' }}>
                      {verifyResult.isValid ? '✅' : '❌'} {verifyResult.message}
                    </h3>
                  </div>

                  {/* KARTU IDENTITAS SERTIFIKAT DIGITAL (.p12) */}
                  {verifyResult.certificate && (
                    <div style={{ backgroundColor: 'white', border: `1px solid ${theme.border}`, borderRadius: '8px', overflow: 'hidden', boxShadow: theme.shadow }}>
                      
                      <div style={{ backgroundColor: '#F3F4F6', padding: '12px 20px', borderBottom: `1px solid ${theme.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <strong style={{ color: '#374151', fontSize: '15px' }}>📜 Detail Sertifikat Kriptografi</strong>
                        <span style={{ fontSize: '12px', backgroundColor: '#D1FAE5', color: '#065F46', padding: '2px 8px', borderRadius: '10px', fontWeight: 'bold' }}>Terverifikasi</span>
                      </div>

                      <div style={{ padding: '20px', display: 'grid', gridTemplateColumns: '130px 1fr', gap: '12px 10px', fontSize: '14px' }}>
                        <div style={{ color: theme.textMuted }}>Penandatangan</div>
                        <div style={{ fontWeight: 'bold', color: theme.primary, fontSize: '16px' }}>
                          {verifyResult.certificate.signerName}
                        </div>

                        <div style={{ color: theme.textMuted }}>Penerbit (Issuer)</div>
                        <div style={{ color: '#4B5563' }}>{verifyResult.certificate.issuerName}</div>

                        <div style={{ color: theme.textMuted }}>Waktu TTD</div>
                        <div style={{ color: '#4B5563' }}>{new Date(verifyResult.certificate.signedAt).toLocaleString('id-ID')}</div>

                        <div style={{ color: theme.textMuted }}>Valid Hingga</div>
                        <div style={{ color: '#B45309', fontWeight: '500' }}>
                          {new Date(verifyResult.certificate.validUntil).toLocaleString('id-ID')}
                        </div>

                        <div style={{ color: theme.textMuted }}>Integritas Dokumen</div>
                        <div style={{ color: '#059669' }}>
                          Hash SHA-256 Cocok. Tidak ada modifikasi sejak ditandatangani.
                        </div>
                      </div>
                      
                    </div>
                  )}

                </div>
              )}
            </div>
          )}

          {/* MENU 4: PENGATURAN & REQUEST ID */}
          {activeMenu === 'settings' && (
            <SettingDocument 
              email={email} 
              role={role} 
              setAppState={setAppState} 
            />
          )}

        {/* MENU 5: DASHBOARD ADMIN (SEKARANG BEBAS DI LUAR KOTAK PUTIH!) */}
        {activeMenu === 'admin' && role === 'admin' && (
          <AdminDashboard />
        )}
        </div> 
      </div> {/* <--- Ini penutup styles.content, biarkan di sini */}
    </div>
  );
}

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<MainApp />} />
        <Route path="/verify/:uuid" element={<VerifyDocument />} />
        <Route path="*" element={<MainApp />} />
      </Routes>
    </Router>
  );
}

// SATU-SATUNYA baris export default harus ada di paling bawah ini
export default App;