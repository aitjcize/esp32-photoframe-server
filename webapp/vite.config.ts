import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vuetify from 'vite-plugin-vuetify';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), vuetify({ autoImport: true })],
  base: './',
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:9607',
        changeOrigin: true,
      },
      '/image': {
        target: 'http://localhost:9607',
        changeOrigin: true,
      },
    },
  },
});
