import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import axios from 'axios';

const VerifyDocument = () => {
  const { uuid } = useParams();
  const [verifyData, setVerifyData] = useState(null);
  const [status, setStatus] = useState('loading'); // 'loading', 'valid', 'invalid'

  useEffect(() => {
    const fetchVerification = async () => {
      const loggedInEmail = localStorage.getItem('email');
      
      try {
        // Kalau belum login, kirim empty string atau biarkan error
        const res = await axios.get(`https://dgsign.test:8081/verify/${uuid}?requester=${loggedInEmail || ''}`);
        setVerifyData(res.data);
        setStatus('valid');
      } catch (err) {
        setStatus('invalid'); // Apapun errornya (QR rusak/blm login), tampilkan invalid
      }
    };

    fetchVerification();
  }, [uuid]);

  // UI LOADING
  if (status === 'loading') return <h3>Memverifikasi...</h3>;

  // UI INVALID (Dipakai untuk: Belum Login, QR Rusak, Dokumen Palsu)
  if (status === 'invalid') {
    return (
      <div style={{ padding: '50px', textAlign: 'center', fontFamily: 'sans-serif' }}>
        <h2 style={{ color: '#991B1B' }}>❌ Dokumen Tidak Ditemukan</h2>
        <p>Maaf, dokumen tidak valid atau QR Code tidak dapat dibaca.</p>
      </div>
    );
  }

  // UI VALID (Tampilkan data asli)
  return (
    <div style={{ maxWidth: '600px', margin: '50px auto', padding: '30px', backgroundColor: '#fff', border: '1px solid #E5E7EB', borderRadius: '12px', boxShadow: '0 4px 6px rgba(0,0,0,0.05)', fontFamily: 'sans-serif', color: '#1F2937' }}>
      <div style={{ backgroundColor: '#ECFDF5', color: '#065F46', padding: '15px', borderRadius: '8px', textAlign: 'center', marginBottom: '25px', fontWeight: 'bold', border: '1px solid #A7F3D0' }}>
        ✅ DOKUMEN ASLI DAN VALID
      </div>

      <h2 style={{ marginBottom: '20px', fontSize: '20px', borderBottom: '2px solid #F3F4F6', paddingBottom: '10px' }}>
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
            style={{ 
              backgroundColor: '#2563EB', 
              color: 'white', 
              padding: '12px 24px', 
              border: 'none', 
              borderRadius: '8px', 
              cursor: 'pointer',
              fontSize: '16px',
              fontWeight: 'bold',
              transition: 'background 0.3s'
            }}
            onMouseOver={(e) => e.target.style.backgroundColor = '#1D4ED8'}
            onMouseOut={(e) => e.target.style.backgroundColor = '#2563EB'}
          >
            👁️ Lihat Dokumen PDF
          </button>
        ) : (
          <p style={{ color: '#991B1B', fontWeight: 'bold' }}>
            Dokumen telah dimodifikasi, tidak dapat ditampilkan.
          </p>
        )}
      </div>
    </div>
    
  );
};

export default VerifyDocument;