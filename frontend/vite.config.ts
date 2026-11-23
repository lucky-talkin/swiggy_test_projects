import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/search-api': { target: process.env.SEARCH_API_TARGET || 'http://localhost:8083', changeOrigin: true, rewrite: path => path.replace(/^\/search-api/, '') },
      '/order-api': { target: process.env.ORDER_API_TARGET || 'http://localhost:8086', changeOrigin: true, rewrite: path => path.replace(/^\/order-api/, '') },
      '/catalog-api': { target: process.env.CATALOG_API_TARGET || 'http://localhost:8082', changeOrigin: true, rewrite: path => path.replace(/^\/catalog-api/, '') },
      '/auth-api': { target: process.env.AUTH_API_TARGET || 'http://localhost:8081', changeOrigin: true, rewrite: path => path.replace(/^\/auth-api/, '') }
    }
  }
})