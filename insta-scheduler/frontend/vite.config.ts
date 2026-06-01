import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/projects/insta-scheduler/',
  plugins: [react()],
})
