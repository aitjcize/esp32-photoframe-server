<template>
  <div>
    <!-- Gallery Content -->
    <div>
      <!-- Header with Stats and Actions -->
      <div
        class="d-flex flex-column flex-sm-row justify-space-between align-start align-sm-center ga-2 mb-4"
      >
        <div>
          <h2 class="text-h6 text-capitalize">
            {{ galleryStore.source.replace('_', ' ') }} Gallery
          </h2>
          <div class="text-caption text-grey">
            {{ galleryStore.totalPhotos }} photo{{
              galleryStore.totalPhotos !== 1 ? 's' : ''
            }}
            total
          </div>
          <div
            v-if="syncing"
            class="text-caption text-primary d-flex align-center ga-1 mt-1"
          >
            <v-progress-circular
              indeterminate
              size="14"
              width="2"
              color="primary"
            ></v-progress-circular>
            Syncing…
          </div>
        </div>
        <div class="d-flex flex-wrap ga-2">
          <v-btn
            v-if="galleryStore.source === 'gallery'"
            color="primary"
            variant="flat"
            height="40"
            :loading="galleryStore.loading"
            :disabled="galleryStore.loading"
            prepend-icon="mdi-upload"
            @click="triggerUpload"
          >
            Upload Photos
          </v-btn>
          <input
            ref="uploadInput"
            type="file"
            accept="image/*"
            multiple
            class="d-none"
            @change="onFilesSelected"
          />
          <v-btn
            v-if="galleryStore.source === 'google_photos'"
            color="primary"
            variant="flat"
            height="40"
            :loading="galleryStore.loading"
            :disabled="galleryStore.loading"
            prepend-icon="mdi-google"
            @click="galleryStore.startPicker"
          >
            Add Photos via Google
          </v-btn>
        </div>
      </div>

      <!-- Notification -->
      <v-alert
        v-if="galleryStore.importMessage"
        :type="
          galleryStore.importMessage.includes('Error') ||
          galleryStore.importMessage.includes('Failed')
            ? 'error'
            : 'success'
        "
        variant="tonal"
        class="mb-4"
        density="compact"
        closable
        @click:close="galleryStore.importMessage = ''"
      >
        {{ galleryStore.importMessage }}
      </v-alert>

      <!-- Album Chips (Immich / Synology) -->
      <div
        v-if="showAlbumChips && albumChips.length > 0"
        class="d-flex align-center flex-wrap ga-2 mb-4"
      >
        <v-chip
          :color="galleryStore.album === null ? 'primary' : undefined"
          :variant="galleryStore.album === null ? 'flat' : 'outlined'"
          @click="selectAlbum(null)"
        >
          All
        </v-chip>
        <v-chip
          v-for="album in albumChips"
          :key="album.id"
          :color="galleryStore.album === album.id ? 'primary' : undefined"
          :variant="galleryStore.album === album.id ? 'flat' : 'outlined'"
          @click="selectAlbum(album.id)"
        >
          {{ album.name
          }}<span v-if="album.asset_count != null" class="ml-1 text-caption"
            >({{ album.asset_count }})</span
          >
        </v-chip>
      </div>

      <!-- Loading Spinner -->
      <div
        v-if="galleryStore.loading"
        class="d-flex justify-center align-center pa-10"
      >
        <v-progress-circular
          indeterminate
          color="primary"
        ></v-progress-circular>
      </div>

      <!-- Photo Grid -->
      <v-row v-else-if="galleryStore.photos.length > 0">
        <v-col
          v-for="photo in galleryStore.photos"
          :key="photo.id"
          class="v-col-6 v-col-sm-4 v-col-md-3 v-col-lg-custom"
        >
          <v-card
            variant="outlined"
            class="position-relative photo-card"
            @click="openPushDialog(photo.id)"
          >
            <v-img
              :src="getThumbnailUrl(photo.thumbnail_url)"
              :lazy-src="getThumbnailUrl(photo.thumbnail_url)"
              aspect-ratio="1"
              contain
              class="bg-grey-lighten-3 rounded"
            >
              <template v-slot:placeholder>
                <div class="d-flex align-center justify-center fill-height">
                  <v-progress-circular
                    color="grey-lighten-4"
                    indeterminate
                  ></v-progress-circular>
                </div>
              </template>
            </v-img>
            <div class="delete-hotspot">
              <v-btn
                icon="mdi-delete"
                size="x-small"
                color="error"
                class="delete-overlay"
                @click.stop="requestDeletePhoto(photo)"
              />
            </div>
          </v-card>
        </v-col>
      </v-row>

      <!-- Delete Image Dialog -->
      <v-dialog v-model="deleteDialog.show" max-width="400">
        <v-card>
          <v-card-title>
            <v-icon icon="mdi-delete" color="error" class="mr-2" />
            Delete Image?
          </v-card-title>
          <v-card-text>
            <div class="mb-3">Are you sure you want to delete this image?</div>
            <div v-if="deleteDialog.photo" class="d-flex justify-center">
              <img
                :src="getThumbnailUrl(deleteDialog.photo.thumbnail_url)"
                alt=""
                class="confirm-thumb"
              />
            </div>
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn variant="text" @click="deleteDialog.show = false">
              Cancel
            </v-btn>
            <v-btn
              color="error"
              :loading="deleteDialog.loading"
              @click="confirmDeletePhoto"
            >
              Delete
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <!-- Push Dialog -->
      <v-dialog v-model="pushDialog.show" max-width="400">
        <v-card>
          <v-card-title>Push to Device</v-card-title>
          <v-card-text>
            <div v-if="loadingDevices" class="d-flex justify-center pa-4">
              <v-progress-circular indeterminate></v-progress-circular>
            </div>
            <div v-else-if="devices.length === 0">
              No devices found. Please add a device in Settings.
            </div>
            <div v-else>
              <v-radio-group v-model="pushDialog.selectedDevice" hide-details>
                <v-radio
                  v-for="dev in devices"
                  :key="dev.id"
                  :label="`${dev.name} (${dev.host})`"
                  :value="dev.id"
                ></v-radio>
              </v-radio-group>

              <v-checkbox
                v-model="pushDialog.remember"
                label="Remember my choice (this session)"
                density="compact"
                hide-details
                class="mt-2"
              ></v-checkbox>

              <v-alert
                v-if="pushDialog.error"
                type="error"
                variant="tonal"
                density="compact"
                class="mt-4"
                closable
                @click:close="pushDialog.error = ''"
              >
                {{ pushDialog.error }}
              </v-alert>
            </div>
          </v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn variant="text" @click="pushDialog.show = false"
              >Cancel</v-btn
            >
            <v-btn
              color="primary"
              :disabled="!pushDialog.selectedDevice"
              :loading="pushDialog.loading"
              @click="confirmPush"
            >
              Push
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <!-- Pagination Controls -->
      <div
        v-if="galleryStore.totalPhotos > galleryStore.limit"
        class="d-flex justify-center mt-6"
      >
        <v-pagination
          v-model="galleryStore.page"
          :length="galleryStore.totalPages"
          :total-visible="5"
          rounded="circle"
          @update:model-value="galleryStore.fetchPhotos"
        ></v-pagination>
      </div>

      <!-- Empty State -->
      <div
        v-if="!galleryStore.loading && galleryStore.totalPhotos === 0"
        class="text-center py-10"
      >
        <v-icon
          icon="mdi-image-off-outline"
          size="64"
          color="grey-lighten-1"
          class="mb-4"
        ></v-icon>
        <h3 class="text-h6 text-grey-darken-1 mb-2">No photos</h3>
        <p class="text-body-2 text-grey mb-4">
          <span v-if="galleryStore.source === 'google_photos'">
            Get started by adding photos from Google Photos.
          </span>
          <span v-else-if="galleryStore.source === 'gallery'">
            Upload photos here, or send them to your Telegram bot.
          </span>
          <span v-else-if="galleryStore.source === 'synology_photos'">
            Open the <b>Synology</b> tab under Data Sources below and click
            <b>Sync Now</b> to import photos.
          </span>
          <span v-else-if="galleryStore.source === 'immich'">
            Open the <b>Immich</b> tab under Data Sources below and click
            <b>Sync Now</b> to import photos.
          </span>
        </p>
        <v-btn
          v-if="galleryStore.source === 'google_photos'"
          color="primary"
          prepend-icon="mdi-plus"
          @click="galleryStore.startPicker"
        >
          Add Photos
        </v-btn>
        <v-btn
          v-else-if="galleryStore.source === 'gallery'"
          color="primary"
          prepend-icon="mdi-upload"
          @click="triggerUpload"
        >
          Upload Photos
        </v-btn>
      </div>
    </div>
  </div>
</template>

<style scoped>
.photo-card {
  cursor: pointer;
  transition:
    transform 0.2s,
    box-shadow 0.2s;
}

.photo-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.delete-hotspot {
  position: absolute;
  top: 0;
  right: 0;
  width: 48px;
  height: 48px;
  z-index: 1;
}

.delete-overlay {
  position: absolute;
  top: 4px;
  right: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}

.delete-hotspot:hover .delete-overlay {
  opacity: 1;
}

.confirm-thumb {
  max-width: 100%;
  max-height: 60vh;
  display: block;
}

@media (min-width: 1280px) {
  .v-col-lg-custom {
    flex: 0 0 16.6667%;
    max-width: 16.6667%;
  }
}
</style>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, reactive, computed, watch } from 'vue';
import { useAuthStore } from '../stores/auth';
import { useGalleryStore } from '../stores/gallery';
import { api, listDevices, pushToDevice, type Device } from '../api';

const authStore = useAuthStore();
const galleryStore = useGalleryStore();

// Album chips (only shown for Immich / Synology sources).
const albumChips = ref<any[]>([]);

const showAlbumChips = computed(
  () =>
    galleryStore.source === 'immich' ||
    galleryStore.source === 'synology_photos'
);

const fetchAlbumChips = async () => {
  if (!showAlbumChips.value) {
    albumChips.value = [];
    return;
  }
  try {
    const res = await api.get(
      `/albums?source=${galleryStore.source}&synced=true`
    );
    // Only show albums that currently have photos — hide ones emptied upstream.
    albumChips.value = (res.data || []).filter(
      (a: any) => (a.asset_count ?? 0) > 0
    );
  } catch (e) {
    console.error('Failed to fetch album chips', e);
    albumChips.value = [];
  }
};

const selectAlbum = (id: number | null) => {
  galleryStore.setAlbum(id);
};

// When the source tab changes, reset album selection and refetch chips.
watch(
  () => galleryStore.source,
  () => {
    prevSyncing = false;
    fetchAlbumChips();
    checkSyncStatus();
  }
);

// --- Auto-refresh while a background sync is running ---
const syncing = ref(false);
let syncPollTimer: number | null = null;
let prevSyncing = false;

const syncStatusEndpoint = (): string | null => {
  if (galleryStore.source === 'immich') return '/immich/sync-status';
  if (galleryStore.source === 'synology_photos') return '/synology/sync-status';
  return null;
};

const checkSyncStatus = async () => {
  const ep = syncStatusEndpoint();
  if (!ep) {
    syncing.value = false;
    prevSyncing = false;
    return;
  }
  try {
    const res = await api.get(ep);
    const running = !!res.data?.running;
    // Refresh while syncing (new photos appear) and once more right after it
    // finishes, so the gallery + album counts stay current without a manual reload.
    if (running || prevSyncing) {
      await galleryStore.fetchPhotos();
      await fetchAlbumChips();
    }
    syncing.value = running;
    prevSyncing = running;
  } catch {
    // ignore transient errors
  }
};

// Settings bumps refreshSignal right after a sync/delete is triggered, so we
// detect it immediately instead of waiting for the next background poll.
watch(
  () => galleryStore.refreshSignal,
  () => {
    // A sync or delete just happened — refetch immediately (a delete is instant
    // and has no sync status to poll, so we can't rely on checkSyncStatus alone)...
    galleryStore.fetchPhotos();
    fetchAlbumChips();
    // ...then poll in case a background sync is still importing more photos.
    syncing.value = true;
    prevSyncing = true;
    checkSyncStatus();
  }
);

const uploadInput = ref<HTMLInputElement | null>(null);

const triggerUpload = () => {
  uploadInput.value?.click();
};

const onFilesSelected = async (e: Event) => {
  const target = e.target as HTMLInputElement;
  if (!target.files || target.files.length === 0) return;
  await galleryStore.uploadFiles(target.files);
  target.value = '';
};

const deleteDialog = reactive({
  show: false,
  photo: null as any,
  loading: false,
});

const requestDeletePhoto = (photo: any) => {
  deleteDialog.photo = photo;
  deleteDialog.show = true;
};

const confirmDeletePhoto = async () => {
  if (!deleteDialog.photo) return;
  deleteDialog.loading = true;
  try {
    await galleryStore.deletePhoto(deleteDialog.photo.id);
    deleteDialog.show = false;
    deleteDialog.photo = null;
  } finally {
    deleteDialog.loading = false;
  }
};

// Push Dialog State
const devices = ref<Device[]>([]);
const loadingDevices = ref(false);
const pushDialog = reactive({
  show: false,
  imageId: 0,
  selectedDevice: null as number | null,
  remember: false,
  loading: false,
  error: '',
});

// Session memory for device preference
const SESSION_KEY_PREFERRED_DEVICE = 'photoframe_preferred_device';

const openPushDialog = async (imageId: number) => {
  pushDialog.imageId = imageId;
  pushDialog.error = ''; // Clear previous error

  // Check session preference
  const savedId = sessionStorage.getItem(SESSION_KEY_PREFERRED_DEVICE);
  if (savedId) {
    const id = parseInt(savedId);
    if (!isNaN(id)) {
      // Auto-push could go here if implemented
    }
  }

  pushDialog.show = true;
  loadingDevices.value = true;

  try {
    const list = await listDevices();
    devices.value = list;

    // If we have a saved preference and it's in the list, pre-select it
    if (savedId) {
      const found = list.find((d: Device) => d.id === parseInt(savedId));
      if (found) {
        pushDialog.selectedDevice = found.id;
      }
    }

    // If no selection yet and only 1 device, pre-select it
    if (!pushDialog.selectedDevice && list.length === 1) {
      pushDialog.selectedDevice = list[0].id;
    }
  } catch (e) {
    console.error(e);
    pushDialog.error = 'Failed to load devices';
  } finally {
    loadingDevices.value = false;
  }
};

const confirmPush = async () => {
  if (!pushDialog.selectedDevice) return;

  pushDialog.error = ''; // Clear previous error

  if (pushDialog.remember) {
    sessionStorage.setItem(
      SESSION_KEY_PREFERRED_DEVICE,
      String(pushDialog.selectedDevice)
    );
  }

  pushDialog.loading = true;
  try {
    await pushToDevice(pushDialog.selectedDevice, pushDialog.imageId);
    galleryStore.importMessage = 'Image pushed to device successfully';
    pushDialog.show = false;
  } catch (e: any) {
    // Extract error message
    let msg = 'Failed to push image';
    if (e.response && e.response.data && e.response.data.error) {
      msg = e.response.data.error;
    } else if (e.message) {
      msg = e.message;
    }
    pushDialog.error = msg;
    // Keep dialog open to show error
  } finally {
    pushDialog.loading = false;
  }
};

const getThumbnailUrl = (url: string) => {
  const token = authStore.token;
  if (!token) return url;
  // If url already has params, append with &
  const separator = url.includes('?') ? '&' : '?';
  return `${url}${separator}token=${token}`;
};

onMounted(async () => {
  // store.fetchSettings() is called by parent (Settings.vue) or app init.
  // Calling it here triggers a loading state loop if this component is mounted inside Settings.vue
  galleryStore.fetchPhotos();
  fetchAlbumChips();
  checkSyncStatus();
  syncPollTimer = window.setInterval(checkSyncStatus, 6000);
});

onUnmounted(() => {
  if (syncPollTimer != null) {
    window.clearInterval(syncPollTimer);
    syncPollTimer = null;
  }
});
</script>
