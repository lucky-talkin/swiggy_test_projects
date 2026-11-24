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
      '/auth-api': { target: process.env.AUTH_API_TARGET || 'http://localhost:8081', changeOrigin: true, rewrite: path => path.replace(/^\/auth-api/, '') },
      '/restaurant-api': { target: process.env.RESTAURANT_API_TARGET || 'http://localhost:8084', changeOrigin: true, rewrite: path => path.replace(/^\/restaurant-api/, '') },
      '/driver-api': { target: process.env.DRIVER_API_TARGET || 'http://localhost:8085', changeOrigin: true, rewrite: path => path.replace(/^\/driver-api/, '') }
    }
  }
})