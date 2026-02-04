import { defineStore } from 'pinia';
import { api } from '../api';
import { useSettingsStore } from './settings';

export const useSynologyStore = defineStore('synology', {
  state: () => ({
    count: 0,
    albums: [] as any[],
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchCount() {
      try {
        const res = await api.get('/synology/count');
        this.count = res.data.count || 0;
      } catch (e: any) {
        console.error('Failed to fetch Synology photo count', e);
      }
    },

    async fetchAlbums() {
      this.loading = true;
      this.error = null;
      try {
        // Ensure settings are saved/fresh?
        // Logic in Settings.vue was "saveSettingsInternal()" before loading.
        // We assume the caller handles saving settings mostly, or we rely on backend having latest.
        const res = await api.get('/synology/albums');
        this.albums = res.data;
      } catch (e: any) {
        if (e.response && e.response.status === 401) {
          // Let the component handle UI feedback for now, or throw
          throw new Error('Session expired');
        }
        this.error = e.response?.data?.error || e.message;
        throw e;
      } finally {
        this.loading = false;
      }
    },

    async testConnection(otpCode: string) {
      this.loading = true;
      try {
        const res = await api.post('/synology/test', { otp_code: otpCode });
        // Start using settings store to refresh settings as SID might be updated
        const settingsStore = useSettingsStore();
        await settingsStore.fetchSettings();
        // also fetch count
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
        await api.post('/synology/sync');
        await this.fetchCount();
      } catch (e: any) {
        throw e;
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
      } catch (e: any) {
        throw e;
      } finally {
        this.loading = false;
      }
    },
  },
});
