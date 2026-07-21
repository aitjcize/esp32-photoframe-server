import { defineStore } from 'pinia';
import { api } from '../api';
import { getApiError } from '../utils/errors';

interface CacheStatus {
  enabled: boolean;
  count: number;
  size_bytes: number;
  size_human: string;
  max_images: number;
  max_size_mb: number;
  populating: boolean;
}

export const useImmichStore = defineStore('immich', {
  state: () => ({
    count: 0,
    albums: [] as any[],
    syncedAlbums: [] as any[],
    loading: false,
    error: null as string | null,
    cacheStatus: null as CacheStatus | null,
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

    async fetchCacheStatus() {
      try {
        const res = await api.get('/immich/cache/status');
        this.cacheStatus = res.data;
      } catch (e) {
        console.error('Failed to fetch Immich cache status', e);
      }
    },

    async populateCache() {
      try {
        await api.post('/immich/cache/populate');
        await this.fetchCacheStatus();
      } catch (e) {
        this.error = getApiError(e);
        throw e;
      }
    },

    async clearCache() {
      try {
        await api.post('/immich/cache/clear');
        await this.fetchCacheStatus();
      } catch (e) {
        this.error = getApiError(e);
        throw e;
      }
    },
  },
});
