import { defineStore } from 'pinia';
import { api } from '../api';
import { useSettingsStore } from './settings';
import { useConnectionsStore } from './connections';

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
      | 'unsplash'
      | 'pexels'
      | 'gallery'
      | 'url_proxy',
    // The connection whose photos are shown (connection-backed sources are
    // always viewed one connection at a time); null for the built-in sources.
    connectionId: null as number | null,
    album: null as number | null,
    // Bumped to ask the Gallery view to refetch photos + album chips
    // (e.g. after a Delete All from the source config tab).
    refreshSignal: 0,
    // The gallery's own albums (folders for uploads), for the album manager,
    // the Telegram target and the per-frame picker.
    albums: [] as any[],
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
        | 'unsplash'
        | 'pexels'
        | 'gallery'
        | 'url_proxy',
      connectionId: number | null = null
    ) {
      this.source = source;
      this.connectionId = connectionId;
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
      const reqConnection = this.connectionId;
      const reqAlbum = this.album;
      const reqPage = this.page;
      const isCurrent = () =>
        this.source === reqSource &&
        this.connectionId === reqConnection &&
        this.album === reqAlbum &&
        this.page === reqPage;
      try {
        const offset = (reqPage - 1) * this.limit;
        let url = `/gallery/photos?source=${reqSource}&limit=${this.limit}&offset=${offset}`;
        if (reqConnection != null) {
          url += `&connection_id=${reqConnection}`;
        }
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
        const scope =
          this.connectionId != null
            ? `connection_id=${this.connectionId}`
            : `source=${this.source}`;
        const res = await api.delete(`/gallery/photos?${scope}`);
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

    // --- Gallery albums ---
    async fetchAlbums() {
      try {
        const res = await api.get('/albums?source=gallery');
        this.albums = res.data || [];
      } catch (e) {
        console.error('Failed to fetch gallery albums', e);
      }
    },

    async createAlbum(name: string) {
      const res = await api.post('/gallery/albums', { name });
      await this.fetchAlbums();
      this.triggerRefresh();
      return res.data;
    },

    async renameAlbum(id: number, name: string) {
      await api.put(`/gallery/albums/${id}`, { name });
      await this.fetchAlbums();
      this.triggerRefresh();
    },

    // Without withPhotos the album's photos stay in the gallery as unfiled.
    async deleteAlbum(id: number, withPhotos: boolean) {
      await api.delete(
        `/gallery/albums/${id}${withPhotos ? '?photos=true' : ''}`
      );
      if (this.album === id) this.album = null;
      await this.fetchAlbums();
      this.triggerRefresh();
    },

    // Files one gallery photo under an album (null = unfiled).
    async setPhotoAlbum(photoId: number, albumId: number | null) {
      await api.put(`/gallery/photos/${photoId}/album`, { album_id: albumId });
      await this.fetchAlbums();
      await this.fetchPhotos();
    },

    // albumId files the uploads under that gallery album.
    async uploadFiles(
      files: FileList | File[],
      caption = '',
      albumId: number | null = null
    ) {
      const list = Array.from(files);
      if (list.length === 0) return;

      this.loading = true;
      let added = 0;
      try {
        for (const file of list) {
          const fd = new FormData();
          fd.append('file', file);
          if (caption) fd.append('caption', caption);
          if (albumId != null) fd.append('album_id', String(albumId));
          await api.post('/gallery/upload', fd, {
            headers: { 'Content-Type': 'multipart/form-data' },
          });
          added++;
        }
        this.importMessage = `Uploaded ${added} photo${added === 1 ? '' : 's'}.`;
        setTimeout(() => (this.importMessage = ''), 5000);
        this.page = 1;
        await this.fetchPhotos();
        if (albumId != null) await this.fetchAlbums();
      } catch (e: any) {
        console.error('Upload failed', e);
        this.importMessage =
          e?.response?.data?.error || 'Failed to upload photo';
        setTimeout(() => (this.importMessage = ''), 5000);
      } finally {
        this.loading = false;
      }
    },

    // Opens the Google Photos Picker for one connected Google account and
    // imports what the user picks into it. Called from the Gallery's "Add
    // Photos via Google" (connectionId optional) and from the Google
    // connection cards (their own id).
    async startPicker(connectionId?: number) {
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

      // Which Google account to import into. With one connected account the
      // button goes straight to it; the Gallery offers a menu when there are
      // several, and the connection cards pass their own id.
      const connections = useConnectionsStore();
      if (!connections.loaded) await connections.fetch();
      const connected = connections.googleConnected();
      const target =
        connectionId != null
          ? connected.find((c) => c.id === connectionId)
          : connected.length === 1
            ? connected[0]
            : undefined;
      if (!target) {
        this.importMessage =
          connected.length === 0
            ? 'Connect a Google account under Data Sources → Google first.'
            : 'Choose which Google account to add photos from.';
        setTimeout(() => (this.importMessage = ''), 5000);
        return;
      }

      this.loading = true;
      try {
        const res = await api.get(
          `/connections/${target.id}/google/picker/session`
        );
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

        this.pollPicker(target.id, id);
      } catch (e: any) {
        console.error(e);
        this.importMessage =
          'Failed to start picker flow: ' +
          (e.response?.data?.error || e.message || e);
        this.loading = false;
      }
    },

    pollPicker(connectionId: number, sessionId: string) {
      if (this.pickerTimer) clearInterval(this.pickerTimer);

      const base = `/connections/${connectionId}/google/picker`;
      this.pickerTimer = window.setInterval(async () => {
        try {
          const res = await api.get(`${base}/poll/${sessionId}`);
          const { complete } = res.data;
          if (complete) {
            if (this.pickerTimer) clearInterval(this.pickerTimer);
            await this.processPicker(connectionId, sessionId);
          }
        } catch (e) {
          console.error('Polling error', e);
        }
      }, 2000);
    },

    async processPicker(connectionId: number, sessionId: string) {
      try {
        const res = await api.post(
          `/connections/${connectionId}/google/picker/process/${sessionId}`
        );
        if (res.status === 202) {
          this.pollProgress(connectionId, sessionId);
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

    pollProgress(connectionId: number, sessionId: string) {
      const progressInterval = setInterval(async () => {
        try {
          const pRes = await api.get(
            `/connections/${connectionId}/google/picker/progress/${sessionId}`
          );
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
