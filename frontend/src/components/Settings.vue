<template>
  <div class="space-y-5">
    <!-- General Settings Card -->
    <section class="bg-white rounded-xl p-6 shadow-xl">
      <h2
        class="text-2xl font-semibold text-gray-800 mb-5 pb-3 border-b-2 border-primary-500"
      >
        General Device Settings
      </h2>

      <div v-if="store.loading" class="text-center text-gray-600 py-10">
        Loading...
      </div>
      <div
        v-else-if="store.error"
        class="bg-red-100 text-red-700 p-3 rounded-lg"
      >
        {{ store.error }}
      </div>

      <form @submit.prevent="save" class="space-y-5">
        <h3 class="text-lg font-medium text-gray-700 mt-4">
          Device Configuration
        </h3>

        <!-- Display Orientation -->
        <div>
          <label class="block mb-2 text-gray-700 font-medium"
            >Display Orientation:</label
          >
          <div class="flex gap-4">
            <label class="flex items-center cursor-pointer">
              <input
                type="radio"
                value="landscape"
                v-model="form.Orientation"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span>Landscape</span>
            </label>
            <label class="flex items-center cursor-pointer">
              <input
                type="radio"
                value="portrait"
                v-model="form.Orientation"
                class="mr-2 text-primary-600 focus:ring-primary-500"
              />
              <span>Portrait</span>
            </label>
          </div>
        </div>

        <!-- Dimensions -->
        <div>
          <label class="block mb-2 text-gray-700 font-medium"
            >Dimensions:</label
          >
          <div class="flex gap-3">
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-600">Width</span>
              <input
                type="number"
                v-model="form.DisplayWidth"
                class="w-24 px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-600">Height</span>
              <input
                type="number"
                v-model="form.DisplayHeight"
                class="w-24 px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
          </div>
        </div>

        <!-- Checkboxes -->
        <div class="space-y-3">
          <label class="flex items-center cursor-pointer">
            <input
              type="checkbox"
              v-model="form.CollageMode"
              class="mr-3 w-5 h-5 text-primary-600 rounded focus:ring-primary-500"
            />
            <span class="text-gray-700"
              >Enable Collage Mode (Combine 2 photos)</span
            >
          </label>
          <label class="flex items-center cursor-pointer">
            <input
              type="checkbox"
              v-model="form.show_date"
              class="mr-3 w-5 h-5 text-primary-600 rounded focus:ring-primary-500"
            />
            <span class="text-gray-700">Show Date</span>
          </label>
          <label class="flex items-center cursor-pointer">
            <input
              type="checkbox"
              v-model="form.show_weather"
              class="mr-3 w-5 h-5 text-primary-600 rounded focus:ring-primary-500"
            />
            <span class="text-gray-700">Show Weather</span>
          </label>
        </div>

        <!-- Weather Overlay -->
        <h3 class="text-lg font-medium text-gray-700 mt-6">Weather Overlay</h3>
        <div>
          <label class="block mb-2 text-gray-700 font-medium">Location:</label>
          <div class="flex gap-3">
            <div class="flex-1">
              <span class="block text-sm text-gray-600 mb-1">Latitude</span>
              <input
                type="text"
                v-model="form.weather_lat"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <div class="flex-1">
              <span class="block text-sm text-gray-600 mb-1">Longitude</span>
              <input
                type="text"
                v-model="form.weather_lon"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
          </div>
        </div>

        <!-- Save Button -->
        <div class="pt-4">
          <button
            type="submit"
            :disabled="store.loading"
            class="px-6 py-3 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {{ store.loading ? 'Saving...' : 'Save General Settings' }}
          </button>
        </div>
      </form>
    </section>

    <!-- Data Sources Card -->
    <section class="bg-white rounded-xl p-6 shadow-xl">
      <h2
        class="text-2xl font-semibold text-gray-800 mb-5 pb-3 border-b-2 border-primary-500"
      >
        Data Sources
      </h2>

      <!-- Tabs Navigation -->
      <div class="flex border-b-2 border-gray-200 mb-5">
        <button
          type="button"
          @click="activeTab = 'google'"
          :class="
            activeTab === 'google'
              ? 'border-primary-600 text-primary-600 font-bold'
              : 'border-transparent text-gray-600 hover:bg-gray-50'
          "
          class="px-5 py-3 border-b-2 transition-colors"
        >
          Google Photos
        </button>
        <button
          type="button"
          @click="activeTab = 'synology'"
          :class="
            activeTab === 'synology'
              ? 'border-primary-600 text-primary-600 font-bold'
              : 'border-transparent text-gray-600 hover:bg-gray-50'
          "
          class="px-5 py-3 border-b-2 transition-colors"
        >
          Synology
        </button>
        <button
          type="button"
          @click="activeTab = 'telegram'"
          :class="
            activeTab === 'telegram'
              ? 'border-primary-600 text-primary-600 font-bold'
              : 'border-transparent text-gray-600 hover:bg-gray-50'
          "
          class="px-5 py-3 border-b-2 transition-colors"
        >
          Telegram
        </button>
      </div>

      <!-- Google Photos Tab -->
      <div v-show="activeTab === 'google'" class="py-3">
        <div v-if="form.google_connected === 'true'">
          <div class="bg-green-100 text-green-700 p-3 rounded-lg mb-5">
            ✅ Connected to Google Photos
          </div>

          <!-- Embed Gallery Component -->
          <div class="mt-5">
            <Gallery />
          </div>

          <div class="mt-5">
            <button
              @click="logoutGoogle"
              class="px-5 py-2.5 bg-red-600 text-white font-semibold rounded-lg hover:bg-red-700 transition"
            >
              Disconnect Google Photos
            </button>
          </div>
        </div>
        <div v-else>
          <div class="bg-blue-50 border-l-4 border-blue-500 p-4 mb-5">
            <div class="font-semibold text-blue-900 mb-2">
              ℹ️ Setup Required
            </div>
            <p class="text-sm text-blue-800">
              To enable Google Photos, create a project in
              <a
                href="https://console.cloud.google.com/"
                target="_blank"
                class="underline hover:text-blue-600"
                >Google Cloud Console</a
              >.
              <br />
              Redirect URI:
              <code class="bg-blue-100 px-2 py-0.5 rounded text-xs"
                >http://[YOUR_SERVER_IP]:8080/api/auth/google/callback</code
              >
            </p>
          </div>

          <div class="space-y-4">
            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Client ID</label
              >
              <input
                type="text"
                v-model.trim="form.google_client_id"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Client Secret</label
              >
              <input
                type="password"
                v-model.trim="form.google_client_secret"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div class="flex gap-3">
              <button
                @click="save"
                class="px-5 py-2.5 bg-gray-600 text-white font-semibold rounded-lg hover:bg-gray-700 transition"
              >
                Save Credentials
              </button>
              <button
                v-if="form.google_client_id && form.google_client_secret"
                @click.prevent="connectGoogle"
                type="button"
                class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all"
              >
                Authorize with Google
              </button>
              <span v-else class="text-sm text-gray-500 self-center"
                >Save credentials first, then authorize.</span
              >
            </div>
          </div>
        </div>
      </div>

      <!-- Synology Tab -->
      <div v-show="activeTab === 'synology'" class="py-3">
        <div v-if="form.synology_sid">
          <div class="bg-green-100 text-green-700 p-3 rounded-lg mb-5">
            ✅ Connected to Synology Photos ({{ form.synology_account }} @
            {{ form.synology_url }})
            <div v-if="synologyPhotoCount !== null" class="text-sm mt-1">
              {{ synologyPhotoCount }} photo{{
                synologyPhotoCount !== 1 ? 's' : ''
              }}
              synced
            </div>
          </div>

          <div class="mb-4">
            <label class="block mb-2 text-gray-700 font-medium"
              >Sync Album (Optional)</label
            >
            <div class="flex gap-3">
              <select
                v-model="form.synology_album_id"
                class="flex-1 px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              >
                <option value="">All Photos</option>
                <option
                  v-for="album in form.albums"
                  :key="album.id"
                  :value="String(album.id)"
                >
                  {{ album.name }}
                </option>
              </select>
              <button
                type="button"
                @click.prevent="loadAlbums"
                class="px-5 py-2.5 bg-gray-600 text-white font-semibold rounded-lg hover:bg-gray-700 transition"
              >
                Refresh Albums
              </button>
            </div>
            <p class="text-sm text-gray-500 mt-1">
              Select an album to limit sync. Leave as "All Photos" to sync
              everything.
            </p>
          </div>

          <div class="mb-4">
            <label class="block mb-2 text-gray-700 font-medium"
              >Photo Space</label
            >
            <select
              v-model="form.synology_space"
              class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
            >
              <option value="personal">Personal Space</option>
              <option value="shared">Shared Space</option>
            </select>
            <p class="text-sm text-gray-500 mt-1">
              Select whether to sync from your personal space or shared space.
            </p>
          </div>

          <div class="mb-4">
            <button
              @click="save"
              class="px-5 py-2.5 bg-gray-600 text-white font-semibold rounded-lg hover:bg-gray-700 transition"
            >
              Save Album Selection
            </button>
          </div>

          <div class="flex gap-3">
            <button
              @click.prevent="syncSynology"
              type="button"
              class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all"
            >
              Sync Now
            </button>
            <button
              @click.prevent="clearSynology"
              type="button"
              class="px-5 py-2.5 bg-amber-600 text-white font-semibold rounded-lg hover:bg-amber-700 transition"
            >
              Clear All Photos
            </button>
            <button
              @click="logoutSynology"
              class="px-5 py-2.5 bg-red-600 text-white font-semibold rounded-lg hover:bg-red-700 transition"
            >
              Log Out
            </button>
          </div>
        </div>
        <div v-else>
          <div class="space-y-4">
            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >NAS URL (e.g. https://192.168.1.10:5001)</label
              >
              <input
                type="text"
                v-model.trim="form.synology_url"
                placeholder="https://..."
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Account</label
              >
              <input
                type="text"
                v-model.trim="form.synology_account"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Password</label
              >
              <input
                type="password"
                v-model.trim="form.synology_password"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div>
              <label class="flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  v-model="form.synology_skip_cert"
                  class="mr-3 w-5 h-5 text-primary-600 rounded focus:ring-primary-500"
                />
                <span class="text-gray-700"
                  >Skip Certificate Verification (Insecure)</span
                >
              </label>
            </div>

            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >OTP Code (If 2FA enabled)</label
              >
              <input
                type="text"
                v-model.trim="form.synology_otp_code"
                placeholder="6-digit code"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>

            <div>
              <button
                @click.prevent="testSynology"
                type="button"
                :disabled="
                  !form.synology_url ||
                  !form.synology_account ||
                  !form.synology_password
                "
                class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Test Connection & Login
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Telegram Tab -->
      <div v-show="activeTab === 'telegram'" class="py-3">
        <div v-if="form.telegram_bot_token">
          <div class="bg-green-100 text-green-700 p-3 rounded-lg mb-5">
            ✅ Telegram Bot Configured
          </div>
          <div class="space-y-4">
            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Telegram Bot Token</label
              >
              <input
                type="text"
                v-model.trim="form.telegram_bot_token"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
              <button
                @click="save"
                class="mt-3 px-5 py-2.5 bg-gray-600 text-white font-semibold rounded-lg hover:bg-gray-700 transition"
              >
                Update Token
              </button>
            </div>
          </div>
        </div>
        <div v-else>
          <div class="space-y-4">
            <div>
              <label class="block mb-2 text-gray-700 font-medium"
                >Telegram Bot Token</label
              >
              <input
                type="text"
                v-model.trim="form.telegram_bot_token"
                placeholder="Enter Bot Token"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
              <p class="text-sm text-gray-500 mt-1">
                Send photos to your bot to display them. Only the last photo
                will be shown.
              </p>
              <button
                @click="save"
                class="mt-3 px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all"
              >
                Save Token
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Global Save Message -->
    <div
      v-if="saveMessage"
      :class="
        saveMessage.includes('Error')
          ? 'bg-red-100 text-red-700'
          : 'bg-green-100 text-green-700'
      "
      class="fixed bottom-5 right-5 z-50 px-6 py-3 rounded-lg shadow-lg"
    >
      {{ saveMessage }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useSettingsStore } from '../stores/settings';
import Gallery from './Gallery.vue';

const store = useSettingsStore();
const activeTab = ref('google');
const synologyPhotoCount = ref<number | null>(null);

const form = reactive({
  Orientation: 'landscape',
  DisplayWidth: '800',
  DisplayHeight: '480',
  CollageMode: false,
  show_date: true,
  show_weather: true,
  google_client_id: '',
  google_client_secret: '',
  google_connected: 'false',
  telegram_bot_token: '',
  weather_lat: '',
  weather_lon: '',
  synology_url: '',
  synology_account: '',
  synology_password: '',
  synology_skip_cert: false,
  synology_space: 'personal',
  synology_album_id: '',
  synology_otp_code: '',
  synology_sid: '',
  albums: [] as any[],
});

onMounted(async () => {
  await store.fetchSettings();
  Object.assign(form, {
    Orientation: store.settings.orientation || 'landscape',
    DisplayWidth: store.settings.display_width || '800',
    DisplayHeight: store.settings.display_height || '480',
    CollageMode: store.settings.collage_mode === 'true',
    show_date: store.settings.show_date !== 'false',
    show_weather: store.settings.show_weather !== 'false',
    google_client_id: store.settings.google_client_id || '',
    google_client_secret: store.settings.google_client_secret || '',
    google_connected: store.settings.google_connected || 'false',
    telegram_bot_token: store.settings.telegram_bot_token || '',
    weather_lat: store.settings.weather_lat || '',
    weather_lon: store.settings.weather_lon || '',
    synology_url: store.settings.synology_url || '',
    synology_account: store.settings.synology_account || '',
    synology_password: store.settings.synology_password || '',
    synology_skip_cert: store.settings.synology_skip_cert === 'true',
    synology_space: store.settings.synology_space || 'personal',
    synology_album_id: store.settings.synology_album_id || '',
    synology_sid: store.settings.synology_sid || '',
  });

  // Load cached albums if available
  if (store.settings.synology_albums_cache) {
    try {
      form.albums = JSON.parse(store.settings.synology_albums_cache);
    } catch (e) {
      console.error('Failed to parse cached albums', e);
    }
  }

  // Fetch Synology photo count if connected
  if (form.synology_sid) {
    await fetchSynologyCount();
  }
});

// Fetch Synology photo count
const fetchSynologyCount = async () => {
  try {
    const res = await fetch('/api/synology/count');
    if (res.ok) {
      const data = await res.json();
      synologyPhotoCount.value = data.count || 0;
    }
  } catch (e) {
    console.error('Failed to fetch Synology photo count', e);
  }
};

const saveSettingsInternal = async () => {
  await store.saveSettings({
    orientation: form.Orientation,
    display_width: String(form.DisplayWidth),
    display_height: String(form.DisplayHeight),
    collage_mode: String(form.CollageMode),
    show_date: String(form.show_date),
    show_weather: String(form.show_weather),
    google_client_id: form.google_client_id,
    google_client_secret: form.google_client_secret,
    telegram_bot_token: form.telegram_bot_token,
    weather_lat: form.weather_lat,
    weather_lon: form.weather_lon,
    synology_url: form.synology_url,
    synology_account: form.synology_account,
    synology_password: form.synology_password,
    synology_skip_cert: String(form.synology_skip_cert),
    synology_space: form.synology_space,
    synology_album_id: form.synology_album_id,
  });
};

const saveMessage = ref('');

// Helper to show toast notifications
const showToast = (message: string, duration = 3000) => {
  saveMessage.value = message;
  setTimeout(() => {
    saveMessage.value = '';
  }, duration);
};

const save = async () => {
  try {
    await saveSettingsInternal();
    showToast('Settings saved successfully!');
  } catch (e) {
    showToast('Error saving settings.');
  }
};

const connectGoogle = async () => {
  try {
    await saveSettingsInternal();
    window.location.href = '/api/auth/google/login';
  } catch (e) {
    showToast('Failed to save settings before connecting: ' + e);
  }
};

const logoutGoogle = async () => {
  if (!confirm('Are you sure you want to disconnect Google Photos?')) return;
  try {
    await fetch('/api/auth/google/logout', { method: 'POST' });
    form.google_connected = 'false';
    showToast('Disconnected Google Photos.');
    await store.fetchSettings();
  } catch (e) {
    showToast('Error disconnecting: ' + e);
  }
};

const testSynology = async () => {
  await saveSettingsInternal();
  try {
    const res = await fetch('/api/synology/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ otp_code: form.synology_otp_code }),
    });
    if (res.ok) {
      showToast('Connection Successful!');
      form.synology_otp_code = '';
      await store.fetchSettings();
      form.synology_sid = store.settings.synology_sid;
    } else {
      const data = await res.json();
      const err = data.error || 'Unknown error';
      if (err.includes('code: 403')) {
        showToast(
          '2FA Required! Please enter OTP code and Test Connection again.',
          5000
        );
      } else {
        showToast('Connection Failed: ' + err);
      }
    }
  } catch (e) {
    showToast('Connection Error: ' + e);
  }
};

const logoutSynology = async () => {
  if (!confirm('Are you sure you want to disconnect Synology?')) return;
  try {
    await fetch('/api/synology/logout', { method: 'POST' });
    form.synology_sid = '';
    showToast('Logged out from Synology.');
  } catch (e) {
    showToast('Error logging out: ' + e);
  }
};

const loadAlbums = async () => {
  await saveSettingsInternal();
  try {
    const res = await fetch('/api/synology/albums');
    if (res.ok) {
      form.albums = await res.json();
      showToast('Albums loaded!');
    } else if (res.status === 401) {
      // Session expired - auto logout
      form.synology_sid = '';
      await store.fetchSettings();
      showToast('Session expired. Please reconnect to Synology.', 5000);
    } else {
      showToast('Failed to load albums.');
    }
  } catch (e) {
    showToast('Error loading albums: ' + e);
  }
};

const syncSynology = async () => {
  await saveSettingsInternal();
  try {
    const res = await fetch('/api/synology/sync', { method: 'POST' });
    if (res.ok) {
      showToast('Sync started/completed successfully!');
      await fetchSynologyCount(); // Update count after sync
    } else if (res.status === 401) {
      // Session expired - auto logout
      form.synology_sid = '';
      await store.fetchSettings();
      showToast('Session expired. Please reconnect to Synology.', 5000);
    } else {
      const data = await res.json();
      showToast('Sync Failed: ' + (data.error || 'Unknown error'));
    }
  } catch (e) {
    showToast('Sync Error: ' + e);
  }
};

const clearSynology = async () => {
  if (
    !confirm(
      'Are you sure you want to clear all Synology photo references? Local files will not be deleted.'
    )
  )
    return;

  try {
    const res = await fetch('/api/synology/clear', { method: 'POST' });
    if (res.ok) {
      showToast('All Synology photos cleared from database.');
      await fetchSynologyCount();
    } else {
      const data = await res.json();
      showToast('Clear Failed: ' + (data.error || 'Unknown error'));
    }
  } catch (e) {
    showToast('Clear Error: ' + e);
  }
};
</script>
