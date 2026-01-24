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

          <!-- Image Endpoint URL -->
          <div class="mb-5">
            <label class="block mb-2 text-gray-700 font-medium"
              >Image Endpoint URL (for firmware config)</label
            >
            <div class="flex gap-2">
              <input
                :value="getImageUrl('google_photos')"
                readonly
                class="flex-1 px-3 py-2 border-2 border-gray-300 rounded-lg bg-gray-50 text-gray-700 font-mono text-sm"
              />
              <button
                @click="copyToClipboard(getImageUrl('google_photos'))"
                class="px-4 py-2 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 transition"
              >
                Copy
              </button>
            </div>
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
            <div v-if="synologyStore.count !== null" class="text-sm mt-1">
              {{ synologyStore.count }} photo{{
                synologyStore.count !== 1 ? 's' : ''
              }}
              synced
            </div>
          </div>

          <!-- Image Endpoint URL -->
          <div class="mb-5">
            <label class="block mb-2 text-gray-700 font-medium"
              >Image Endpoint URL (for firmware config)</label
            >
            <div class="flex gap-2">
              <input
                :value="getImageUrl('synology')"
                readonly
                class="flex-1 px-3 py-2 border-2 border-gray-300 rounded-lg bg-gray-50 text-gray-700 font-mono text-sm"
              />
              <button
                @click="copyToClipboard(getImageUrl('synology'))"
                class="px-4 py-2 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 transition"
              >
                Copy
              </button>
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

          <!-- Image Endpoint URL -->
          <div class="mb-5">
            <label class="block mb-2 text-gray-700 font-medium"
              >Image Endpoint URL (for firmware config)</label
            >
            <div class="flex gap-2">
              <input
                :value="getImageUrl('telegram')"
                readonly
                class="flex-1 px-3 py-2 border-2 border-gray-300 rounded-lg bg-gray-50 text-gray-700 font-mono text-sm"
              />
              <button
                @click="copyToClipboard(getImageUrl('telegram'))"
                class="px-4 py-2 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 transition"
              >
                Copy
              </button>
            </div>
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
            </div>

            <div class="border-t border-gray-200 pt-4 mt-4">
              <h3 class="text-lg font-medium text-gray-700 mb-3">
                Push to Device
              </h3>
              <p class="text-sm text-gray-600 mb-4">
                Enable to push generic images directly to the device display
                when sent to the bot.
              </p>

              <label class="flex items-center cursor-pointer mb-4">
                <input
                  type="checkbox"
                  v-model="form.telegram_push_enabled"
                  class="mr-3 w-5 h-5 text-primary-600 rounded focus:ring-primary-500"
                />
                <span class="text-gray-700">Enable Push to Device</span>
              </label>

              <div v-if="form.telegram_push_enabled">
                <label class="block mb-2 text-gray-700 font-medium"
                  >Device Host (IP or Hostname)</label
                >
                <input
                  type="text"
                  v-model.trim="form.device_host"
                  placeholder="photoframe.local"
                  class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
                />
                <p class="text-xs text-gray-500 mt-1">
                  The IP address or hostname of your ESP32 device on the local
                  network.
                </p>
              </div>
            </div>

            <button
              @click="save"
              class="mt-3 px-5 py-2.5 bg-gray-600 text-white font-semibold rounded-lg hover:bg-gray-700 transition"
            >
              Update Settings
            </button>
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

    <!-- Security & Access Card -->
    <section class="bg-white rounded-xl p-6 shadow-xl">
      <h2
        class="text-2xl font-semibold text-gray-800 mb-5 pb-3 border-b-2 border-primary-500"
      >
        Security & Access
      </h2>

      <!-- Change Password Section -->
      <div class="mb-8">
        <div class="flex justify-between items-center mb-4">
          <h3 class="text-lg font-medium text-gray-800">Change Password</h3>
          <button
            @click="showPasswordForm = !showPasswordForm"
            class="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 font-medium rounded-lg transition"
          >
            {{ showPasswordForm ? 'Cancel' : 'Change Password' }}
          </button>
        </div>
        <div
          v-if="showPasswordForm"
          class="bg-gray-50 p-4 rounded-lg border border-gray-200"
        >
          <div class="space-y-4 max-w-md">
            <div>
              <label class="block mb-1 text-sm text-gray-600"
                >Current Password</label
              >
              <input
                v-model="passwordForm.oldPassword"
                type="password"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <div>
              <label class="block mb-1 text-sm text-gray-600"
                >New Password</label
              >
              <input
                v-model="passwordForm.newPassword"
                type="password"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <div>
              <label class="block mb-1 text-sm text-gray-600"
                >Confirm New Password</label
              >
              <input
                v-model="passwordForm.confirmPassword"
                type="password"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <button
              @click="changePassword"
              class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 transition"
            >
              Update Password
            </button>
          </div>
        </div>
      </div>

      <!-- Access Tokens Section -->
      <div>
        <h3 class="text-lg font-medium text-gray-800 mb-4">
          Device Access Tokens
        </h3>

        <!-- Generated Token Alert -->
        <div
          v-if="generatedToken"
          class="bg-green-100 border-l-4 border-green-500 p-4 mb-5"
        >
          <div class="flex justify-between items-start">
            <div>
              <h4 class="font-bold text-green-900 mb-1">Token Generated!</h4>
              <p class="text-green-800 text-sm mb-2">
                Copy this token securely. It will not be shown again.
              </p>
            </div>
            <button
              @click="generatedToken = ''"
              class="text-green-800 hover:text-green-900 text-xl"
            >
              ×
          </button>
          </div>
          <div class="bg-white p-2 rounded border border-green-200 relative">
            <code class="break-all text-sm font-mono text-gray-800 block pr-20">{{
              generatedToken
            }}</code>
            <button
              @click="copyToken"
              class="absolute right-2 top-1/2 -translate-y-1/2 px-3 py-1.5 bg-primary-600 hover:bg-primary-700 text-white text-xs font-medium rounded transition-colors"
            >
              {{ tokenCopied ? '✓ Copied!' : 'Copy' }}
            </button>
          </div>
        </div>

        <!-- Generate Form -->
        <div class="bg-gray-50 p-4 rounded-lg mb-6 border border-gray-200">
          <h4 class="text-md font-medium text-gray-800 mb-3">
            Generate New Token
          </h4>
          <div class="flex gap-3 items-end max-w-2xl">
            <div class="flex-1">
              <label class="block mb-1 text-sm text-gray-600"
                >Token Name (e.g. Living Room Frame)</label
              >
              <input
                v-model="newTokenName"
                type="text"
                placeholder="Device Name"
                class="w-full px-3 py-2 border-2 border-gray-300 rounded-lg focus:border-primary-500 focus:ring-2 focus:ring-primary-200 transition"
              />
            </div>
            <button
              @click="generateToken"
              class="px-5 py-2.5 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 transition"
            >
              Generate
            </button>
          </div>
        </div>

        <!-- Tokens Table -->
        <h4 class="text-md font-medium text-gray-800 mb-3">Active Tokens</h4>
        <div class="overflow-x-auto border border-gray-200 rounded-lg">
          <table class="w-full text-left border-collapse">
            <thead>
              <tr class="bg-gray-50 border-b border-gray-200">
                <th class="p-3 text-sm font-semibold text-gray-600">Name</th>
                <th class="p-3 text-sm font-semibold text-gray-600">
                  Created At
                </th>
                <th class="p-3 text-sm font-semibold text-gray-600 text-right">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="token in authStore.tokens"
                :key="token.id"
                class="border-b border-gray-100 hover:bg-gray-50"
              >
                <td class="p-3 font-medium text-gray-800">{{ token.name }}</td>
                <td class="p-3 text-sm text-gray-500">
                  {{ new Date(token.created_at).toLocaleString() }}
                </td>
                <td class="p-3 text-right">
                  <button
                    @click="revokeToken(token.id)"
                    class="text-red-600 hover:text-red-800 hover:underline text-sm font-medium"
                  >
                    Revoke
                  </button>
                </td>
              </tr>
              <tr v-if="authStore.tokens.length === 0">
                <td colspan="3" class="p-10 text-center text-gray-500">
                  No active tokens found. Create one above to connect a device.
                </td>
              </tr>
            </tbody>
          </table>
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
import { useSynologyStore } from '../stores/synology';
import { useAuthStore } from '../stores/auth';
import { api } from '../api';
import Gallery from './Gallery.vue';

const store = useSettingsStore();
const synologyStore = useSynologyStore();
const authStore = useAuthStore();
const activeTab = ref('google');

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
  telegram_push_enabled: false,
  device_host: '',
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
    telegram_push_enabled: store.settings.telegram_push_enabled === 'true',
    device_host: store.settings.device_host || '',
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
    await synologyStore.fetchCount();
  }

  await authStore.fetchTokens();
});

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
    telegram_push_enabled: String(form.telegram_push_enabled),
    device_host: form.device_host,
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
    await api.post('/auth/google/logout');
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
    await synologyStore.testConnection(form.synology_otp_code);
    showToast('Connection Successful!');
    form.synology_otp_code = '';
    // Store updates settings internally, but we need to update form
    form.synology_sid = store.settings.synology_sid;
  } catch (e: any) {
    const err = e.response?.data?.error || 'Unknown error';
    if (err.includes('code: 403')) {
      showToast(
        '2FA Required! Please enter OTP code and Test Connection again.',
        5000
      );
    } else {
      showToast('Connection Failed: ' + err);
    }
  }
};

const logoutSynology = async () => {
  if (!confirm('Are you sure you want to disconnect Synology?')) return;
  try {
    await synologyStore.logout();
    form.synology_sid = '';
    showToast('Logged out from Synology.');
  } catch (e) {
    showToast('Error logging out: ' + e);
  }
};

const loadAlbums = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.fetchAlbums();
    form.albums = synologyStore.albums;
    showToast('Albums loaded!');
  } catch (e: any) {
    if (
      e.message === 'Session expired' ||
      (e.response && e.response.status === 401)
    ) {
      showToast(
        'Session expired or Unauthorized. Please check login/settings.',
        5000
      );
    } else {
      showToast(
        'Failed to load albums: ' + (e.response?.data?.error || e.message)
      );
    }
  }
};

const syncSynology = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.sync();
    showToast('Sync started/completed successfully!');
  } catch (e: any) {
    if (e.response && e.response.status === 401) {
      showToast('Session expired. Please reconnect.', 5000);
    } else {
      showToast('Sync Failed: ' + (e.response?.data?.error || 'Unknown error'));
    }
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
    await api.post('/synology/clear');
    showToast('All Synology photos cleared from database.');
    await synologyStore.fetchCount();
  } catch (e: any) {
    showToast('Clear Failed: ' + (e.response?.data?.error || e.message));
  }
};

// Token Management
const generatedToken = ref('');
const newTokenName = ref('');
const tokenCopied = ref(false);

const copyToken = async () => {
  try {
    await navigator.clipboard.writeText(generatedToken.value);
    tokenCopied.value = true;
    setTimeout(() => {
      tokenCopied.value = false;
    }, 2000);
  } catch (e) {
    showToast('Failed to copy token');
  }
};


// Password Change
const showPasswordForm = ref(false);
const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
});

const generateToken = async () => {
  if (!newTokenName.value) {
    showToast('Please enter a name for the token.');
    return;
  }
  try {
    const token = await authStore.generateToken(newTokenName.value);
    generatedToken.value = token;
    newTokenName.value = '';
    showToast('Token generated!');
  } catch (e: any) {
    showToast(
      'Failed to generate token: ' + (e.response?.data?.error || e.message)
    );
  }
};

const revokeToken = async (id: number) => {
  if (!confirm('Revoke this token? Device will lose access.')) return;
  try {
    await authStore.revokeToken(id);
    showToast('Token revoked.');
  } catch (e: any) {
    showToast('Failed: ' + e.message);
  }
};

const changePassword = async () => {
  if (!passwordForm.oldPassword || !passwordForm.newPassword) {
    showToast('Please fill in all password fields.');
    return;
  }
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    showToast('New passwords do not match.');
    return;
  }
  if (passwordForm.newPassword.length < 6) {
    showToast('New password must be at least 6 characters.');
    return;
  }
  try {
    await api.post('/auth/password', {
      old_password: passwordForm.oldPassword,
      new_password: passwordForm.newPassword,
    });
    passwordForm.oldPassword = '';
    passwordForm.newPassword = '';
    passwordForm.confirmPassword = '';
    showToast('Password updated successfully!');
  } catch (e: any) {
    showToast('Failed: ' + (e.response?.data?.error || e.message));
  }
};

// Get image endpoint URL
const getImageUrl = (source: string) => {
  const host = window.location.host;
  const protocol = window.location.protocol;
  return `${protocol}//${host}/image/${source}`;
};

// Copy to clipboard
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    showToast('URL copied to clipboard!');
  } catch (e) {
    showToast('Failed to copy to clipboard');
  }
};
</script>
