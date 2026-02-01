<template>
  <div>
    <!-- Telegram Mode Notice -->
    <v-alert
      v-if="store.settings.source === 'telegram'"
      type="info"
      variant="tonal"
      class="mb-4"
    >
      <div class="text-subtitle-1 font-weight-bold mb-1">
        Telegram Mode Active
      </div>
      <div class="text-body-2">
        The frame is currently displaying photos sent to your Telegram Bot.
        <br />
        Go to <b>Settings</b> to switch back to Google Photos mode.
      </div>
    </v-alert>

    <!-- Gallery Content -->
    <div v-else>
      <!-- Header with Stats and Actions -->
      <div class="d-flex justify-space-between align-center mb-4">
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
        </div>
        <div class="d-flex gap-2 ga-2">
          <v-btn
            v-if="galleryStore.totalPhotos > 0"
            color="error"
            variant="flat"
            height="40"
            prepend-icon="mdi-delete"
            @click="galleryStore.deleteAllPhotos"
          >
            Delete All
          </v-btn>
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

      <!-- Photo Grid -->
      <v-row v-if="galleryStore.photos.length > 0">
        <v-col
          v-for="photo in galleryStore.photos"
          :key="photo.id"
          class="v-col-6 v-col-sm-4 v-col-md-3 v-col-lg-custom"
        >
          <v-card
            class="position-relative photo-card overflow-visible"
            elevation="2"
            @click="openPushDialog(photo.id)"
          >
            <v-img
              :src="getThumbnailUrl(photo.thumbnail_url)"
              :lazy-src="getThumbnailUrl(photo.thumbnail_url)"
              aspect-ratio="1"
              cover
              class="bg-grey-lighten-2 rounded"
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

            <!-- Delete Badge (Top Right) -->
            <v-btn
              icon="mdi-close"
              color="error"
              size="x-small"
              variant="flat"
              class="action-btn delete-btn"
              title="Delete"
              density="comfortable"
              elevation="4"
              @click.stop="galleryStore.deletePhoto(photo.id)"
            ></v-btn>
          </v-card>
        </v-col>
      </v-row>

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
      <div v-if="galleryStore.totalPhotos === 0" class="text-center py-10">
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
          <span v-else>
            Use the <b>Sync Now</b> button above to import photos from Synology.
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
      </div>
    </div>
  </div>
</template>

<style scoped>
.photo-card {
  transition: transform 0.2s;
  cursor: pointer;
}

.photo-card:hover {
  transform: translateY(-2px);
}

.action-btn {
  position: absolute;
  pointer-events: auto;
  opacity: 0;
  transition:
    opacity 0.2s,
    transform 0.1s;
  z-index: 10;
}

.delete-btn {
  top: -8px;
  right: -8px;
  border-radius: 50%;
  min-width: 24px;
  width: 24px;
  height: 24px;
  padding: 0;
}

/* Show buttons on card hover */
.photo-card:hover .action-btn {
  opacity: 1;
}

.action-btn:hover {
  transform: scale(1.1);
}

@media (min-width: 1280px) {
  .v-col-lg-custom {
    flex: 0 0 12.5%;
    max-width: 12.5%;
  }
}
</style>

<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue';
import { useSettingsStore } from '../stores/settings';
import { useAuthStore } from '../stores/auth';
import { useGalleryStore } from '../stores/gallery';
import { listDevices, pushToDevice, type Device } from '../api';

const store = useSettingsStore();
const authStore = useAuthStore();
const galleryStore = useGalleryStore();

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
});
</script>
