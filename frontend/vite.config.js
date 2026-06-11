import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import fs from 'fs'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000, // Memaksa Vite berjalan di port 3000
    host: 'dgsign.test', // Menggunakan domain lokal yang baru dibuat
    https: {
      // Pastikan path ini benar mengarah ke lokasi file .pem di folder backend Anda
      key: fs.readFileSync(path.resolve(__dirname, '../backend/key.pem')),
      cert: fs.readFileSync(path.resolve(__dirname, '../backend/cert.pem')),
    }
  }
})