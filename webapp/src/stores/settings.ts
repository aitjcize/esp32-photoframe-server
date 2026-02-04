import { defineStore } from 'pinia';
import { getSettings, updateSettings } from '../api';

export const useSettingsStore = defineStore('settings', {
  state: () => ({
    settings: {} as Record<string, string>,
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchSettings() {
      this.loading = true;
      try {
        this.settings = await getSettings();
      } catch (err: any) {
        this.error = err.message;
      } finally {
        this.loading = false;
      }
    },
    async saveSettings(newSettings: Record<string, string>) {
      this.loading = true;
      try {
        await updateSettings(newSettings);
        this.settings = { ...this.settings, ...newSettings };
      } catch (err: any) {
        this.error = err.message;
        throw err;
      } finally {
        this.loading = false;
      }
    },
  },
});
