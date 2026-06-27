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
    source: 'gallery' as
      | 'google_photos'
      | 'synology_photos'
      | 'immich'
      | 'gallery'
      | 'url_proxy',
    album: null as number | null,
    // Bumped to ask the Gallery view to refetch photos + album chips
    // (e.g. after a Delete All from the source config tab).
    refreshSignal: 0,
  }),
  getters: {
    totalPages: (state) => Math.ceil(state.totalPhotos / state.limit),
  },
  actions: {
    setSource(
      source:
        | 'google_photos'
        | 'synology_photos'
        | 'immich'
        | 'gallery'
        | 'url_proxy'
    ) {
      this.source = source;
      this.page = 1;
      this.album = null;
      this.photos = [];
      this.totalPhotos = 0;
      this.fetchPhotos();
    },

    setAlbum(album: number | null) {
      this.album = album;
      this.page = 1;
      this.photos = [];
      this.totalPhotos = 0;
      this.fetchPhotos();
    },

    triggerRefresh() {
      this.refreshSignal++;
    },

    async fetchPhotos() {
      this.loading = true;
      // Capture the request context so a slow response from a previous
      // source/album/page doesn't clobber a newer one (e.g. on load, the
      // default 'gallery' fetch racing the hash-restored source).
      const reqSource = this.source;
      const reqAlbum = this.album;
      const reqPage = this.page;
      const isCurrent = () =>
        this.source === reqSource &&
        this.album === reqAlbum &&
        this.page === reqPage;
      try {
        const offset = (reqPage - 1) * this.limit;
        let url = `/gallery/photos?source=${reqSource}&limit=${this.limit}&offset=${offset}`;
        if (reqAlbum != null) {
          url += `&album=${reqAlbum}`;
        }
        const res = await api.get(url);
        if (!isCurrent()) return; // discard stale response
        this.photos = res.data.photos || [];
        this.totalPhotos = res.data.total || 0;
      } catch (e) {
        console.error('Failed to fetch photos', e);
      } finally {
        if (isCurrent()) this.loading = false;
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
        await api.delete(`/gallery/photos/${id}`);
        await this.fetchPhotos();
      } catch (e) {
        console.error('Failed to delete photo', e);
        throw e;
      }
    },

    async deleteAllPhotos() {
      try {
        const res = await api.delete(`/gallery/photos?source=${this.source}`);
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

    async uploadFiles(files: FileList | File[], caption = '') {
      const list = Array.from(files);
      if (list.length === 0) return;

      this.loading = true;
      let added = 0;
      try {
        for (const file of list) {
          const fd = new FormData();
          fd.append('file', file);
          if (caption) fd.append('caption', caption);
          await api.post('/gallery/upload', fd, {
            headers: { 'Content-Type': 'multipart/form-data' },
          });
          added++;
        }
        this.importMessage = `Uploaded ${added} photo${added === 1 ? '' : 's'}.`;
        setTimeout(() => (this.importMessage = ''), 5000);
        this.page = 1;
        await this.fetchPhotos();
      } catch (e: any) {
        console.error('Upload failed', e);
        this.importMessage =
          e?.response?.data?.error || 'Failed to upload photo';
        setTimeout(() => (this.importMessage = ''), 5000);
      } finally {
        this.loading = false;
      }
    },

    async startPicker() {
      const store = useSettingsStore();

      if (this.source === 'synology_photos') {
        this.importMessage =
          'Use the Sync button in Synology settings to add photos.';
        setTimeout(() => (this.importMessage = ''), 5000);
        return;
      }

      if (this.source === 'immich') {
        this.importMessage =
          'Use the Sync button in Immich settings to add photos.';
        setTimeout(() => (this.importMessage = ''), 5000);
        return;
      }

      if (this.source === 'gallery') {
        this.importMessage =
          'Use the Upload button to add photos to the gallery.';
        setTimeout(() => (this.importMessage = ''), 5000);
        return;
      }

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
