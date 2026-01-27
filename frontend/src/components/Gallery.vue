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
          <h2 class="text-h6">Photo Gallery</h2>
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
            color="primary"
            variant="flat"
            height="40"
            :loading="galleryStore.loading"
            :disabled="galleryStore.loading"
            prepend-icon="mdi-google-photos"
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

            <v-btn
              icon="mdi-close"
              color="error"
              size="small"
              variant="flat"
              class="position-absolute delete-btn"
              style="top: -12px; right: -12px; z-index: 10"
              elevation="3"
              @click="galleryStore.deletePhoto(photo.id)"
            ></v-btn>
          </v-card>
        </v-col>
      </v-row>

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
          Get started by adding photos from Google Photos.
        </p>
        <v-btn
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
.photo-card .delete-btn {
  opacity: 0;
  transition: opacity 0.2s ease-in-out;
}

.photo-card:hover .delete-btn {
  opacity: 1;
}

@media (min-width: 1280px) {
  .v-col-lg-custom {
    flex: 0 0 12.5%;
    max-width: 12.5%;
  }
}
</style>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useSettingsStore } from '../stores/settings';
import { useAuthStore } from '../stores/auth';
import { useGalleryStore } from '../stores/gallery';

const store = useSettingsStore();
const authStore = useAuthStore();
const galleryStore = useGalleryStore();

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
