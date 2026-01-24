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
            {{ totalPhotos }} photo{{ totalPhotos !== 1 ? 's' : '' }} total
          </p>
        </div>
        <div class="flex gap-3">
          <button
            v-if="totalPhotos > 0"
            @click="deleteAllPhotos"
            class="px-4 py-2 bg-red-600 text-white font-semibold rounded-lg hover:bg-red-700 transition"
          >
            Delete All
          </button>
          <button
            @click="startPicker"
            :disabled="loading"
            class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="loading">Processing...</span>
            <span v-else>Add Photos via Google</span>
          </button>
        </div>
      </div>

      <!-- Notification -->
      <div
        v-if="importMessage"
        :class="
          importMessage.includes('Error') || importMessage.includes('Failed')
            ? 'bg-red-100 text-red-700'
            : 'bg-green-100 text-green-700'
        "
        class="p-3 rounded-lg mb-5"
      >
        {{ importMessage }}
      </div>

      <!-- Photo Grid -->
      <div
        v-if="photos.length > 0"
        class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4"
      >
        <div
          v-for="photo in photos"
          :key="photo.id"
          class="relative group border-2 border-gray-200 rounded-lg p-2 bg-gray-50 hover:border-primary-500 hover:-translate-y-1 hover:shadow-lg transition-all cursor-pointer"
        >
          <img
            :src="photo.thumbnail_url"
            :alt="photo.caption"
            class="w-full h-auto object-contain rounded border border-gray-300"
            loading="lazy"
          />

          <!-- Delete Button -->
          <button
            @click="deletePhoto(photo.id)"
            title="Delete Photo"
            class="absolute -top-2 -right-2 w-7 h-7 bg-white text-gray-600 border-2 border-gray-200 rounded-full flex items-center justify-center text-xl leading-none opacity-0 group-hover:opacity-100 hover:bg-white hover:text-red-500 hover:border-red-300 hover:scale-110 transition-all shadow-md"
          >
            ×
          </button>
        </div>
      </div>

      <!-- Pagination Controls -->
      <div
        v-if="totalPhotos > limit"
        class="flex justify-center items-center gap-4 mt-6"
      >
        <button
          @click="previousPage"
          :disabled="currentPage === 1"
          class="px-4 py-2 bg-gray-200 text-gray-700 font-medium rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed transition"
        >
          Previous
        </button>
        <span class="text-gray-700">
          Page {{ currentPage }} of {{ totalPages }}
        </span>
        <button
          @click="nextPage"
          :disabled="currentPage === totalPages"
          class="px-4 py-2 bg-gray-200 text-gray-700 font-medium rounded-lg hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed transition"
        >
          Next
        </button>
      </div>

      <!-- Empty State -->
      <div v-if="totalPhotos === 0" class="text-center py-10">
        <h3 class="text-xl font-medium text-gray-700 mb-2">No photos</h3>
        <p class="text-gray-500 mb-5">
          Get started by adding photos from Google Photos.
        </p>
        <button
          @click="startPicker"
          class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all"
        >
          Add Photos
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useSettingsStore } from '../stores/settings';

const store = useSettingsStore();
const photos = ref<any[]>([]);
const loading = ref(false);
const totalPhotos = ref(0);
const currentPage = ref(1);
const limit = 50;

const totalPages = computed(() => Math.ceil(totalPhotos.value / limit));

// Fetch Photos with Pagination
const fetchPhotos = async () => {
  try {
    const offset = (currentPage.value - 1) * limit;
    const res = await fetch(
      `${import.meta.env.VITE_API_BASE || ''}/api/google-photos?limit=${limit}&offset=${offset}`
    );
    if (res.ok) {
      const data = await res.json();
      photos.value = data.photos || [];
      totalPhotos.value = data.total || 0;
    }
  } catch (e) {
    console.error('Failed to fetch photos', e);
  }
};

const nextPage = () => {
  if (currentPage.value < totalPages.value) {
    currentPage.value++;
    fetchPhotos();
  }
};

const previousPage = () => {
  if (currentPage.value > 1) {
    currentPage.value--;
    fetchPhotos();
  }
};

// Delete Photo
const deletePhoto = async (id: number) => {
  if (!confirm('Are you sure you want to delete this photo?')) return;

  try {
    const res = await fetch(
      `${import.meta.env.VITE_API_BASE || ''}/api/google-photos/${id}`,
      {
        method: 'DELETE',
      }
    );
    if (res.ok) {
      // Refresh current page
      await fetchPhotos();
    } else {
      alert('Failed to delete photo');
    }
  } catch (e) {
    console.error('Failed to delete photo', e);
  }
};

// Delete All Photos
const deleteAllPhotos = async () => {
  if (
    !confirm(
      `Are you sure you want to delete all ${totalPhotos.value} photos? This cannot be undone.`
    )
  )
    return;

  try {
    const res = await fetch(
      `${import.meta.env.VITE_API_BASE || ''}/api/google-photos`,
      {
        method: 'DELETE',
      }
    );
    if (res.ok) {
      const data = await res.json();
      importMessage.value = data.message || 'All photos deleted successfully!';
      setTimeout(() => (importMessage.value = ''), 5000);
      currentPage.value = 1;
      await fetchPhotos();
    } else {
      alert('Failed to delete photos');
    }
  } catch (e) {
    console.error('Failed to delete photos', e);
    alert('Error deleting photos');
  }
};

// Picker Logic (Simplified from Settings.vue)
const pickerTimer = ref<number | null>(null);

const startPicker = async () => {
  // Check Credentials
  if (
    !store.settings.google_client_id ||
    !store.settings.google_client_secret
  ) {
    importMessage.value =
      'Please configure Google Photos Credentials in Settings first.';
    setTimeout(() => (importMessage.value = ''), 5000);
    return;
  }

  loading.value = true;
  try {
    // 1. Create Session
    const res = await fetch(
      `${import.meta.env.VITE_API_BASE || ''}/api/google/picker/session`
    );
    if (!res.ok) throw new Error('Failed to create session');
    const { id, pickerUri } = await res.json();

    // 2. Open Popup
    const width = 800;
    const height = 600;
    const left = (window.screen.width - width) / 2;
    const top = (window.screen.height - height) / 2;
    window.open(
      pickerUri,
      'GooglePicker',
      `width=${width},height=${height},top=${top},left=${left}`
    );

    // 3. Poll for completion
    pollPicker(id);
  } catch (e) {
    console.error(e);
    importMessage.value = 'Failed to start picker flow';
    loading.value = false;
  }
};

const pollPicker = (sessionId: string) => {
  if (pickerTimer.value) clearInterval(pickerTimer.value);

  pickerTimer.value = window.setInterval(async () => {
    try {
      const res = await fetch(
        `${import.meta.env.VITE_API_BASE || ''}/api/google/picker/poll/${sessionId}`
      );
      if (res.ok) {
        const { complete } = await res.json();
        if (complete) {
          // Stop polling
          if (pickerTimer.value) clearInterval(pickerTimer.value);

          // Trigger Sync
          await processPicker(sessionId);
        }
      }
    } catch (e) {
      console.error('Polling error', e);
    }
  }, 2000);
};

const importMessage = ref('');

const processPicker = async (sessionId: string) => {
  try {
    const res = await fetch(
      `${import.meta.env.VITE_API_BASE || ''}/api/google/picker/process/${sessionId}`,
      {
        method: 'POST',
      }
    );

    if (res.status === 202) {
      // Poll Progress
      const progressInterval = setInterval(async () => {
        try {
          const pRes = await fetch(
            `${import.meta.env.VITE_API_BASE || ''}/api/google/picker/progress/${sessionId}`
          );
          if (pRes.ok) {
            const pData = await pRes.json();
            // Refresh gallery periodically to show progress
            fetchPhotos();

            if (pData.status === 'done') {
              clearInterval(progressInterval);
              importMessage.value = `Successfully added ${pData.processed} photos!`;
              setTimeout(() => (importMessage.value = ''), 5000);
              loading.value = false;
            } else if (pData.status === 'error') {
              clearInterval(progressInterval);
              importMessage.value = `Error: ${pData.error}`;
              loading.value = false;
            }
          }
        } catch (e) {
          console.error('Progress poll error', e);
        }
      }, 2000);
    } else if (res.ok) {
      const { count } = await res.json();
      importMessage.value = `Successfully added ${count} photos!`;
      setTimeout(() => (importMessage.value = ''), 5000);
      fetchPhotos(); // Refresh list
      loading.value = false;
    } else {
      importMessage.value = 'Failed to process photos';
      loading.value = false;
    }
  } catch (e) {
    console.error('Process error', e);
    importMessage.value = 'Error processing photos';
    loading.value = false;
  }
};

onMounted(async () => {
  await store.fetchSettings();
  fetchPhotos();
});
</script>
