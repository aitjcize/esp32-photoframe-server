import { defineStore } from 'pinia';
import { api } from '../api';

export const useImmichStore = defineStore('immich', {
  state: () => ({
    count: 0,
    albums: [] as any[],
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchCount() {
      try {
        const res = await api.get('/immich/count');
        this.count = res.data.count || 0;
      } catch (e: any) {
        console.error('Failed to fetch Immich photo count', e);
      }
    },

    async fetchAlbums() {
      this.loading = true;
      this.error = null;
      try {
        const res = await api.get('/immich/albums');
        this.albums = res.data;
      } catch (e: any) {
        this.error = e.response?.data?.error || e.message;
        throw e;
      } finally {
        this.loading = false;
      }
    },

    async testConnection() {
      this.loading = true;
      try {
        const res = await api.post('/immich/test');
        await this.fetchCount();
        return res.data;
      } catch (e: any) {
        throw e;
      } finally {
        this.loading = false;
      }
    },

    async sync() {
      this.loading = true;
      try {
        await api.post('/immich/sync');
        await this.fetchCount();
      } catch (e: any) {
        throw e;
      } finally {
        this.loading = false;
      }
    },
  },
});
