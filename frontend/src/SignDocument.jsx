import React, { useEffect, useMemo, useRef, useState } from 'react';
import axios from 'axios';
import { Document, Page, pdfjs } from 'react-pdf';
import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';

pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

const SIGN_BOX_DEFAULT = { page: 1, x: 80, y: 80, width: 280, height: 140 };

const SignDocument = ({ userEmail, docToSign, clearDoc }) => {
  const [pdfFile, setPdfFile] = useState(null);
  const [pdfUrl, setPdfUrl] = useState(null);
  const [signaturePngFile, setSignaturePngFile] = useState(null);
  const [signaturePngPreview, setSignaturePngPreview] = useState('');
  
  // --- PERBAIKAN 1: Tambahkan state signerName ---
  const [signerName, setSignerName] = useState('');
  
  const [passphrase, setPassphrase] = useState('');
  const [otpCode, setOtpCode] = useState('');
  const [numPages, setNumPages] = useState(null);
  const [pageNumber, setPageNumber] = useState(1);
  const [signatureBox, setSignatureBox] = useState(SIGN_BOX_DEFAULT);
  
  const [workflowID, setWorkflowID] = useState(null); 
  // Berikan nilai default kertas A4 standar (595x842) agar tidak pernah 0
  const [pdfPageInfo, setPdfPageInfo] = useState({ width: 595.28, height: 841.89 });

  const [isDragging, setIsDragging] = useState(false);
  const [isResizing, setIsResizing] = useState(false);
  
  const [dragStart, setDragStart] = useState({ mouseX: 0, mouseY: 0, boxX: 0, boxY: 0 });
  const [resizeStart, setResizeStart] = useState({ mouseX: 0, mouseY: 0, width: 0, height: 0 });
  
  const previewRef = useRef(null);
  const [previewWidth, setPreviewWidth] = useState(600);

  // LOGIKA AMBIL FILE JIKA DITERUSKAN DARI WORKFLOW
  useEffect(() => {
    if (docToSign) {
      const loadDoc = async () => {
        try {
          const targetPath = docToSign.file_path;
          const encodedPath = encodeURIComponent(targetPath);
          
          const response = await axios.get(`https://dgsign.test:8081/get-file?path=${encodedPath}`, { 
            responseType: 'blob' 
          });
          
          const blob = new Blob([response.data], { type: 'application/pdf' });
          const file = new File([blob], docToSign.document_name, { type: 'application/pdf' });

          setPdfFile(file);
          setPdfUrl(URL.createObjectURL(file));
          setWorkflowID(docToSign.id); 
          setSignatureBox({ ...SIGN_BOX_DEFAULT, page: 1 });
          
          clearDoc(); 
        } catch (err) {
          console.error("Gagal memuat dokumen otomatis:", err);
          alert("Gagal memuat dokumen otomatis dari antrean.");
        }
      };
      loadDoc();
    }
  }, [docToSign, clearDoc]);

  useEffect(() => {
    if (!pdfUrl) return;
    return () => URL.revokeObjectURL(pdfUrl);
  }, [pdfUrl]);

  useEffect(() => {
    if (!signaturePngPreview) return;
    return () => URL.revokeObjectURL(signaturePngPreview);
  }, [signaturePngPreview]);

  const handlePdfUpload = (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setPdfFile(file);
    setPdfUrl(URL.createObjectURL(file));
    setWorkflowID(null); 
    setSignatureBox({ ...SIGN_BOX_DEFAULT, page: 1 });
  };

  const handleSignaturePngUpload = (e) => {
    const file = e.target.files[0];
    if (!file) return;
    if (!file.type.includes('png') && !file.name.toLowerCase().endsWith('.png')) {
      alert('Harap pilih file PNG saja untuk tanda tangan gambar.');
      e.target.value = '';
      return;
    }
    setSignaturePngFile(file);
    setSignaturePngPreview(URL.createObjectURL(file));
  };

  const onDocumentLoadSuccess = ({ numPages }) => {
    setNumPages(numPages);
    setPageNumber(1);
  };

  const onPageLoadSuccess = (page) => {
    const viewport = page.getViewport({ scale: 1 });
    setPdfPageInfo({ width: viewport.width, height: viewport.height });
  };

  const updatePreviewScale = () => {
    if (previewRef.current) {
      setPreviewWidth(previewRef.current.clientWidth || 600);
    }
  };

  useEffect(() => {
    updatePreviewScale();
    window.addEventListener('resize', updatePreviewScale);
    return () => window.removeEventListener('resize', updatePreviewScale);
  }, []);

  const handlePageClick = (e) => {
    if (isDragging || isResizing) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const clickX = Math.max(0, Math.min(rect.width, e.clientX - rect.left));
    const clickY = Math.max(0, Math.min(rect.height, e.clientY - rect.top));
    setSignatureBox((prev) => ({
      ...prev,
      page: pageNumber,
      x: Math.min(Math.max(0, clickX - prev.width / 2), rect.width - prev.width),
      y: Math.min(Math.max(0, clickY - prev.height / 2), rect.height - prev.height),
    }));
  };

  const startDrag = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
    setDragStart({ 
      mouseX: e.clientX, 
      mouseY: e.clientY, 
      boxX: signatureBox.x, 
      boxY: signatureBox.y 
    });
  };

  const startResize = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsResizing(true);
    setResizeStart({ 
      mouseX: e.clientX, 
      mouseY: e.clientY, 
      width: signatureBox.width, 
      height: signatureBox.height 
    });
  };

  const handleMouseMove = (e) => {
    if (!previewRef.current) return;
    const rect = previewRef.current.getBoundingClientRect();
    const maxX = Math.max(0, rect.width - signatureBox.width);
    const maxY = Math.max(0, rect.height - signatureBox.height);

    if (isDragging) {
      const deltaX = e.clientX - dragStart.mouseX;
      const deltaY = e.clientY - dragStart.mouseY;
      const nextX = Math.min(Math.max(0, dragStart.boxX + deltaX), maxX);
      const nextY = Math.min(Math.max(0, dragStart.boxY + deltaY), maxY);
      setSignatureBox((prev) => ({ ...prev, x: nextX, y: nextY }));
    }

    if (isResizing) {
      const deltaX = e.clientX - resizeStart.mouseX;
      const deltaY = e.clientY - resizeStart.mouseY;
      const newWidth = Math.max(100, Math.min(rect.width - signatureBox.x, resizeStart.width + deltaX));
      const newHeight = Math.max(50, Math.min(rect.height - signatureBox.y, resizeStart.height + deltaY));
      setSignatureBox((prev) => ({ ...prev, width: newWidth, height: newHeight }));
    }
  };

  const stopInteraction = () => {
    setIsDragging(false);
    setIsResizing(false);
  };

  useEffect(() => {
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', stopInteraction);
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', stopInteraction);
    };
  });

  const statusText = useMemo(() => {
    const box = signatureBox;
    return `Halaman ${box.page} • X:${Math.round(box.x)} Y:${Math.round(box.y)} • ${Math.round(box.width)}x${Math.round(box.height)}`;
  }, [signatureBox]);

  const handleSignDocument = async (e) => {
    e.preventDefault();
    if (!pdfFile) {
      alert('Harap unggah dokumen PDF terlebih dahulu.');
      return;
    }

    const scale = pdfPageInfo.width ? (previewWidth / pdfPageInfo.width) : 1;
    const finalWidth = signatureBox.width / scale;
    const finalHeight = signatureBox.height / scale;
    const finalX = signatureBox.x / scale;
    const finalY = pdfPageInfo.height - (signatureBox.y / scale) - finalHeight;

    const formData = new FormData();
    formData.append('email', userEmail);
    // --- PERBAIKAN 2: Masukkan signerName ke formData ---
    formData.append('signerName', signerName);
    
    formData.append('passphrase', passphrase);
    formData.append('otpCode', otpCode);
    formData.append('file', pdfFile);
    formData.append('page', String(signatureBox.page));
    formData.append('x', String(Math.round(finalX)));
    formData.append('y', String(Math.round(finalY)));
    formData.append('width', String(Math.round(finalWidth)));
    formData.append('height', String(Math.round(finalHeight)));
    
    if (workflowID) {
      formData.append('workflow_id', String(workflowID));
    }
    if (signaturePngFile) {
      formData.append('signatureImage', signaturePngFile);
    }

    try {
      const res = await axios.post('https://dgsign.test:8081/web-sign', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      alert(res.data?.message || 'Dokumen berhasil ditandatangani.');
      
      // Reset form setelah berhasil
      setSignerName('');
      setPassphrase('');
      setOtpCode('');
      setSignaturePngFile(null);
      setSignaturePngPreview('');
      
    } catch (err) {
      alert(err.response?.data || 'Gagal melakukan tanda tangan digital.');
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: '10px' }}>Digital Signature</h2>
      <p style={{ color: '#6B7280', marginBottom: '30px' }}>Pilih area tanda tangan, lalu konfirmasi dengan passphrase dan OTP.</p>

      <div style={{ marginBottom: '20px', padding: '15px', border: '1px solid #E5E7EB', borderRadius: '8px' }}>
        <h4 style={{ margin: '0 0 10px 0' }}>Unggah Dokumen PDF {workflowID && <span style={{ color: '#10B981' }}>(Otomatis terisi)</span>} <span style={{ color: '#E11D48' }}>*</span></h4>
        <input type="file" accept="application/pdf" onChange={handlePdfUpload} style={{ padding: '8px', width: '100%', boxSizing: 'border-box' }} />
      </div>

      {pdfUrl && (
        <div style={{ marginBottom: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
            <h4 style={{ margin: 0 }}>Tentukan Posisi Signature</h4>
            <span style={{ fontSize: '13px', color: '#6B7280' }}>{statusText}</span>
          </div>
          <div style={{ display: 'flex', gap: '10px', marginBottom: '15px' }}>
            <button disabled={pageNumber <= 1} onClick={() => setPageNumber(pageNumber - 1)} style={{ padding: '6px 12px', cursor: pageNumber <= 1 ? 'not-allowed' : 'pointer' }}>Sebelumnya</button>
            <button disabled={pageNumber >= numPages} onClick={() => setPageNumber(pageNumber + 1)} style={{ padding: '6px 12px', cursor: pageNumber >= numPages ? 'not-allowed' : 'pointer' }}>Selanjutnya</button>
          </div>

          <div ref={previewRef} style={{ position: 'relative', display: 'inline-block', border: '1px solid #333', maxWidth: '100%', overflow: 'hidden' }}>
            <Document file={pdfUrl} onLoadSuccess={onDocumentLoadSuccess}>
              <Page
                pageNumber={pageNumber}
                onClick={handlePageClick}
                onLoadSuccess={onPageLoadSuccess}
                renderTextLayer={false}
                renderAnnotationLayer={false}
                width={previewWidth}
              />
            </Document>

            {signatureBox.page === pageNumber && (
              <div
                onMouseDown={startDrag}
                onClick={(e) => e.stopPropagation()}
                style={{
                  position: 'absolute',
                  left: signatureBox.x,
                  top: signatureBox.y,
                  width: signatureBox.width,
                  height: signatureBox.height,
                  background: 'rgba(79, 70, 229, 0.08)',
                  border: '2px dashed #4F46E5',
                  borderRadius: '6px',
                  cursor: isDragging ? 'grabbing' : 'grab'
                }}
              >
                <div style={{ position: 'absolute', inset: '0', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '12px', color: '#4F46E5', fontWeight: '700' }}>
                  Digitally Signed
                </div>
                <div
                  onMouseDown={startResize}
                  style={{
                    position: 'absolute',
                    right: '-6px',
                    bottom: '-6px',
                    width: '12px',
                    height: '12px',
                    background: '#4F46E5',
                    borderRadius: '50%',
                    cursor: 'nwse-resize'
                  }}
                />
              </div>
            )}
          </div>
        </div>
      )}

      {pdfUrl && (
        <form onSubmit={handleSignDocument} style={{ padding: '20px', border: '1px solid #E5E7EB', borderRadius: '8px', backgroundColor: '#F9FAFB' }}>
          <h4 style={{ margin: '0 0 15px 0' }}>Konfirmasi & Sign</h4>
          
          {/* --- PERBAIKAN 3: Tambahkan kolom input untuk Nama Penandatangan --- */}
          <input 
            type="text" 
            placeholder="Nama Penandatangan *" 
            required 
            value={signerName} 
            onChange={(e) => setSignerName(e.target.value)} 
            style={{ width: '100%', padding: '10px', marginBottom: '10px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box' }} 
          />

          <div style={{ marginBottom: '15px' }}>
            <label style={{ display: 'block', marginBottom: '6px', fontSize: '13px', color: '#6B7280' }}>Upload tanda tangan PNG (opsional)</label>
            <input type="file" accept="image/png" onChange={handleSignaturePngUpload} style={{ padding: '8px', width: '100%', boxSizing: 'border-box' }} />
            {signaturePngPreview && (
              <div style={{ marginTop: '10px' }}>
                <img src={signaturePngPreview} alt="Preview tanda tangan" style={{ maxWidth: '220px', maxHeight: '120px', border: '1px solid #E5E7EB', borderRadius: '6px' }} />
              </div>
            )}
          </div>
          
          <input type="password" placeholder="Passphrase Digital ID *" required value={passphrase} onChange={(e) => setPassphrase(e.target.value)} style={{ width: '100%', padding: '10px', marginBottom: '10px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box' }} />
          <input type="text" placeholder="6 Digit OTP dari Aplikasi *" maxLength="6" required value={otpCode} onChange={(e) => setOtpCode(e.target.value)} style={{ width: '100%', padding: '10px', marginBottom: '15px', borderRadius: '5px', border: '1px solid #ccc', boxSizing: 'border-box', letterSpacing: '2px' }} />
          <button type="submit" style={{ width: '100%', padding: '12px', backgroundColor: '#4F46E5', color: 'white', border: 'none', borderRadius: '8px', fontWeight: 'bold', cursor: 'pointer' }}>
            Tandatangani PDF
          </button>
        </form>
      )}
    </div>
  );
};

export default SignDocument;