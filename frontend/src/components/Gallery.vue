<template>
  <div>
    <!-- Telegram Mode Notice -->
    <div
      v-if="store.settings.source === 'telegram'"
      class="bg-blue-50 border-l-4 border-blue-500 p-4 mb-5"
    >
      <h3 class="font-semibold text-blue-900 mb-2">Telegram Mode Active</h3>
      <p class="text-sm text-blue-800">
        The frame is currently displaying photos sent to your Telegram Bot.
        <br />
        Go to <b>Settings</b> to switch back to Google Photos mode.
      </p>
    </div>

    <!-- Gallery Content -->
    <div v-else>
      <!-- Header with Stats and Actions -->
      <div class="flex justify-between items-center mb-5">
        <div>
          <h2 class="text-xl font-semibold text-gray-800">Photo Gallery</h2>
          <p class="text-sm text-gray-500 mt-1">
            {{ galleryStore.totalPhotos }} photo{{
              galleryStore.totalPhotos !== 1 ? 's' : ''
            }}
            total
          </p>
        </div>
        <div class="flex gap-3">
          <button
            v-if="galleryStore.totalPhotos > 0"
            @click="galleryStore.deleteAllPhotos"
            class="px-4 py-2 bg-red-600 text-white font-semibold rounded-lg hover:bg-red-700 transition"
          >
            Delete All
          </button>
          <button
            @click="galleryStore.startPicker"
            :disabled="galleryStore.loading"
            class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="galleryStore.loading">Processing...</span>
            <span v-else>Add Photos via Google</span>
          </button>
        </div>
      </div>

      <!-- Notification -->
      <!-- Notification -->
      <div
        v-if="galleryStore.importMessage"
        :class="
          galleryStore.importMessage.includes('Error') ||
          galleryStore.importMessage.includes('Failed')
            ? 'bg-red-100 text-red-700'
            : 'bg-green-100 text-green-700'
        "
        class="p-3 rounded-lg mb-5"
      >
        {{ galleryStore.importMessage }}
      </div>

      <!-- Photo Grid -->
      <div
        v-if="galleryStore.photos.length > 0"
        class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4"
      >
        <div
          v-for="photo in galleryStore.photos"
          :key="photo.id"
          class="relative group border-2 border-gray-200 rounded-lg p-2 bg-gray-50 hover:border-primary-500 hover:-translate-y-1 hover:shadow-lg transition-all cursor-pointer"
        >
          <img
            :src="getThumbnailUrl(photo.thumbnail_url)"
            :alt="photo.caption"
            class="w-full h-auto object-contain rounded border border-gray-300"
            loading="lazy"
          />

          <!-- Delete Button -->
          <button
            @click="galleryStore.deletePhoto(photo.id)"
            title="Delete Photo"
            class="absolute -top-2 -right-2 w-7 h-7 bg-white text-gray-600 border-2 border-gray-200 rounded-full flex items-center justify-center text-xl leading-none opacity-0 group-hover:opacity-100 hover:bg-white hover:text-red-500 hover:border-red-300 hover:scale-110 transition-all shadow-md"
          >
            ×
          </button>
        </div>
      </div>

      <!-- Pagination Controls -->
      <!-- Pagination Controls -->
      <div
        v-if="galleryStore.totalPhotos > galleryStore.limit"
        class="flex justify-center items-center gap-4 mt-6"
      >
        <button
          @click="galleryStore.previousPage"
          :disabled="galleryStore.page === 1"
          class="px-4 py-2 bg-gray-200 text-gray-700 font-medium rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed transition"
        >
          Previous
        </button>
        <span class="text-gray-700">
          Page {{ galleryStore.page }} of {{ galleryStore.totalPages }}
        </span>
        <button
          @click="galleryStore.nextPage"
          :disabled="galleryStore.page === galleryStore.totalPages"
          class="px-4 py-2 bg-gray-200 text-gray-700 font-medium rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed transition"
        >
          Next
        </button>
      </div>

      <!-- Empty State -->
      <div v-if="galleryStore.totalPhotos === 0" class="text-center py-10">
        <h3 class="text-xl font-medium text-gray-700 mb-2">No photos</h3>
        <p class="text-gray-500 mb-5">
          Get started by adding photos from Google Photos.
        </p>
        <button
          @click="galleryStore.startPicker"
          class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all"
        >
          Add Photos
        </button>
      </div>
    </div>
  </div>
</template>

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
  await store.fetchSettings();
  galleryStore.fetchPhotos();
});
</script>
