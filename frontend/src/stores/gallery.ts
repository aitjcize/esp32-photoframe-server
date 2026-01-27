import { defineStore } from 'pinia';
import { api } from '../api';
import { useSettingsStore } from './settings';

export const useGalleryStore = defineStore('gallery', {
  state: () => ({
    photos: [] as any[],
    totalPhotos: 0,
    loading: false,
    page: 1,
    limit: 48,
    importMessage: '',
    pickerTimer: null as number | null,
  }),
  getters: {
    totalPages: (state) => Math.ceil(state.totalPhotos / state.limit),
  },
  actions: {
    async fetchPhotos() {
      this.loading = true;
      try {
        const offset = (this.page - 1) * this.limit;
        const res = await api.get(
          `/google-photos?limit=${this.limit}&offset=${offset}`
        );
        this.photos = res.data.photos || [];
        this.totalPhotos = res.data.total || 0;
      } catch (e) {
        console.error('Failed to fetch photos', e);
      } finally {
        this.loading = false;
      }
    },

    nextPage() {
      if (this.page < this.totalPages) {
        this.page++;
        this.fetchPhotos();
      }
    },

    previousPage() {
      if (this.page > 1) {
        this.page--;
        this.fetchPhotos();
      }
    },

    async deletePhoto(id: number) {
      try {
        await api.delete(`/google-photos/${id}`);
        await this.fetchPhotos();
      } catch (e) {
        console.error('Failed to delete photo', e);
        throw e;
      }
    },

    async deleteAllPhotos() {
      try {
        const res = await api.delete('/google-photos');
        this.importMessage =
          res.data.message || 'All photos deleted successfully!';
        setTimeout(() => (this.importMessage = ''), 5000);
        this.page = 1;
        await this.fetchPhotos();
      } catch (e) {
        console.error('Failed to delete photos', e);
        throw e;
      }
    },

    async startPicker() {
      const store = useSettingsStore();
      if (
        !store.settings.google_client_id ||
        !store.settings.google_client_secret
      ) {
        this.importMessage =
          'Please configure Google Photos Credentials in Settings first.';
        setTimeout(() => (this.importMessage = ''), 5000);
        return;
      }

      this.loading = true;
      try {
        const res = await api.get('/google/picker/session');
        const { id, pickerUri } = res.data;

        // Open Popup
        const width = 800;
        const height = 600;
        const left = (window.screen.width - width) / 2;
        const top = (window.screen.height - height) / 2;
        window.open(
          pickerUri,
          'GooglePicker',
          `width=${width},height=${height},top=${top},left=${left}`
        );

        this.pollPicker(id);
      } catch (e) {
        console.error(e);
        this.importMessage = 'Failed to start picker flow';
        this.loading = false;
      }
    },

    pollPicker(sessionId: string) {
      if (this.pickerTimer) clearInterval(this.pickerTimer);

      this.pickerTimer = window.setInterval(async () => {
        try {
          const res = await api.get(`/google/picker/poll/${sessionId}`);
          const { complete } = res.data;
          if (complete) {
            if (this.pickerTimer) clearInterval(this.pickerTimer);
            await this.processPicker(sessionId);
          }
        } catch (e) {
          console.error('Polling error', e);
        }
      }, 2000);
    },

    async processPicker(sessionId: string) {
      try {
        const res = await api.post(`/google/picker/process/${sessionId}`);
        if (res.status === 202) {
          this.pollProgress(sessionId);
        } else {
          const { count } = res.data;
          this.importMessage = `Successfully added ${count} photos!`;
          setTimeout(() => (this.importMessage = ''), 5000);
          this.fetchPhotos();
          this.loading = false;
        }
      } catch (e) {
        console.error('Process error', e);
        this.importMessage = 'Error processing photos';
        this.loading = false;
      }
    },

    pollProgress(sessionId: string) {
      const progressInterval = setInterval(async () => {
        try {
          const pRes = await api.get(`/google/picker/progress/${sessionId}`);
          const pData = pRes.data;
          this.fetchPhotos();

          if (pData.status === 'done') {
            clearInterval(progressInterval);
            this.importMessage = `Successfully added ${pData.processed} photos!`;
            setTimeout(() => (this.importMessage = ''), 5000);
            this.loading = false;
          } else if (pData.status === 'error') {
            clearInterval(progressInterval);
            this.importMessage = `Error: ${pData.error}`;
            this.loading = false;
          }
        } catch (e) {
          console.error('Progress poll error', e);
        }
      }, 2000);
    },
  },
});
