import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import axios from 'axios';

const VerifyDocument = () => {
  const { uuid } = useParams();
  const navigate = useNavigate();
  const [verifyData, setVerifyData] = useState(null);
  const [status, setStatus] = useState('loading'); // 'loading', 'valid', 'invalid'

  useEffect(() => {
    const fetchVerification = async () => {
      const loggedInEmail = localStorage.getItem('email');

      try {
        const res = await axios.get(`https://dgsign.test:8081/verify/${uuid}?requester=${loggedInEmail || ''}`);
        setVerifyData(res.data);

        if (res.data && res.data.isValid === false) {
          setStatus('invalid');
        } else {
          setStatus('valid');
        }
      } catch (err) {
        setStatus('invalid');
      }
    };

    fetchVerification();
  }, [uuid]);

  // Helper: kalau file asli terverifikasi (hash cocok & ACTIVE), arahkan ke UUID baru
  // bila status QR saat ini SCAN_LIMIT/REVOKED — agar tetap bisa "lihat dokumen".
  const jumpToActive = (resData) => {
    if (resData && resData.isValid === true && resData.uuid && resData.uuid !== uuid) {
      navigate(`/verify/${resData.uuid}`);
    }
  };

  // UI LOADING
  if (status === 'loading') {
    return (
      <div style={cardStyle}>
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#6B7280' }}>
          Memverifikasi dokumen...
        </div>
      </div>
    );
  }

  // UI INVALID (REVOKED / SCAN_LIMIT_REACHED / dokumen dimodifikasi / tidak ditemukan)
  if (status === 'invalid') {
    const detailMessage = verifyData && verifyData.message
      ? verifyData.message
      : 'Maaf, dokumen tidak valid atau QR Code tidak dapat dibaca.';

    const isScanLimit = verifyData && verifyData.status === 'SCAN_LIMIT_REACHED';

    return (
      <div style={cardStyle}>
        <div style={{
          backgroundColor: isScanLimit ? '#FFF7ED' : '#FEF2F2',
          color: isScanLimit ? '#92400E' : '#991B1B',
          ...bannerBase,
          border: isScanLimit ? '1px solid #FED7AA' : '1px solid #FECACA',
        }}>
          {isScanLimit ? '❌ DOKUMEN TIDAK VALID' : '❌ DOKUMEN TIDAK VALID'}
        </div>
        <h2 style={sectionTitle}>
          {isScanLimit ? 'QR Code Kadaluwarsa' : 'Verifikasi Gagal'}
        </h2>
        <p style={{ color: '#6B7280', lineHeight: '1.6' }}>{detailMessage}</p>

        <FileCheckSection uuid={uuid} onAuthentic={jumpToActive} />
      </div>
    );
  }

  // UI VALID (tampilkan info tanda tangan lengkap)
  return (
    <div style={cardStyle}>
      <div style={{ backgroundColor: '#ECFDF5', color: '#065F46', ...bannerBase, border: '1px solid #A7F3D0' }}>
        ✅ DOKUMEN ASLI DAN VALID
      </div>

      <h2 style={sectionTitle}>
        Informasi Tanda Tangan
      </h2>

      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
        <tbody>
          {[
            ['Nama Dokumen', verifyData.filename],
            ['Penandatangan', verifyData.signerName],
            ['Email', verifyData.signerEmail],
            ['Waktu Tanda Tangan', verifyData.signedAt],
            ['Penerbit Sertifikat (Issuer)', verifyData.issuerName],
            ['Berlaku Hingga', verifyData.validUntil],
            ['Nomor Seri', verifyData.certificateSerial]
          ].map(([label, value], index) => (
            <tr key={index} style={{ borderBottom: '1px solid #F3F4F6' }}>
              <td style={{ padding: '12px 0', fontWeight: '600', width: '40%', color: '#6B7280' }}>{label}</td>
              <td style={{ padding: '12px 0', wordBreak: 'break-all' }}>{value}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div style={{ marginTop: '30px', textAlign: 'center' }}>
        {verifyData.isValid ? (
          <button
            onClick={() => window.open(`https://dgsign.test:8081/view/${uuid}`, '_blank')}
            style={primaryBtn}
            onMouseOver={(e) => (e.target.style.backgroundColor = '#1D4ED8')}
            onMouseOut={(e) => (e.target.style.backgroundColor = '#2563EB')}
          >
            👁️ Lihat Dokumen PDF
          </button>
        ) : (
          <p style={{ color: '#991B1B', fontWeight: 'bold' }}>
            Dokumen telah dimodifikasi, tidak dapat ditampilkan.
          </p>
        )}
      </div>

      <FileCheckSection uuid={uuid} onAuthentic={jumpToActive} />
    </div>
  );
};

// --- Section upload untuk pengecekan integritas file lebih lanjut ---
// Bandingkan hash PDF upload dengan hash record UUID dari URL QR.
// Hanya file yang cocok dengan UUID ini yang dianggap ASLI.
const FileCheckSection = ({ uuid, onAuthentic }) => {
  const [selectedFile, setSelectedFile] = useState(null);
  const [result, setResult] = useState(null);
  const [uploading, setUploading] = useState(false);

  const handleFileChange = (e) => {
    setSelectedFile(e.target.files[0]);
    setResult(null);
  };

  const handleVerify = async () => {
    if (!selectedFile) return;
    setUploading(true);
    setResult(null);

    const formData = new FormData();
    formData.append('file', selectedFile);
    formData.append('uuid', uuid);

    try {
      const res = await axios.post(`https://dgsign.test:8081/verify-file`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      setResult(res.data);
      if (res.data && res.data.isValid === true && typeof onAuthentic === 'function') {
        onAuthentic(res.data);
      }
    } catch (err) {
      setResult({
        isValid: false,
        message: 'Gagal menghubungi server verifikasi. Coba lagi.',
      });
    } finally {
      setUploading(false);
    }
  };

  return (
    <div style={{ marginTop: '30px', paddingTop: '25px', borderTop: '2px solid #F3F4F6' }}>
      <h3 style={{ fontSize: '16px', marginBottom: '8px', color: '#1F2937' }}>
        🔬 Pengecekan Integritas File
      </h3>
      <p style={{ color: '#6B7280', fontSize: '13px', marginBottom: '15px', lineHeight: '1.5' }}>
        Untuk memastikan PDF fisik di tangan Anda benar-benar asli (bukan dipalsukan, ditukar,
        atau QR dipindah dari dokumen lain), unggah file PDF di bawah ini. Sistem akan
        mencocokkan file dengan QR Code ini.
      </p>

      <input
        type="file"
        accept="application/pdf"
        onChange={handleFileChange}
        style={{ width: '100%', padding: '10px', border: '1px solid #D1D5DB', borderRadius: '8px', marginBottom: '15px', boxSizing: 'border-box' }}
      />

      <button
        onClick={handleVerify}
        disabled={!selectedFile || uploading}
        style={{
          width: '100%',
          backgroundColor: (!selectedFile || uploading) ? '#9CA3AF' : '#4F46E5',
          color: 'white',
          padding: '12px 24px',
          border: 'none',
          borderRadius: '8px',
          cursor: (!selectedFile || uploading) ? 'not-allowed' : 'pointer',
          fontSize: '15px',
          fontWeight: 'bold',
        }}
      >
        {uploading ? 'Memeriksa...' : 'Periksa File'}
      </button>

      {result && (
        <div style={{
          marginTop: '20px',
          padding: '15px',
          borderRadius: '8px',
          textAlign: 'center',
          fontWeight: 'bold',
          backgroundColor: result.isValid ? '#ECFDF5' : (result.isAuthentic ? '#FFF7ED' : '#FEF2F2'),
          color: result.isValid ? '#065F46' : (result.isAuthentic ? '#991B1B' : '#991B1B'),
          border: result.isValid ? '1px solid #A7F3D0' : (result.isAuthentic ? '1px solid #FED7AA' : '1px solid #FECACA'),
        }}>
          {result.isValid ? '✅ FILE ASLI' : (result.isAuthentic ? '❌ FILE TIDAK COCOK' : '❌ FILE TIDAK COCOK')}
          <p style={{ marginTop: '10px', fontWeight: 'normal', fontSize: '13px', lineHeight: '1.5' }}>
            {result.message}
          </p>
        </div>
      )}
    </div>
  );
};

// --- Shared styles ---
const cardStyle = {
  maxWidth: '600px',
  margin: '50px auto',
  padding: '30px',
  backgroundColor: '#fff',
  border: '1px solid #E5E7EB',
  borderRadius: '12px',
  boxShadow: '0 4px 6px rgba(0,0,0,0.05)',
  fontFamily: 'sans-serif',
  color: '#1F2937',
};

const bannerBase = {
  padding: '15px',
  borderRadius: '8px',
  textAlign: 'center',
  marginBottom: '25px',
  fontWeight: 'bold',
};

const sectionTitle = {
  marginBottom: '20px',
  fontSize: '20px',
  borderBottom: '2px solid #F3F4F6',
  paddingBottom: '10px',
};

const primaryBtn = {
  backgroundColor: '#2563EB',
  color: 'white',
  padding: '12px 24px',
  border: 'none',
  borderRadius: '8px',
  cursor: 'pointer',
  fontSize: '16px',
  fontWeight: 'bold',
  transition: 'background 0.3s',
};

export default VerifyDocument;
