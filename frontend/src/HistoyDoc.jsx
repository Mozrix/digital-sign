import React, { useState, useEffect } from 'react';
import axios from 'axios';

const HistoryDocument = ({ userEmail }) => {
  const [histories, setHistories] = useState([]);

  useEffect(() => {
    fetchHistory();
  }, []);

  const fetchHistory = async () => {
    try {
      const res = await axios.get(`https://dgsign.test:8081/history?email=${userEmail}`);
      if (res.data) setHistories(res.data);
    } catch (err) {
      console.error(err);
    }
  };

  const handleDownload = (id, status) => {
    if (status !== 'disetujui') {
      alert("Dokumen belum disetujui Admin. Tidak bisa diunduh.");
      return;
    }
    window.open(`https://dgsign.test:8081/download?id=${id}`);
  };

  return (
    <div>
      <h2>Riwayat Dokumen Saya</h2>
      <table border="1" cellPadding="10" style={{ width: '100%', borderCollapse: 'collapse', marginTop: '20px' }}>
        <thead>
          <tr>
            <th>ID</th><th>Nama Dokumen</th><th>Waktu TTD</th><th>Status</th><th>Aksi</th>
          </tr>
        </thead>
        <tbody>
          {histories.map(h => (
            <tr key={h.id}>
              <td>{h.id}</td>
              <td>{h.document_name}</td>
              <td>{h.signed_at}</td>
              <td style={{ fontWeight: 'bold', color: h.status === 'disetujui' ? 'green' : h.status === 'ditolak' ? 'red' : 'orange' }}>
                {h.status.toUpperCase()}
              </td>
              <td>
                <button 
                  onClick={() => handleDownload(h.id, h.status)}
                  disabled={h.status !== 'disetujui'}
                  style={{ 
                    cursor: h.status === 'disetujui' ? 'pointer' : 'not-allowed', 
                    backgroundColor: h.status === 'disetujui' ? '#4F46E5' : '#ccc', 
                    color: '#fff', padding: '5px 10px', border: 'none', borderRadius: '4px' 
                  }}
                >
                  Download File
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default HistoryDoc;