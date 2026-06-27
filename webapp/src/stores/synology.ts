import { defineStore } from 'pinia';
import { api } from '../api';
import { useSettingsStore } from './settings';
import { getApiError } from '../utils/errors';

export const useSynologyStore = defineStore('synology', {
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
        const res = await api.get('/synology/count');
        this.count = res.data.count || 0;
      } catch (e) {
        console.error('Failed to fetch Synology photo count', e);
      }
    },

    async fetchAlbums() {
      this.loading = true;
      this.error = null;
      try {
        const res = await api.get('/synology/albums');
        this.albums = res.data;
      } catch (e: any) {
        if (e.response && e.response.status === 401) {
          throw new Error('Session expired');
        }
        this.error = getApiError(e);
        throw e;
      } finally {
        this.loading = false;
      }
    },

    async fetchSyncedAlbums() {
      try {
        const res = await api.get('/albums?source=synology_photos');
        this.syncedAlbums = res.data || [];
        return this.syncedAlbums;
      } catch (e) {
        console.error('Failed to fetch synced Synology albums', e);
        throw e;
      }
    },

    async saveSyncAlbums(album_ids: string[]) {
      this.loading = true;
      try {
        await api.post('/synology/sync-albums', { album_ids });
        await this.fetchSyncedAlbums();
        await this.fetchCount();
      } finally {
        this.loading = false;
      }
    },

    async testConnection(otpCode: string) {
      this.loading = true;
      try {
        const res = await api.post('/synology/test', { otp_code: otpCode });
        // SID may be updated server-side; refresh settings view.
        const settingsStore = useSettingsStore();
        await settingsStore.fetchSettings();
        await this.fetchCount();
        return res.data;
      } finally {
        this.loading = false;
      }
    },

    async sync() {
      this.loading = true;
      try {
        await api.post('/synology/sync');
        await this.fetchCount();
      } finally {
        this.loading = false;
      }
    },

    async logout() {
      this.loading = true;
      try {
        await api.post('/synology/logout');
        const settingsStore = useSettingsStore();
        await settingsStore.fetchSettings(); // Refresh to clear SID from local state view
        this.count = 0;
        this.albums = [];
        this.syncedAlbums = [];
      } finally {
        this.loading = false;
      }
    },
  },
});
