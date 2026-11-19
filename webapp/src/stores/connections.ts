import { defineStore } from 'pinia';
import { api } from '../api';

// One configured instance of a connection-backed source: an Immich server, a
// Synology Photos account, a Google account (for Photos, or for Calendar), or
// an Unsplash / Pexels API key.
// `config` is the server's redacted view (URLs and flags, never secrets;
// `has_api_key` / `has_password` say whether one is stored; Google: the
// account `email`) and `connected` is whether it can be used right now.
export interface Connection {
  id: number;
  source:
    | 'immich'
    | 'nextcloud_memories'
    | 'synology_photos'
    | 'google_photos'
    | 'google_calendar'
    | 'unsplash'
    | 'pexels'
    | string;
  name: string;
  enabled: boolean;
  connected: boolean;
  config: Record<string, any>;
  created_at: string;
}

// The photo sources that are configured as connections (the source picker
// and the add dialog list these).
export const CONNECTION_SOURCES = [
  'immich',
  'nextcloud_memories',
  'synology_photos',
  'google_photos',
  'unsplash',
  'pexels',
];
// A Google account whose calendar frames can show: a connection too, but
// not a photo source; it lives on the Calendar tab.
export const CALENDAR_SOURCE = 'google_calendar';
// The Google sources: both sign in through /connections/:id/google/login.
export const GOOGLE_SOURCES = ['google_photos', CALENDAR_SOURCE];
// The connection sources whose albums are search topics typed by the user.
export const TOPIC_SOURCES = ['unsplash', 'pexels'];
// The connection sources that sync albums in the background (Google imports
// through the picker instead and has no sync status).
export const SYNCING_SOURCES = [
  'immich',
  'nextcloud_memories',
  'synology_photos',
  ...TOPIC_SOURCES,
];
// The connection sources that offer library-wide collections (All Photos,
// Favorites, On This Day) alongside their albums.
export const VIRTUAL_COLLECTION_SOURCES = ['immich', 'nextcloud_memories'];
// Album pickers fed by a connection's remote albums (as opposed to typed
// topics or Google's picker).
export const ALBUM_SOURCES = [
  'immich',
  'nextcloud_memories',
  'synology_photos',
];

export const SOURCE_LABELS: Record<string, string> = {
  immich: 'Immich',
  nextcloud_memories: 'Nextcloud Memories',
  synology_photos: 'Synology Photos',
  google_photos: 'Google Photos',
  google_calendar: 'Google Calendar',
  unsplash: 'Unsplash',
  pexels: 'Pexels',
};

export const SOURCE_ICONS: Record<string, string> = {
  immich: 'mdi-server',
  nextcloud_memories: 'mdi-cloud',
  synology_photos: 'mdi-nas',
  google_photos: 'mdi-google',
  google_calendar: 'mdi-calendar-account',
  unsplash: 'mdi-camera-iris',
  pexels: 'mdi-camera',
};

// What adding a connection of each source gets you (the add dialog's list).
export const SOURCE_DESCRIPTIONS: Record<string, string> = {
  immich: 'Sync albums and collections from an Immich server',
  nextcloud_memories:
    'Sync albums and collections from the Memories app on Nextcloud',
  synology_photos: 'Sync albums from a Synology Photos account',
  google_photos: 'Pick photos from a Google account',
  google_calendar: "Show a Google account's calendar on your frames",
  unsplash: 'Search topics on Unsplash with your API access key',
  pexels: 'Search topics on Pexels with your API key',
};

// One line telling two connections of the same source apart: the server,
// the account, or (for an API key) nothing better than the source name.
export function connectionSubtitle(c: Connection): string {
  const cfg = c.config || {};
  switch (c.source) {
    case 'immich':
      return cfg.url || '';
    case 'synology_photos':
      return cfg.account && cfg.url
        ? `${cfg.account} @ ${cfg.url}`
        : cfg.url || '';
    case 'nextcloud_memories':
      return cfg.username && cfg.url
        ? `${cfg.username} @ ${cfg.url}`
        : cfg.url || '';
    case 'google_photos':
    case 'google_calendar':
      return cfg.email || '';
    default:
      return '';
  }
}

export const useConnectionsStore = defineStore('connections', {
  state: () => ({
    connections: [] as Connection[],
    loaded: false,
    loading: false,
    // Photo count per connection id, filled lazily by the cards.
    counts: {} as Record<number, number>,
  }),
  getters: {
    byId: (state) => (id: number) => state.connections.find((c) => c.id === id),
    bySource: (state) => (source: string) =>
      state.connections.filter((c) => c.source === source),
    // Albums of a source can come from several connections; the pickers
    // prefix album names with the connection name when that is the case.
    hasSeveral: (state) => (source: string) =>
      state.connections.filter((c) => c.source === source).length > 1,
  },
  actions: {
    upsert(c: Connection) {
      const i = this.connections.findIndex((x) => x.id === c.id);
      if (i >= 0) this.connections[i] = c;
      else this.connections.push(c);
    },

    async fetch() {
      this.loading = true;
      try {
        const res = await api.get('/connections');
        this.connections = res.data || [];
        this.loaded = true;
      } finally {
        this.loading = false;
      }
    },

    async create(payload: {
      source: string;
      name?: string;
      config?: Record<string, any>;
    }): Promise<Connection> {
      const res = await api.post('/connections', payload);
      this.upsert(res.data);
      return res.data;
    },

    async update(
      id: number,
      payload: {
        name?: string;
        enabled?: boolean;
        config?: Record<string, any>;
      }
    ): Promise<Connection> {
      const res = await api.put(`/connections/${id}`, payload);
      this.upsert(res.data);
      return res.data;
    },

    async remove(id: number) {
      await api.delete(`/connections/${id}`);
      this.connections = this.connections.filter((c) => c.id !== id);
      delete this.counts[id];
    },

    async refreshOne(id: number) {
      const res = await api.get(`/connections/${id}`);
      this.upsert(res.data);
      return res.data as Connection;
    },

    // Verifies the credentials against the server (Synology: logs in; the
    // one-time code is for accounts with 2FA). Returns the refreshed view.
    async test(id: number, otpCode = ''): Promise<Connection> {
      const res = await api.post(
        `/connections/${id}/test`,
        otpCode ? { otp_code: otpCode } : {}
      );
      this.upsert(res.data);
      return res.data;
    },

    async logout(id: number) {
      await api.post(`/connections/${id}/logout`);
      await this.refreshOne(id);
    },

    // Google: the consent page URL for this connection. The browser goes
    // there and Google sends it back to the dashboard once authorised.
    async googleLoginUrl(id: number): Promise<string> {
      const res = await api.get(`/connections/${id}/google/login`);
      return res.data.url;
    },

    // Nextcloud: start Login Flow v2 for the connection's URL; returns the
    // page where the user signs in and grants access.
    async nextcloudLoginStart(id: number): Promise<string> {
      const res = await api.post(`/connections/${id}/nextcloud/login`);
      return res.data.login_url;
    },

    // Nextcloud: the sign-in's progress (idle | pending | done | failed).
    async nextcloudLoginStatus(
      id: number
    ): Promise<{ state: string; login_url?: string; error?: string }> {
      const res = await api.get(`/connections/${id}/nextcloud/login`);
      return res.data;
    },

    // Google: the connected accounts, i.e. the ones a picker can import into.
    googleConnected(): Connection[] {
      return this.bySource('google_photos').filter((c) => c.connected);
    },

    // Google Calendar: the calendars of one connected account.
    async calendars(id: number): Promise<any[]> {
      const res = await api.get(`/connections/${id}/calendars`);
      return res.data || [];
    },

    async fetchCount(id: number) {
      try {
        const res = await api.get(`/connections/${id}/count`);
        this.counts[id] = res.data.count || 0;
      } catch (e) {
        console.error('Failed to fetch photo count', e);
      }
    },

    // The albums the remote server offers, in the source's own shape
    // (Immich: id/albumName; Synology: id/name/shared_with_me).
    async remoteAlbums(id: number): Promise<any[]> {
      const res = await api.get(`/connections/${id}/remote-albums`);
      return res.data || [];
    },

    // The persisted album rows of one connection (with sync_enabled).
    async syncedAlbums(id: number): Promise<any[]> {
      const res = await api.get(`/albums?connection_id=${id}`);
      return res.data || [];
    },

    async saveSyncAlbums(
      id: number,
      payload: {
        album_ids: string[];
        favorites?: boolean;
        all?: boolean;
        memories?: boolean;
      }
    ) {
      await api.post(`/connections/${id}/sync-albums`, payload);
    },

    // Unsplash / Pexels: the connection's search topics, each a synced album.
    async topics(id: number): Promise<string[]> {
      const res = await api.get(`/connections/${id}/topics`);
      return res.data?.topics || [];
    },

    // Replaces the topic set; a removed topic's photos go with it. Nothing is
    // imported until the next sync.
    async setTopics(id: number, topics: string[]): Promise<string[]> {
      const res = await api.post(`/connections/${id}/topics`, { topics });
      return res.data?.topics || [];
    },

    async sync(id: number) {
      await api.post(`/connections/${id}/sync`);
    },

    async syncStatus(
      id: number
    ): Promise<{ running: boolean; last_error: string }> {
      const res = await api.get(`/connections/${id}/sync-status`);
      return {
        running: !!res.data?.running,
        last_error:
          typeof res.data?.last_error === 'string' ? res.data.last_error : '',
      };
    },

    // Deletes the connection's photos and turns its album sync off, so an
    // auto-sync does not re-import them (same as the gallery's Delete All).
    async clearPhotos(id: number) {
      await api.delete(`/gallery/photos?connection_id=${id}`);
      this.counts[id] = 0;
    },

    // Deletes the connection's photos but leaves its albums (topics) syncing,
    // so the next sync fills them again.
    async clearPhotosKeepSync(id: number) {
      await api.post(`/connections/${id}/clear`);
      this.counts[id] = 0;
    },
  },
});
