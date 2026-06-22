# Penjelasan Keamanan QR Code & Tanda Tangan Digital (untuk Dosen)

Dokumen ini menjelaskan mengapa sistem menggunakan **dua lapis keamanan** untuk mendeteksi dokumen yang dimanipulasi, termasuk kasus **stamp tanda tangan yang dipindahkan ke dokumen lain**.

---

## Pertanyaan Awal

> "QR-nya harus memiliki URL berbeda ketika stamp dipindahkan ke dokumen lain, dan bukan dari hashing."

## Kenapa "URL berbeda saat stamp dipindah" tidak bisa dilakukan oleh QR saja

**QR Code adalah gambar statis.** Isinya adalah sebuah URL tertentu, misalnya:

```
https://dgsign.test:3000/verify/1234-abc
```

Saat stamp (QR) dipindahkan ke dokumen lain, **gambarnya identik 100%** — sama persis byte-for-byte. Jadi ketika dipindai:

| Aksi | Request yang diterima server |
|------|------------------------------|
| Scan QR di dokumen ASLI | `GET /verify/1234-abc` |
| Scan QR di dokumen PALSU | `GET /verify/1234-abc` |

**Dua request itu tidak bisa dibedakan oleh server.** Satu-satunya saluran informasi dari QR ke server adalah URL itu sendiri. Karena gambar QR-nya sama, URL-nya juga pasti sama.

Analoginya seperti **fotokopi kunci pintu**: kunci yang difotokopi tetap membuka pintu yang sama. Gagalang pintu tidak bisa tahu siapa yang memegang kuncinya.

> **Tidak ada aplikasi tanda tangan digital di dunia (termasuk Adobe Acrobat, BSrE, Perteleja) yang membuat QR sendiri mendeteksi pemindahan stamp.** Standar industri menggunakan mekanisme berbeda — lihat penjelasan di bawah.

---

## Solusi: Dua Lapis Keamanan (yang sudah ada di sistem ini)

Sistem sudah melindungi dari pemindahan stamp, **tapi di layer yang berbeda**, bukan di QR.

### Lapis 1 — Signature Kriptografis tertanam di dalam PDF (UTAMA)

Saat sebuah dokumen ditandatangani, sistem **menanam tanda tangan digital asli (standar PAdES/PKCS#7) ke dalam byte PDF itu sendiri**, menggunakan kunci privat RSA-2048 milik penandatangan.

Properti penting dari signature ini:

- **Terikat ke byte PDF** — signature dihitung dari seluruh isi dokumen.
- **TIDAK ikut pindah** kalau QR/stamp gambar-nya di-copy ke dokumen lain. Signature tertanam di dalam struktur internal PDF, bukan di gambar QR.

**Cara memverifikasi (langsung terlihat oleh siapapun, tanpa server):**

1. Buka file PDF di **Adobe Acrobat Reader**, **Microsoft Edge**, atau **PDF reader standar** lainnya.
2. Lihat panel "Signatures" / "Tanda Tangan" (biasanya di panel kiri).
3. Hasilnya:

| Situasi | Yang terlihat di PDF Reader |
|---------|------------------------------|
| Dokumen ASLI (QR asli, tidak diubah) | ✅ **"Signed and all signatures are valid"** |
| Stamp dipindah ke dokumen lain | ❌ **"This document is NOT signed"** (signature tidak ikut pindah) |
| PDF diedit setelah ditandatangani | ❌ **"Signature is INVALID / document has been altered"** |

**Ini cara baku industri.** Adobe, BSrE, dan penyelenggara sertifikat elektronik semua pakai pendekatan ini.

### Lapis 2 — QR Code online check (CEK VERSI)

Scan QR → buka link → server mengecek di database:

| Status di DB | Hasil yang ditampilkan |
|--------------|------------------------|
| `ACTIVE` (tanda tangan terbaru) | ✅ **VALID** + info penandatangan |
| `REVOKED` (dokumen sudah ditandatangani ulang, ada versi lebih baru) | ❌ **INVALID** — "Dokumen ini telah digantikan oleh versi tanda tangan yang lebih baru." |

Fungsi Lapis 2: mencegah orang memakai **versi lama** dokumen yang sudah ada tanda tangan baru. Bukan untuk deteksi stamp dipindah.

---

## Tabel Ringkas: Apa Dideteksi, Apa Tidak

| Skenario Serangan | Lapis 1 (PDF Reader) | Lapis 2 (QR Online) |
|-------------------|----------------------|---------------------|
| PDF diedit/dimodifikasi | ❌ INVALID | ❌ INVALID (tamper hash check) |
| **Stamp dipindah ke dokumen lain** | ❌ **INVALID** (signature tidak ikut pindah) | (tidak bisa — butuh PDF fisik) |
| Memakai dokumen versi lama setelah re-sign | (masih valid di reader) | ❌ **INVALID** (REVOKED) |
| QR difoto/ditiru URL-nya | (tidak relevan) | (sama seperti scan normal) |

> Catatan: Mendeteksi "stamp dipindah ke dokumen **lain**" **wajib butuh akses ke file PDF fisik** — karena harus cek signature yang tertanam di dalamnya. Tidak bisa dilakukan hanya dari URL/UUID QR. Itulah kenapa Lapis 1 ada di PDF Reader.

---

## Demo ke Dosen (Cara membuktikannya)

**Skenario A — Dokumen asli:**
1. Unduh PDF yang sudah ditandatangani.
2. Buka di Adobe Acrobat Reader.
3. Tampilkan panel Signature → muncul: ✅ "Signed and all signatures are valid".
4. Scan QR → halaman verifikasi menampilkan ✅ VALID.

**Skenario B — Stamp dipindah ke dokumen palsu:**
1. Screenshot gambar QR/stamp dari dokumen asli.
2. Tempel ke dokumen PDF lain (palsu).
3. Buka dokumen palsu di Adobe Acrobat Reader.
4. Panel Signature → muncul: ❌ "This document is not signed" / tidak ada signature.
   → **Bukti kriptografis dokumen ini palsu.**
5. Scan QR → halaman verifikasi tetap menampilkan info (karena QR-nya valid), TAPI pemeriksa dokumen **langsung tahu dokumen fisiknya palsu** dari Lapis 1.

**Kesimpulan:** Pemindahan stamp TIDAK menghasilkan dokumen yang sah, karena signature asli tidak bisa dipalsukan dan tidak ikut pindah.

---

## Ringkasan untuk Diskusi

- Permintaan "QR menghasilkan URL berbeda saat dipindah" **secara teknis tidak mungkin** karena QR adalah gambar statis — tidak ada software manapun di dunia yang bisa melakukan ini.
- Yang sebenarnya dicari (deteksi stamp dipindah) **sudah terpenuhi** lewat **signature kriptografis tertanam (Lapis 1)**, yang merupakan **standar internasional** (PAdES, dipakai Adobe & BSrE).
- Sistem menggunakan **2 layer saling melengkapi**: PDF Reader untuk deteksi stamp dipindah/modifikasi, QR online untuk deteksi versi lama.
