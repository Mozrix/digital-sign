import React, { useState, useEffect } from 'react';
import axios from 'axios';

// Sesuaikan URL ini dengan backend Anda

// Bawa objek theme ke sini agar styling tabelnya tetap berfungsi
const theme = {
  bg: '#f4f7f6',
  card: '#ffffff',
  primary: '#4F46E5',
  text: '#1F2937',
  textMuted: '#6B7280',
  border: '#E5E7EB',
  radius: '12px',
  shadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)'
};

  const PROTOCOL = window.location.protocol; 
  const API_BASE_URL = `${PROTOCOL}//dgsign.test:8081`;

  const isUnsecured = window.location.protocol === 'http:' || window.location.hostname === 'localhost';

const HistoryDocument = ({ userEmail }) => {
  const [historyList, setHistoryList] = useState([]);
  
  useEffect(() => {
    if (userEmail) {
      fetchHistory();
    }
  }, [userEmail]);

  const fetchHistory = async () => {
    try {
      const res = await axios.get(`${API_BASE_URL}/history?email=${userEmail}`);
      setHistoryList(res.data || []);
    } catch (err) {
      console.error("Gagal mengambil riwayat:", err);
    }
  };

  const handleDownload = async (id, docName, status, token) => {
    if (isUnsecured) {
      alert("Akses ditolak: Fitur unduh dikunci pada koneksi tidak aman (HTTP).");
      return;
    }
    if (status !== 'disetujui') {
      alert("Dokumen belum disetujui Admin. Tidak bisa diunduh.");
      return;
    }

    try {
      // Menggunakan cara download blob agar lebih aman & rapi
      const response = await axios.get(`${API_BASE_URL}/download?token=${token}`, { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([response.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', docName); 
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link); // Bersihkan sisa link di memori
    } catch (err) {
      alert("Gagal mengunduh dokumen. File mungkin belum siap di server.");
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: '10px' }}>Riwayat Document</h2>
      <p style={{ color: theme.textMuted, marginBottom: '30px' }}>Daftar dokumen yang telah Anda tandatangani secara digital.</p>
      
      {isUnsecured && (
        <div style={{ padding: '12px 15px', backgroundColor: '#FEF2F2', border: '1px solid #EF4444', borderRadius: '8px', marginBottom: '20px', fontSize: '14px', color: '#991B1B' }}>
          ⚠️ <b>Fitur Unduh Terkunci:</b> Anda sedang menggunakan koneksi tidak aman (HTTP). Silakan akses via HTTPS untuk mengunduh dokumen rahasia Anda.
        </div>
      )}
      
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
                <th style={{ padding: '12px', color: theme.textMuted }}>Status</th>
                <th style={{ padding: '12px', color: theme.textMuted }}>Waktu</th>
                <th style={{ padding: '12px', color: theme.textMuted, textAlign: 'center' }}>Aksi</th>
              </tr>
            </thead>
            <tbody>
              {historyList.map((item, index) => (
                <tr key={item.id} style={{ borderBottom: `1px solid ${theme.border}` }}>
                  <td style={{ padding: '12px' }}>{index + 1}</td>
                  <td style={{ padding: '12px', fontWeight: '500', color: theme.primary }}>{item.document_name}</td>
                  <td style={{ padding: '12px', fontWeight: 'bold', color: item.status === 'disetujui' ? '#10B981' : item.status === 'ditolak' ? '#EF4444' : '#F59E0B' }}>
                    {item.status ? item.status.toUpperCase() : 'MENUNGGU'}
                  </td>
                  <td style={{ padding: '12px', color: theme.textMuted }}>{new Date(item.signed_at).toLocaleString('id-ID')}</td>
                  <td style={{ padding: '12px', textAlign: 'center' }}>
                    <button 
                      onClick={() => handleDownload(item.id, item.document_name, item.status, item.file_token)}
                      disabled={item.status !== 'disetujui' || isUnsecured}
                      style={{ 
                        padding: '6px 12px', 
                        backgroundColor: (item.status === 'disetujui' && !isUnsecured) ? '#10B981' : '#ccc', 
                        color: 'white', border: 'none', borderRadius: '4px', 
                        cursor: (item.status === 'disetujui' && !isUnsecured) ? 'pointer' : 'not-allowed', 
                        fontSize: '13px', fontWeight: '500' 
                      }}
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
  );
};

export default HistoryDocument;