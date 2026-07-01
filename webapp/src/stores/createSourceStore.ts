import { defineStore } from 'pinia';
import { api } from '../api';

// Shared Pinia store factory for "search-topic" image sources (ARTIC, Unsplash,
// Pexels). Each user-entered topic becomes a synced album (external_id = the
// topic text) — structurally identical to Immich/Synology synced albums, so the
// generic /albums?source= + AlbumPicker machinery applies unchanged.
//
// Immich/Synology are NOT built from this factory: they have source-specific
// shapes (sync-albums payloads with favorites/all/memories modes, connection
// tests, OTP/2FA, logout) that don't fit a topic model. Kept separate on
// purpose; this factory covers only the topic-source common denominator.
export const createSourceStore = (source: string) =>
  defineStore(`source-${source}`, {
    state: () => ({
      count: 0,
      // Synced albums for this source (each corresponds to a topic).
      syncedAlbums: [] as any[],
      loading: false,
      error: null as string | null,
    }),
    actions: {
      async fetchCount() {
        try {
          const res = await api.get(`/${source}/count`);
          this.count = res.data.count || 0;
        } catch (e) {
          console.error(`Failed to fetch ${source} photo count`, e);
        }
      },

      async fetchSyncedAlbums() {
        try {
          const res = await api.get(`/albums?source=${source}`);
          this.syncedAlbums = res.data || [];
          return this.syncedAlbums;
        } catch (e) {
          console.error(`Failed to fetch synced ${source} albums`, e);
          throw e;
        }
      },

      // Replace the synced topic set. Each topic becomes an album with
      // external_id = the topic text. Does NOT import — call sync() after.
      async setSyncTopics(topics: string[]) {
        this.loading = true;
        try {
          await api.post(`/${source}/sync-topics`, { topics });
          await this.fetchSyncedAlbums();
        } finally {
          this.loading = false;
        }
      },

      async sync() {
        this.loading = true;
        try {
          await api.post(`/${source}/sync`);
          await this.fetchCount();
        } finally {
          this.loading = false;
        }
      },

      async syncStatus() {
        try {
          const res = await api.get(`/${source}/sync-status`);
          return res.data;
        } catch (e) {
          console.error(`Failed to fetch ${source} sync status`, e);
          throw e;
        }
      },

      async clear() {
        this.loading = true;
        try {
          await api.post(`/${source}/clear`);
          await this.fetchCount();
          await this.fetchSyncedAlbums();
        } finally {
          this.loading = false;
        }
      },
    },
  });

export const useArticStore = createSourceStore('artic');
export const useUnsplashStore = createSourceStore('unsplash');
export const usePexelsStore = createSourceStore('pexels');
