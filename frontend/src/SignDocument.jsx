import React, { useState } from 'react';
import axios from 'axios';
import { Document, Page, pdfjs } from 'react-pdf';
import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString();

const SignDocument = ({ userEmail }) => {
  const [signatureImage, setSignatureImage] = useState(null);
  const [pdfFile, setPdfFile] = useState(null);
  const [pdfUrl, setPdfUrl] = useState(null);

  // Input Data Penandatangan
  const [signerName, setSignerName] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [otpCode, setOtpCode] = useState('');

  // State untuk Preview PDF dan Koordinat
  const [numPages, setNumPages] = useState(null);
  const [pageNumber, setPageNumber] = useState(1);
  const [signaturePos, setSignaturePos] = useState({ page: 1, x: 0, y: 0, visualX: 0, visualY: 0 });

  const handleImageUpload = (e) => {
    const file = e.target.files[0];
    if (file) setSignatureImage(file);
  };

  const handlePdfUpload = (e) => {
    const file = e.target.files[0];
    if (file) {
      setPdfFile(file);
      setPdfUrl(URL.createObjectURL(file)); 
    }
  };

  const onDocumentLoadSuccess = ({ numPages }) => {
    setNumPages(numPages);
    setPageNumber(1);
  };

  const handlePageClick = (e) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const clickX = e.clientX - rect.left;
    const clickY = e.clientY - rect.top;
    const pdfcpuY = rect.height - clickY; // Balik nilai Y untuk backend

    setSignaturePos({
      page: pageNumber,
      x: Math.round(clickX), 
      y: Math.round(pdfcpuY),
      visualX: clickX,
      visualY: clickY 
    });
  };

  const handleSignDocument = async (e) => {
    e.preventDefault();
    if (!signaturePos.x || !signaturePos.y) {
      alert("Silakan klik pada dokumen PDF untuk menentukan letak tanda tangan!");
      return;
    }

    const formData = new FormData();
    formData.append('email', userEmail);
    formData.append('signerName', signerName);
    formData.append('passphrase', passphrase);
    formData.append('otpCode', otpCode);
    formData.append('file', pdfFile);
    formData.append('signatureImage', signatureImage);
    formData.append('page', signaturePos.page);
    formData.append('x', signaturePos.x);
    formData.append('y', signaturePos.y);

    try {
      const res = await axios.post('http://localhost:8081/web-sign', formData);
      alert(res.data);
      // Opsional: Reload halaman setelah sukses agar bersih
      window.location.reload();
    } catch (err) {
      alert("Gagal: " + err.response?.data);
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: '10px' }}>Upload & Sign Document</h2>
      <p style={{ color: '#6B7280', marginBottom: '30px' }}>Lengkapi data di bawah ini untuk membubuhkan tanda tangan digital pada dokumen Anda.</p>

      {/* TAHAP 1: Upload Gambar Tanda Tangan */}
      <div style={{ marginBottom: '20px', padding: '15px', border: '1px solid #E5E7EB', borderRadius: '8px', backgroundColor: '#F9FAFB' }}>
        <h4 style={{ margin: '0 0 10px 0' }}>Tahap 1: Unggah Tanda Tangan (PNG/JPG) <span style={{ color: '#E11D48' }}>*</span></h4>
        <input type="file" accept="image/png, image/jpeg" onChange={handleImageUpload} style={{ padding: '8px', width: '100%', boxSizing: 'border-box' }} />
        {signatureImage && <p style={{ color: '#10B981', margin: '10px 0 0 0', fontSize: '14px' }}>✅ Gambar berhasil diunggah.</p>}
      </div>

      {/* TAHAP 2: Upload PDF */}
      <div style={{ marginBottom: '20px', padding: '15px', border: '1px solid #E5E7EB', borderRadius: '8px', opacity: signatureImage ? 1 : 0.5 }}>
        <h4 style={{ margin: '0 0 10px 0' }}>Tahap 2: Unggah Dokumen PDF <span style={{ color: '#E11D48' }}>*</span></h4>
        <input type="file" accept="application/pdf" onChange={handlePdfUpload} disabled={!signatureImage} style={{ padding: '8px', width: '100%', boxSizing: 'border-box' }} />
        {!signatureImage && <small style={{ display: 'block', color: '#E11D48', marginTop: '5px' }}>*Wajib unggah gambar TTD terlebih dahulu</small>}
      </div>

      {/* TAHAP 3: Preview dan Pilih Koordinat */}
      {pdfUrl && (
        <div style={{ marginBottom: '20px' }}>
          <h4 style={{ marginBottom: '10px' }}>Tahap 3: Tentukan Letak Koordinat Tanda Tangan</h4>
          <p style={{ fontSize: '14px', marginBottom: '10px' }}>Halaman: {pageNumber} dari {numPages}</p>
          
          <div style={{ display: 'flex', gap: '10px', marginBottom: '15px' }}>
            <button disabled={pageNumber <= 1} onClick={() => setPageNumber(pageNumber - 1)} style={{ padding: '6px 12px', cursor: 'pointer' }}>Sebelahnya</button>
            <button disabled={pageNumber >= numPages} onClick={() => setPageNumber(pageNumber + 1)} style={{ padding: '6px 12px', cursor: 'pointer' }}>Selanjutnya</button>
          </div>

          <div style={{ position: 'relative', display: 'inline-block', border: '1px solid #333', cursor: 'crosshair', maxWidth: '100%', overflow: 'auto' }}>
            <Document file={pdfUrl} onLoadSuccess={onDocumentLoadSuccess}>
              <Page pageNumber={pageNumber} onClick={handlePageClick} renderTextLayer={false} renderAnnotationLayer={false} width={600} />
            </Document>

            {signaturePos.page === pageNumber && signaturePos.visualX > 0 && (
              <div style={{
                position: 'absolute', left: signaturePos.visualX, top: signaturePos.visualY,
                width: '120px', height: '60px', backgroundColor: 'rgba(16, 185, 129, 0.4)',
                border: '2px dashed #10B981', transform: 'translate(0, -100%)', pointerEvents: 'none'
              }}>
                <small style={{ color: '#000', fontWeight: 'bold', padding: '2px' }}>Letak TTD</small>
              </div>
            )}
          </div>
          <p style={{ fontSize: '14px', color: '#4F46E5', fontWeight: 'bold' }}>Koordinat terpilih: X: {signaturePos.x}, Y: {signaturePos.y}</p>
        </div>
      )}

      {/* TAHAP 4: Eksekusi */}
      {signaturePos.x > 0 && (
        <form onSubmit={handleSignDocument} style={{ padding: '20px', border: '1px solid #E5E7EB', borderRadius: '8px', backgroundColor: '#F9FAFB' }}>
          <h4 style={{ margin: '0 0 15px 0' }}>Tahap 4: Otentikasi & Eksekusi</h4>
          
          <input type="text" placeholder="Nama Penandatangan *" required value={signerName} onChange={e => setSignerName(e.target.value)} style={{ width: '100%', padding: '10px', marginBottom: '10px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box' }} />
          <input type="password" placeholder="Passphrase Digital ID *" required value={passphrase} onChange={e => setPassphrase(e.target.value)} style={{ width: '100%', padding: '10px', marginBottom: '10px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box' }} />
          <input type="text" placeholder="6 Digit OTP dari Aplikasi *" maxLength="6" required value={otpCode} onChange={e => setOtpCode(e.target.value)} style={{ width: '100%', padding: '10px', marginBottom: '15px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box', letterSpacing: '2px' }} />
          
          <button type="submit" style={{ width: '100%', padding: '12px', backgroundColor: '#4F46E5', color: 'white', border: 'none', borderRadius: '8px', fontWeight: 'bold', cursor: 'pointer' }}>
            Eksekusi Tanda Tangan
          </button>
        </form>
      )}
    </div>
  );
};

export default SignDocument;