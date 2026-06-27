import { defineStore } from 'pinia';
import { api } from '../api';
import { getApiError } from '../utils/errors';

export const useImmichStore = defineStore('immich', {
  state: () => ({
    count: 0,
    albums: [] as any[],
    syncedAlbums: [] as any[],
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchCount() {
      try {
        const res = await api.get('/immich/count');
        this.count = res.data.count || 0;
      } catch (e) {
        console.error('Failed to fetch Immich photo count', e);
      }
    },

    async fetchAlbums() {
      this.loading = true;
      this.error = null;
      try {
        const res = await api.get('/immich/albums');
        this.albums = res.data;
      } catch (e) {
        this.error = getApiError(e);
        throw e;
      } finally {
        this.loading = false;
      }
    },

    async fetchSyncedAlbums() {
      try {
        const res = await api.get('/albums?source=immich');
        this.syncedAlbums = res.data || [];
        return this.syncedAlbums;
      } catch (e) {
        console.error('Failed to fetch synced Immich albums', e);
        throw e;
      }
    },

    async saveSyncAlbums(payload: {
      album_ids: string[];
      favorites: boolean;
      all: boolean;
      memories: boolean;
    }) {
      this.loading = true;
      try {
        await api.post('/immich/sync-albums', payload);
        await this.fetchSyncedAlbums();
        await this.fetchCount();
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
      } finally {
        this.loading = false;
      }
    },

    async sync() {
      this.loading = true;
      try {
        await api.post('/immich/sync');
        await this.fetchCount();
      } finally {
        this.loading = false;
      }
    },
  },
});
