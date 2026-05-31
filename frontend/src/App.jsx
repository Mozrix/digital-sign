import React, { useState, useEffect } from 'react';
import { QRCodeCanvas } from 'qrcode.react';
import axios from 'axios';
import SignDocument from './SignDocument';

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

function App() {
  // appState: 'AUTH' | 'SETUP_OTP' | 'VERIFY_OTP' | 'DASHBOARD'
  const [appState, setAppState] = useState('AUTH');
  const [authMode, setAuthMode] = useState('login');
  
  // Data Sesi
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [otpCode, setOtpCode] = useState('');
  const [qrUrl, setQrUrl] = useState('');
  
  // Data Dashboard
  const [activeMenu, setActiveMenu] = useState('upload');
  const [notification, setNotification] = useState('');
  const [historyList, setHistoryList] = useState([]);

  // State untuk Verifikasi Dokumen
  const [verifyFile, setVerifyFile] = useState(null);
  const [verifyResult, setVerifyResult] = useState(null);

  const [signaturePos, setSignaturePos] = useState({ page: 1, x: 100, y: 150 });

  const handlePdfClick = (e) => {
  const rect = e.target.getBoundingClientRect();
  const clickX = e.clientX - rect.left; // Koordinat X lokal
  const clickY = rect.bottom - e.clientY; // Koordinat Y (dibalik karena pdfcpu membaca dari bawah-kiri)

  setSignaturePos({ ...signaturePos, x: Math.round(clickX), y: Math.round(clickY) });
};

  const fetchHistory = async () => {
    try {
      const res = await axios.get(`http://localhost:8081/history?email=${email}`);
      setHistoryList(res.data || []);
    } catch (err) {
      console.error("Gagal mengambil riwayat:", err);
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
        await axios.post('http://localhost:8081/register', { email, password });
        setNotification('Registrasi sukses. Silakan masuk.');
        setAuthMode('login');
      } else {
        const res = await axios.post('http://localhost:8081/login', { email, password });
        // Cek apakah user perlu setup OTP (pengguna baru) atau langsung verifikasi OTP (pengguna lama)
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
    const res = await axios.post('http://localhost:8081/generate', { email });
    setQrUrl(res.data.url);
  };

  const handleVerifyOTP = async (e) => {
    e.preventDefault();
    try {
      await axios.post('http://localhost:8081/verify-otp', { email, otpCode });
      setOtpCode('');
      setNotification('');
      setAppState('DASHBOARD'); // BERHASIL MASUK KE SISTEM UTAMA
    } catch (err) {
      setNotification("Kode OTP salah atau kedaluwarsa.");
    }
  };

  // Tambahkan state baru atau perbarui state signData sebelumnya
  const [signData, setSignData] = useState({ 
    signerName: '', 
    passphrase: '', 
    otp: '', 
    file: null, 
    signatureImage: null 
  });

  // Validasi ukuran PDF maksimal 5MB
  const handlePdfUpload = (e) => {
    const selectedFile = e.target.files[0];
    if (selectedFile) {
      const fileSizeMB = selectedFile.size / (1024 * 1024);
      if (fileSizeMB > 5) {
        alert("Gagal: Ukuran dokumen PDF melebihi batas maksimal 5MB.");
        e.target.value = null; // Reset input file
        setSignData({ ...signData, file: null });
      } else {
        setSignData({ ...signData, file: selectedFile });
      }
    }
  };

  const handleDownload = async (id, docName) => {
    try {
      const response = await axios.get(`http://localhost:8081/download?id=${id}`, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `Signed_${docName}`); // Memberikan awalan Signed_ pada file
      document.body.appendChild(link);
      link.click();
    } catch (err) {
      alert("Gagal mengunduh dokumen. File mungkin tidak ditemukan di server.");
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
      const res = await axios.post('http://localhost:8081/verify-pdf', formData);
      setVerifyResult(res.data);
    } catch (err) {
      alert(err.response?.data || "Dokumen tidak valid.");
      setVerifyResult(null); // Reset hasil jika gagal
    }
  };

  const handleSignDocument = async (e) => {
    e.preventDefault();
    const formData = new FormData();
    formData.append('email', email);
    formData.append('otpCode', signData.otp);
    formData.append('passphrase', signData.passphrase);
    formData.append('signerName', signData.signerName);

    formData.append("page", signaturePos.page); 
    formData.append("x", signaturePos.x);
    formData.append("y", signaturePos.y);
    
    // Append File Wajib
    if (signData.file) {
      formData.append('file', signData.file);
    }
    // Append Gambar Opsional
    if (signData.signatureImage) {
      formData.append('signatureImage', signData.signatureImage);
    }

    try {
      const res = await axios.post('http://localhost:8081/web-sign', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      });
      alert(res.data);
      // Reset form setelah berhasil
      setSignData({ signerName: '', passphrase: '', otp: '', file: null, signatureImage: null });
      e.target.reset();
    } catch (err) { 
      alert(err.response?.data || "Gagal melakukan tanda tangan"); 
    }
  };

  const handleRequestID = async (e) => {
      e.preventDefault();
      try {
        const response = await axios.post('http://localhost:8081/request-id', { ...reqData, email }, { responseType: 'blob' });
        
        const link = document.createElement('a');
        link.href = window.URL.createObjectURL(new Blob([response.data]));
        link.setAttribute('download', `${email}_digital_id.p12`);
        document.body.appendChild(link); 
        link.click();
        
        alert("Digital ID (.p12) berhasil dibuat dan diunduh. Anda sekarang dapat menggunakannya di platform ini maupun di-import ke Adobe Acrobat.");
      } catch (err) { 
        // KODE BARU: Membaca teks error blob dari backend
        const errorText = await err.response?.data.text();
        alert(errorText || "Gagal membuat ID."); 
      }
    };
  // ==========================================
  // RENDER TAMPILAN
  // ==========================================

  // TAMPILAN 1: LOGIN / REGISTER
  if (appState === 'AUTH') {
    return (
      <div style={styles.container}>
        <div style={styles.centerWrap}>
          <div style={styles.card}>
            <h2 style={{ textAlign: 'center', marginBottom: '10px'}}>DGSign Portal</h2>
            <p style={{ textAlign: 'center', color: theme.textMuted, marginBottom: '30px', fontSize: '14px' }}>Sistem Tanda Tangan Digital Terpadu</p>
            
            <form onSubmit={handleAuth}>
              <input style={styles.input} type="email" placeholder="Alamat Email (mis: nama@unila.ac.id)" required value={email} onChange={e => setEmail(e.target.value)} />
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

  // TAMPILAN 2: GATEWAY SETUP OTP (Khusus Pengguna Baru)
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

  // TAMPILAN 3: GATEWAY VERIFIKASI OTP (Saat Login Normal)
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
          <div style={styles.menuItem(activeMenu === 'upload')} onClick={() => setActiveMenu('upload')}>✍️ Sign Document</div>
          <div style={styles.menuItem(activeMenu === 'history')} onClick={() => setActiveMenu('history')}>🗂️ Riwayat Document</div>
          <div style={styles.menuItem(activeMenu === 'verify')} onClick={() => setActiveMenu('verify')}>✅ Verifikasi Document</div>
          <div style={{ margin: '20px 0', borderBottom: `1px solid ${theme.border}` }}></div>
          <div style={styles.menuItem(activeMenu === 'settings')} onClick={() => setActiveMenu('settings')}>⚙️ Pengaturan ID</div>
        </div>

        <div style={{ padding: '15px', backgroundColor: '#F9FAFB', borderRadius: '8px', fontSize: '13px', color: theme.textMuted }}>
          Login sebagai:<br/><b style={{ color: theme.text }}>{email}</b>
          <button style={{ ...styles.btnOutline, padding: '8px', marginTop: '15px' }} onClick={() => { setAppState('AUTH'); setEmail(''); setPassword(''); }}>Keluar</button>
        </div>
      </div>

      {/* CONTENT AREA */}
      <div style={styles.content}>
        <div style={{ maxWidth: '700px', backgroundColor: theme.card, padding: '40px', borderRadius: theme.radius, boxShadow: theme.shadow }}>
          
          {/* MENU 1: SIGN/UPLOAD DOCUMENT */}
          {activeMenu === 'upload' && (
            <SignDocument userEmail={email} />
          )}

        {/* MENU 2: RIWAYAT DOCUMENT */}
          {activeMenu === 'history' && (
            <div>
              <h2 style={{ marginBottom: '10px' }}>Riwayat Document</h2>
              <p style={{ color: theme.textMuted, marginBottom: '30px' }}>Daftar dokumen yang telah Anda tandatangani secara digital.</p>
              
              {historyList.length === 0 ? (
                <div style={{ padding: '40px', textAlign: 'center', border: `2px dashed ${theme.border}`, borderRadius: '8px', color: theme.textMuted }}>
                  <i>Belum ada riwayat dokumen yang ditandatangani.</i>
                </div>
              ) : (
                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '14px' }}>
                    <thead>
                      <tr style={{ backgroundColor: '#F3F4F6', borderBottom: `2px solid ${theme.border}` }}>
                        <th style={{ padding: '12px', color: theme.textMuted }}>No</th>
                        <th style={{ padding: '12px', color: theme.textMuted }}>Nama Dokumen</th>
                        <th style={{ padding: '12px', color: theme.textMuted }}>Penandatangan</th>
                        <th style={{ padding: '12px', color: theme.textMuted }}>Waktu</th>
                        <th style={{ padding: '12px', color: theme.textMuted, textAlign: 'center' }}>Aksi</th>
                      </tr>
                    </thead>
                    <tbody>
                      {historyList.map((item, index) => (
                        <tr key={item.id} style={{ borderBottom: `1px solid ${theme.border}` }}>
                          <td style={{ padding: '12px' }}>{index + 1}</td>
                          <td style={{ padding: '12px', fontWeight: '500', color: theme.primary }}>{item.document_name}</td>
                          <td style={{ padding: '12px' }}>{item.signer_name}</td>
                          <td style={{ padding: '12px', color: theme.textMuted }}>{new Date(item.signed_at).toLocaleString('id-ID')}</td>
                          <td style={{ padding: '12px', textAlign: 'center' }}>
                            <button 
                              onClick={() => handleDownload(item.id, item.document_name)}
                              style={{ padding: '6px 12px', backgroundColor: '#10B981', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', fontWeight: '500' }}
                            >
                              ⬇ Unduh PDF
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
{/* MENU 3: VERIFIKASI DOCUMENT */}
          {activeMenu === 'verify' && (
            <div>
              <h2 style={{ marginBottom: '10px' }}>Verifikasi Document</h2>
              <p style={{ color: theme.textMuted, marginBottom: '30px' }}>Unggah dokumen PDF untuk mengekstrak dan memverifikasi keabsahan tanda tangan digital di dalamnya.</p>
              
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

              {/* Menampilkan Hasil Verifikasi jika berhasil */}
              {verifyResult && (
                <div style={{ marginTop: '20px', padding: '20px', backgroundColor: '#ECFDF5', border: '1px solid #10B981', borderRadius: '8px' }}>
                  <h3 style={{ color: '#047857', marginBottom: '10px', display: 'flex', alignItems: 'center', gap: '10px' }}>
                    ✅ {verifyResult.message}
                  </h3>
                  <div style={{ color: '#065F46', fontSize: '15px' }}>
                    <p style={{ margin: '5px 0' }}><strong>Nama Penandatangan:</strong> {verifyResult.signer}</p>
                    <p style={{ margin: '5px 0' }}><strong>Waktu Tanda Tangan:</strong> {new Date(verifyResult.date).toLocaleString('id-ID')}</p>
                    <p style={{ margin: '5px 0' }}><strong>Status Integritas:</strong> Dokumen belum dimodifikasi sejak ditandatangani.</p>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* MENU 4: PENGATURAN & REQUEST ID */}
          {activeMenu === 'settings' && (
            <div>
              <h2 style={{ marginBottom: '10px' }}>Pengaturan Identitas Digital</h2>
              <p style={{ color: theme.textMuted, marginBottom: '30px' }}>Kelola sertifikat dan otentikasi akun Anda.</p>
              
              <div style={{ marginBottom: '40px' }}>
                <h3 style={{ fontSize: '16px', marginBottom: '15px' }}>1. Buat Sertifikat Baru (.p12)</h3>
                <form onSubmit={handleRequestID}>
                  <input type="text" placeholder="Nama Lengkap" required style={styles.input} onChange={e => setReqData({...reqData, name: e.target.value})} />
                  <input type="password" placeholder="Passphrase Baru" required style={styles.input} onChange={e => setReqData({...reqData, passphrase: e.target.value})} />
                  <button type="submit" style={{ ...styles.btn, backgroundColor: '#10B981', width: 'auto', padding: '10px 25px' }}>Request & Download</button>
                </form>
              </div>

              <div style={{ borderTop: `1px solid ${theme.border}`, paddingTop: '30px' }}>
                <h3 style={{ fontSize: '16px', marginBottom: '15px' }}>2. Reset Perangkat Authenticator</h3>
                <p style={{ fontSize: '14px', color: theme.textMuted, marginBottom: '15px' }}>Gunakan ini jika Anda kehilangan akses ke perangkat *smartphone* Anda. Anda akan diminta melakukan *scan* ulang saat login berikutnya.</p>
                <button style={{ ...styles.btnOutline, borderColor: '#E11D48', color: '#E11D48', width: 'auto', padding: '10px 25px' }} onClick={async () => {
                  if(window.confirm('Reset OTP sekarang?')) {
                    await axios.post('http://localhost:8081/reset-otp', { email });
                    setAppState('AUTH'); setPassword('');
                    alert("OTP direset. Silakan login kembali untuk setup ulang.");
                  }
                }}>Reset Keamanan 2FA</button>
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  );
}

export default App;