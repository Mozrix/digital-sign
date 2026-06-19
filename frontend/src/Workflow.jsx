import React, { useState, useEffect } from 'react';
import axios from 'axios';

const Workflow = ({ email, role, onPrepareSign }) => {
  const [workflows, setWorkflows] = useState([]);
  const [file, setFile] = useState(null);
  const [dosenEmail, setDosenEmail] = useState('');
  const [dosenList, setDosenList] = useState([]);

  // LOGIKA KEAMANAN: Kunci semua akses HTTP (termasuk localhost)
  const PROTOCOL = window.location.protocol; 
  const API_BASE_URL = `${PROTOCOL}//dgsign.test:8081`;

  const isUnsecured = window.location.protocol === 'http:' || window.location.hostname === 'localhost';

  const fetchWorkflows = async () => {
    try {
      const res = await axios.get(`${API_BASE_URL}/workflow/list?email=${email}&role=${role}`);
      setWorkflows(res.data || []);
    } catch (err) {
      console.error("Gagal memuat list workflow:", err);
    }
  };

  const fetchDosens = async () => {
    try {
      const res = await axios.get(`${API_BASE_URL}/dosen/list`);
      setDosenList(res.data || []);
    } catch (err) {
      console.error("Gagal memuat list dosen", err);
    }
  };

  useEffect(() => {
    fetchDosens();
    fetchWorkflows();
  }, [email, role]);

  const handleUpload = async (e) => {
    e.preventDefault();
    if (!file || !dosenEmail) return alert("Lengkapi data!");
    const formData = new FormData();
    formData.append('file', file);
    formData.append('mahasiswa_email', email);
    formData.append('dosen_email', dosenEmail);

    try {
      await axios.post(`${API_BASE_URL}/workflow/create`, formData);
      alert("Pengajuan berhasil dikirim!");
      setFile(null);
      fetchWorkflows();
    } catch (err) {
      alert("Gagal mengirim pengajuan");
    }
  };

  const updateStatus = async (id, action) => {
    try {
      await axios.post(`${API_BASE_URL}/workflow/action`, { id, action });
      fetchWorkflows();
    } catch (err) {
      alert("Gagal memproses aksi status");
    }
  };

  const handleDownloadFile = (path) => {
    // PROTEKSI EKSTRA: Cegah bypass dari inspect element
    if (isUnsecured) {
      alert("Akses ditolak: Fitur unduh dikunci pada koneksi tidak aman (HTTP).");
      return;
    }
    
    if (!path) return alert("Lokasi file kosong atau tidak valid.");
    const url = `${API_BASE_URL}/get-file?path=${encodeURIComponent(path)}`;
    window.open(url, '_blank');
  };

  const getStatusBadge = (status) => {
    const badges = {
      'menunggu_dosen': { bg: '#FEF3C7', color: '#D97706', text: 'Menunggu Dosen' },
      'diterima_dosen': { bg: '#DBEAFE', color: '#2563EB', text: 'Diterima Dosen (Proses TTD)' },
      'ditandatangani': { bg: '#EDE9FE', color: '#7C3AED', text: 'Menunggu Admin/Akademik' },
      'selesai': { bg: '#D1FAE5', color: '#059669', text: 'Selesai (Trusted)' }
    };
    const b = badges[status] || { bg: '#F3F4F6', color: '#374151', text: status };
    return <span style={{ backgroundColor: b.bg, color: b.color, padding: '4px 8px', borderRadius: '4px', fontSize: '12px', fontWeight: 'bold' }}>{b.text}</span>;
  };

  return (
    <div>
      {/* BANNER PERINGATAN KEAMANAN */}
      {isUnsecured && (
        <div style={{ padding: '12px 15px', backgroundColor: '#FEF2F2', border: '1px solid #EF4444', borderRadius: '8px', marginBottom: '20px', fontSize: '14px', color: '#991B1B' }}>
          ⚠️ <b>Fitur Unduh Terkunci:</b> Anda sedang menggunakan koneksi tidak aman (HTTP). Silakan akses via HTTPS untuk mengunduh dokumen rahasia Anda.
        </div>
      )}

      {role === 'user' && (
        <form onSubmit={handleUpload} style={{ padding: '20px', backgroundColor: '#F9FAFB', borderRadius: '8px', border: '1px solid #E5E7EB', marginBottom: '30px' }}>
          <h4 style={{ marginTop: 0 }}>Ajukan Dokumen Baru</h4>
          <label style={{ fontSize: '14px', marginBottom: '5px', display: 'block' }}>Pilih Dosen/Kaprodi:</label>
          <select value={dosenEmail} onChange={e => setDosenEmail(e.target.value)} required style={{ padding: '10px', width: '100%', marginBottom: '10px', borderRadius: '5px', border: '1px solid #ccc' }}>
            <option value="">-- Pilih Dosen / Kaprodi --</option>
            {dosenList.map(d => (
              <option key={d.email} value={d.email}>{d.name} ({d.email})</option>
            ))}
          </select>
          <input type="file" accept="application/pdf" required onChange={e => setFile(e.target.files[0])} style={{ marginBottom: '10px', display: 'block' }} />
          <button type="submit" style={{ padding: '10px 20px', backgroundColor: '#4F46E5', color: 'white', border: 'none', borderRadius: '5px', cursor: 'pointer' }}>Kirim Dokumen</button>
        </form>
      )}

      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '14px' }}>
        <thead>
          <tr style={{ backgroundColor: '#F3F4F6', borderBottom: '2px solid #E5E7EB', textAlign: 'left' }}>
            <th style={{ padding: '12px' }}>Dokumen</th>
            <th style={{ padding: '12px' }}>Mahasiswa</th>
            <th style={{ padding: '12px' }}>Dosen</th>
            <th style={{ padding: '12px' }}>Status</th>
            <th style={{ padding: '12px' }}>Aksi</th>
          </tr>
        </thead>
        <tbody>
          {workflows.map(wf => (
            <tr key={wf.id} style={{ borderBottom: '1px solid #E5E7EB' }}>
              <td style={{ padding: '12px', fontWeight: 'bold' }}>{wf.document_name}</td>
              <td style={{ padding: '12px' }}>{wf.mahasiswa_email}</td>
              <td style={{ padding: '12px' }}>{wf.dosen_email}</td>
              <td style={{ padding: '12px' }}>{getStatusBadge(wf.status)}</td>
              <td style={{ padding: '12px' }}>
                {role === 'dosen' && wf.status === 'menunggu_dosen' && (
                  <button 
                    onClick={async () => {
                      await updateStatus(wf.id, 'diterima_dosen');
                      if (onPrepareSign) { onPrepareSign(wf); }
                    }} 
                    style={{ padding: '6px 12px', backgroundColor: '#b20000', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 'bold' }}
                  >
                    Terima & Siapkan TTD
                  </button>
                )}
                {role === 'admin' && wf.status === 'ditandatangani' && (
                  <button onClick={() => updateStatus(wf.id, 'selesai')} style={{ padding: '6px 12px', backgroundColor: '#10B981', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 'bold' }}>Validasi & Setujui</button>
                )}
                
                {/* LOGIKA TOMBOL UNDUH YANG DIKUNCI JIKA HTTP */}
                {wf.status === 'selesai' && (
                  <button 
                    onClick={() => handleDownloadFile(wf.file_path)} 
                    disabled={isUnsecured}
                    style={{ 
                      padding: '6px 12px', 
                      backgroundColor: isUnsecured ? '#ccc' : '#4F46E5', 
                      color: 'white', 
                      border: 'none', 
                      borderRadius: '4px', 
                      cursor: isUnsecured ? 'not-allowed' : 'pointer' 
                    }}
                  >
                    {isUnsecured ? '⬇ Unduh' : '⬇ Unduh'}
                  </button>
                )}

                {wf.status !== 'selesai' && role === 'user' && <span style={{ color: '#9CA3AF', fontSize: '12px' }}>Menunggu Proses</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default Workflow;