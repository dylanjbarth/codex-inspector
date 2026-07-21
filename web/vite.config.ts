import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
export default defineConfig({plugins:[react(),tailwindcss()],resolve:{alias:{'@':path.resolve(__dirname,'./src')}},build:{rollupOptions:{output:{entryFileNames:'assets/index.js',chunkFileNames:'assets/[name].js',assetFileNames:asset=>asset.name?.endsWith('.css')?'assets/index.css':'assets/[name][extname]'}}}})
