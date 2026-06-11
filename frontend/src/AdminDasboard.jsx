import React, { useState, useEffect } from 'react';
import axios from 'axios';

const AdminDashboard = () => {
  const [allHistories, setAllHistories] = useState([]);

  useEffect(() => {
    fetchAllHistory();
  }, []);

  const fetchAllHistory = async () => {
    try {
      const res = await axios.get('http://localhost:8081/admin/history');
      if (res.data) setAllHistories(res.data);
    } catch (err) {
      console.error(err);
    }
  };

    const updateStatus = async (id, newStatus) => {
    if (!window.confirm(`Yakin ingin mengubah status dokumen ini menjadi ${newStatus.toUpperCase()}?`)) return;
    try {
      const res = await axios.post('http://localhost:8081/admin/approve', {
        document_id: id,
        status: newStatus
      });
      alert(res.data);
      fetchAllHistory(); // Refresh tabel
    } catch (err) {
      // TAMPILKAN ERROR ASLI DARI SERVER BUKAN PESAN DEFAULT
      alert("GAGAL: " + (err.response?.data || err.message));
      console.error(err);
    }
  };

  return (
    <div>
      <h2 style={{ color: '#E11D48' }}>Dashboard Administrator - Persetujuan Dokumen</h2>
      <table border="1" cellPadding="10" style={{ width: '100%', borderCollapse: 'collapse', marginTop: '20px' }}>
        <thead style={{ backgroundColor: '#F3F4F6' }}>
          <tr>
            <th>ID</th><th>Email User</th><th>Nama Dokumen</th><th>Status Saat Ini</th><th>Aksi Admin</th>
          </tr>
        </thead>
        <tbody>
          {allHistories.map(h => (
            <tr key={h.id}>
              <td>{h.id}</td>
              <td>{h.email}</td>
              <td>{h.document_name}</td>
              <td style={{ fontWeight: 'bold' }}>{h.status.toUpperCase()}</td>
              <td>
                {h.status === 'menunggu' && (
                  <>
                    <button onClick={() => updateStatus(h.id, 'disetujui')} style={{ backgroundColor: '#10B981', color: 'white', marginRight: '10px', padding: '5px', border: 'none', cursor: 'pointer' }}>Setujui</button>
                    <button onClick={() => updateStatus(h.id, 'ditolak')} style={{ backgroundColor: '#EF4444', color: 'white', padding: '5px', border: 'none', cursor: 'pointer' }}>Tolak</button>
                  </>
                )}
                {h.status === 'disetujui' && <span style={{ color: 'green' }}>✓ Sudah Disetujui</span>}
                {h.status === 'ditolak' && <span style={{ color: 'red' }}>✕ Ditolak</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default AdminDashboard;