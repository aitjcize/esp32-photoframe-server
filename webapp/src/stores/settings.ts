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
      // NOTE: deliberately does NOT toggle `loading`. The settings panel is
      // gated on `loading` for the INITIAL fetch only; flipping it on every
      // save (e.g. the debounced album auto-save) would unmount/remount the
      // whole panel and reset the user's scroll position.
      try {
        await updateSettings(newSettings);
        this.settings = { ...this.settings, ...newSettings };
      } catch (err: any) {
        this.error = err.message;
        throw err;
      }
    },
  },
});
