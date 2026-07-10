<template>
  <div class="pa-1 pa-sm-4">
    <!-- Sources Card (gallery on top, settings below) -->
    <v-card class="mb-6">
      <v-tabs v-model="sourceTab" color="primary" show-arrows>
        <v-tab value="gallery">Gallery</v-tab>
        <v-tab value="immich">Immich</v-tab>
        <v-tab value="synology_photos">Synology</v-tab>
        <v-tab value="google_photos">Google</v-tab>
        <v-tab value="unsplash">Unsplash</v-tab>
        <v-tab value="pexels">Pexels</v-tab>
        <v-tab value="url">URL Proxy</v-tab>
        <v-tab value="ai_generation">AI Generation</v-tab>
      </v-tabs>
      <v-card-text>
        <div v-if="sourceHasGallery">
          <Gallery />
          <v-divider class="my-6"></v-divider>
        </div>
        <v-window v-model="sourceTab" :touch="false">
          <!-- URL Proxy -->
          <v-window-item value="url">
            <v-card-text>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                Add external image URLs to be served by the photoframe. You can
                bind URLs to specific devices or leave them global.
              </v-alert>

              <div class="d-flex justify-end mb-4">
                <v-btn
                  color="primary"
                  prepend-icon="mdi-plus"
                  class="mb-4"
                  @click="openAddURLDialog"
                >
                  Add URL Source
                </v-btn>
              </div>

              <v-table density="comfortable" class="border rounded">
                <thead>
                  <tr>
                    <th>URL</th>
                    <th>Bound Devices</th>
                    <th class="text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="src in urlSources" :key="src.id">
                    <td class="text-truncate" style="max-width: 300px">
                      <a :href="src.url" target="_blank">{{ src.url }}</a>
                    </td>
                    <td>
                      <div v-if="src.device_ids && src.device_ids.length">
                        <v-chip
                          v-for="did in src.device_ids"
                          :key="did"
                          size="x-small"
                          class="mr-1"
                        >
                          {{ getDeviceName(did) }}
                        </v-chip>
                      </div>
                      <span v-else class="text-grey text-caption">Global</span>
                    </td>
                    <td class="text-right">
                      <v-btn
                        color="primary"
                        variant="text"
                        size="small"
                        icon="mdi-pencil"
                        class="mr-2"
                        @click="openEditURLDialog(src)"
                      ></v-btn>
                      <v-btn
                        color="error"
                        variant="text"
                        size="small"
                        icon="mdi-delete"
                        @click="deleteURLSourceWrapper(src.id)"
                      ></v-btn>
                    </td>
                  </tr>
                  <tr v-if="urlSources.length === 0">
                    <td colspan="4" class="text-center text-grey py-4">
                      No URL sources added.
                    </td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-window-item>

          <!-- Add/Edit URL Dialog -->
          <v-dialog v-model="showAddURLDialog" max-width="500px">
            <v-card>
              <v-card-title>{{
                isEditingURL ? 'Edit URL Source' : 'Add URL Source'
              }}</v-card-title>
              <v-card-text>
                <v-form @submit.prevent="saveURLSource">
                  <v-text-field
                    v-model="newURL.url"
                    label="Image URL"
                    placeholder="https://example.com/image.jpg"
                    variant="outlined"
                    class="mb-2"
                    :rules="[(v) => !!v || 'URL is required']"
                  ></v-text-field>

                  <v-select
                    v-model="newURL.device_ids"
                    :items="availableDevices"
                    item-title="name"
                    item-value="id"
                    label="Bind to Devices (Optional)"
                    placeholder="Leave empty for Global"
                    variant="outlined"
                    multiple
                    chips
                    class="mb-4"
                    hint="If selected, only these devices will see this image."
                    persistent-hint
                  ></v-select>
                </v-form>
              </v-card-text>
              <v-card-actions>
                <v-spacer></v-spacer>
                <v-btn
                  color="grey"
                  variant="text"
                  @click="showAddURLDialog = false"
                  >Cancel</v-btn
                >
                <v-btn color="primary" @click="saveURLSource">Save</v-btn>
              </v-card-actions>
            </v-card>
          </v-dialog>

          <!-- Google (Photos + Calendar) -->
          <v-window-item value="google_photos">
            <v-card-text>
              <!-- Shared Google API Credentials -->
              <h3 class="text-subtitle-1 font-weight-bold mb-3">
                Google API Credentials
              </h3>

              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                <div class="text-body-2">
                  These credentials are shared by Google Photos and Google
                  Calendar. Create a project in
                  <a href="https://console.cloud.google.com/" target="_blank"
                    >Google Cloud Console</a
                  >
                  and add the redirect URI:
                  <br />
                  <code
                    >http://[YOUR_SERVER_IP]:8080/api/auth/google/callback</code
                  >
                </div>
              </v-alert>

              <v-text-field
                v-model="form.google_client_id"
                label="Client ID"
                variant="outlined"
                class="mb-2"
              ></v-text-field>

              <v-text-field
                v-model="form.google_client_secret"
                label="Client Secret"
                type="password"
                variant="outlined"
                class="mb-4"
              ></v-text-field>

              <v-btn color="grey-darken-1" @click="save" class="mb-2"
                >Save Credentials</v-btn
              >

              <!-- Photos Section -->
              <v-divider class="my-6"></v-divider>
              <h3 class="text-subtitle-1 font-weight-bold mb-3">Photos</h3>

              <div v-if="form.google_connected === 'true'">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Connected to Google Photos
                </v-alert>

                <div class="d-flex flex-wrap ga-2 mt-4">
                  <v-btn
                    color="warning"
                    :loading="deletingAllPhotos"
                    @click="deleteAllPhotosForSource('google_photos')"
                    >Clear All Photos</v-btn
                  >
                  <v-btn color="error" variant="text" @click="logoutGoogle">
                    Disconnect Google Photos
                  </v-btn>
                </div>
              </div>

              <div v-else>
                <v-btn
                  v-if="form.google_client_id && form.google_client_secret"
                  color="primary"
                  @click="connectGoogle"
                >
                  Authorize Google Photos
                </v-btn>
                <v-alert
                  v-else
                  type="warning"
                  variant="tonal"
                  density="compact"
                >
                  Enter Google API credentials above first.
                </v-alert>
              </div>

              <!-- Calendar Section -->
              <v-divider class="my-6"></v-divider>
              <h3 class="text-subtitle-1 font-weight-bold mb-3">Calendar</h3>

              <div v-if="form.google_calendar_connected === 'true'">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Google Calendar connected
                </v-alert>

                <v-btn
                  color="error"
                  variant="text"
                  @click="logoutGoogleCalendar"
                >
                  Disconnect Google Calendar
                </v-btn>
              </div>

              <div v-else>
                <v-alert
                  type="info"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                >
                  Connect a Google account for Calendar integration. This can be
                  a different account than Google Photos.
                </v-alert>

                <v-btn
                  v-if="form.google_client_id && form.google_client_secret"
                  color="primary"
                  @click="connectGoogleCalendar"
                >
                  Authorize Google Calendar
                </v-btn>
                <v-alert
                  v-else
                  type="warning"
                  variant="tonal"
                  density="compact"
                >
                  Enter Google API credentials above first.
                </v-alert>
              </div>
            </v-card-text>
          </v-window-item>

          <!-- Synology -->
          <v-window-item value="synology_photos">
            <v-card-text>
              <div v-if="form.synology_sid">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Connected to Synology Photos ({{ form.synology_account }} @
                  {{ form.synology_url }})
                </v-alert>

                <v-row class="mt-2">
                  <v-col cols="12">
                    <div class="d-flex align-center justify-space-between mb-1">
                      <h3 class="text-subtitle-1 font-weight-bold">
                        Albums to sync
                      </h3>
                      <div class="d-flex ga-2">
                        <v-btn
                          size="small"
                          variant="text"
                          prepend-icon="mdi-refresh"
                          :loading="synologyStore.loading"
                          @click="loadAlbums"
                          >Refresh albums</v-btn
                        >
                      </div>
                    </div>
                    <div class="text-caption text-grey mb-2">
                      Choose which Synology Photos albums to pull photos from.
                      Saving replaces the entire sync set.
                    </div>

                    <AlbumPicker
                      v-model="synologySyncAlbumIds"
                      :albums="synologyStore.albums"
                      label-field="name"
                      stringify
                      empty-text='No albums found. Click "Refresh albums" to load them from Synology.'
                    ></AlbumPicker>
                  </v-col>
                </v-row>

                <v-row class="mt-1">
                  <v-col cols="12" md="6">
                    <v-checkbox
                      v-model="form.synology_auto_sync_enabled"
                      label="Auto Sync Album"
                      color="primary"
                      density="compact"
                      hide-details
                      @update:model-value="saveSettingsInternal()"
                    ></v-checkbox>
                  </v-col>
                  <v-col cols="12" md="6">
                    <v-select
                      v-model="form.synology_auto_sync_interval_minutes"
                      :items="autoSyncIntervalOptions"
                      item-title="title"
                      item-value="value"
                      label="Auto Sync Interval"
                      variant="outlined"
                      density="compact"
                      :disabled="!form.synology_auto_sync_enabled"
                      hint="How often to refresh photos from the selected album"
                      persistent-hint
                      @update:model-value="saveSettingsInternal()"
                    ></v-select>
                  </v-col>
                </v-row>

                <div class="d-flex flex-wrap ga-2 mt-4">
                  <v-btn
                    color="primary"
                    :loading="synologyStore.loading"
                    @click="syncSynology"
                    >Sync Now</v-btn
                  >
                  <v-btn color="warning" @click="clearSynology"
                    >Clear All Photos</v-btn
                  >
                  <v-btn color="error" variant="text" @click="logoutSynology"
                    >Disconnect</v-btn
                  >
                </div>
              </div>

              <div v-else>
                <v-text-field
                  v-model="form.synology_url"
                  label="NAS URL"
                  placeholder="https://192.168.1.10:5001"
                  variant="outlined"
                  class="mb-2"
                ></v-text-field>

                <v-text-field
                  v-model="form.synology_account"
                  label="Account"
                  variant="outlined"
                  class="mb-2"
                ></v-text-field>

                <v-text-field
                  v-model="form.synology_password"
                  label="Password"
                  type="password"
                  variant="outlined"
                  class="mb-2"
                ></v-text-field>

                <v-checkbox
                  v-model="form.synology_skip_cert"
                  label="Skip Certificate Verification (Insecure)"
                  color="primary"
                  density="compact"
                ></v-checkbox>

                <v-text-field
                  v-model="form.synology_otp_code"
                  label="OTP Code (If 2FA enabled)"
                  placeholder="6-digit code"
                  variant="outlined"
                  class="mb-4"
                ></v-text-field>

                <v-btn
                  color="primary"
                  :disabled="
                    !form.synology_url ||
                    !form.synology_account ||
                    !form.synology_password
                  "
                  :loading="synologyStore.loading"
                  @click="testSynology"
                >
                  Connect
                </v-btn>
              </div>
            </v-card-text>
          </v-window-item>

          <!-- Immich -->
          <v-window-item value="immich">
            <v-card-text>
              <div v-if="immichConnected">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Connected to Immich ({{ form.immich_url }})
                </v-alert>

                <v-row class="mt-2">
                  <v-col cols="12">
                    <div class="d-flex align-center justify-space-between mb-1">
                      <h3 class="text-subtitle-1 font-weight-bold">
                        Albums to sync
                      </h3>
                      <div class="d-flex ga-2">
                        <v-btn
                          size="small"
                          variant="text"
                          prepend-icon="mdi-refresh"
                          :loading="immichStore.loading"
                          @click="loadImmichAlbums"
                          >Refresh albums</v-btn
                        >
                      </div>
                    </div>
                    <div class="text-caption text-grey mb-2">
                      Choose which Immich albums and collections to pull photos
                      from. Saving replaces the entire sync set.
                    </div>

                    <AlbumPicker
                      v-model="syncAlbumIds"
                      :albums="immichStore.albums"
                      label-field="albumName"
                      empty-text='No albums found. Click "Refresh albums" to load them from Immich.'
                    >
                      <template #prepend>
                        <v-list-item>
                          <v-checkbox
                            v-model="syncFavorites"
                            label="Favorites"
                            color="primary"
                            density="compact"
                            hide-details
                          ></v-checkbox>
                        </v-list-item>
                        <v-list-item>
                          <v-checkbox
                            v-model="syncAll"
                            label="All Photos (entire library)"
                            color="primary"
                            density="compact"
                            hide-details
                          ></v-checkbox>
                        </v-list-item>
                        <v-list-item>
                          <v-checkbox
                            v-model="syncMemories"
                            label="Memories (on this day)"
                            color="primary"
                            density="compact"
                            hide-details
                          ></v-checkbox>
                        </v-list-item>
                      </template>
                    </AlbumPicker>
                  </v-col>
                </v-row>

                <v-row v-if="syncMemories" class="mt-0">
                  <v-col cols="12">
                    <v-select
                      v-model="form.immich_memory_mode"
                      :items="immichMemoryModeOptions"
                      item-title="title"
                      item-value="value"
                      label="Memory Years"
                      variant="outlined"
                      density="compact"
                      hint="Shuffle across all past years, or only the most recent year's photos"
                      persistent-hint
                      @update:model-value="saveSettingsInternal()"
                    ></v-select>
                  </v-col>
                </v-row>

                <v-row class="mt-1">
                  <v-col cols="12" md="6">
                    <v-checkbox
                      v-model="form.immich_auto_sync_enabled"
                      label="Auto Sync Album"
                      color="primary"
                      density="compact"
                      hide-details
                      @update:model-value="saveSettingsInternal()"
                    ></v-checkbox>
                  </v-col>
                  <v-col cols="12" md="6">
                    <v-select
                      v-model="form.immich_auto_sync_interval_minutes"
                      :items="autoSyncIntervalOptions"
                      item-title="title"
                      item-value="value"
                      label="Auto Sync Interval"
                      variant="outlined"
                      density="compact"
                      :disabled="!form.immich_auto_sync_enabled"
                      hint="How often to refresh photos from the selected album"
                      persistent-hint
                      @update:model-value="saveSettingsInternal()"
                    ></v-select>
                  </v-col>
                </v-row>

                <div class="d-flex flex-wrap ga-2 mt-4">
                  <v-btn
                    color="primary"
                    :loading="immichStore.loading"
                    @click="syncImmich"
                    >Sync Now</v-btn
                  >
                  <v-btn color="warning" @click="clearImmich"
                    >Clear All Photos</v-btn
                  >
                  <v-btn color="error" variant="text" @click="disconnectImmich"
                    >Disconnect</v-btn
                  >
                </div>
              </div>

              <div v-else>
                <v-text-field
                  v-model="form.immich_url"
                  label="Immich Server URL"
                  placeholder="http://192.168.1.10:2283"
                  variant="outlined"
                  class="mb-2"
                ></v-text-field>

                <v-text-field
                  v-model="form.immich_api_key"
                  label="API Key"
                  type="password"
                  variant="outlined"
                  class="mb-4"
                ></v-text-field>

                <v-btn
                  color="primary"
                  :disabled="!form.immich_url || !form.immich_api_key"
                  :loading="immichStore.loading"
                  @click="testImmich"
                >
                  Connect
                </v-btn>
              </div>
            </v-card-text>
          </v-window-item>

          <!-- Unsplash -->
          <v-window-item value="unsplash">
            <v-card-text>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                Unsplash provides free high-resolution photos. Enter your
                Unsplash API access key, then add search topics — each topic
                becomes a synced album.
              </v-alert>

              <div v-if="unsplashConnected">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Connected to Unsplash
                </v-alert>

                <TopicManager
                  title="Search topics"
                  :store="unsplashStore"
                  hint="Each topic becomes an album of matching photos."
                  :randomize-results="form.unsplash_randomize_results"
                  :auto-sync-enabled="form.unsplash_auto_sync_enabled"
                  :auto-sync-interval="form.unsplash_auto_sync_interval_minutes"
                  :auto-sync-options="autoSyncIntervalOptions"
                  @update:randomize-results="
                    form.unsplash_randomize_results = $event;
                    saveSettingsInternal();
                  "
                  @update:auto-sync-enabled="
                    form.unsplash_auto_sync_enabled = $event;
                    saveSettingsInternal();
                  "
                  @update:auto-sync-interval="
                    form.unsplash_auto_sync_interval_minutes = $event;
                    saveSettingsInternal();
                  "
                  @save="(topics) => saveTopics('unsplash', topics)"
                  @sync="syncTopicSource('unsplash')"
                  @clear="clearTopicSource('unsplash')"
                >
                  <template #actions>
                    <v-btn
                      color="error"
                      variant="text"
                      @click="disconnectUnsplash"
                      >Disconnect</v-btn
                    >
                  </template>
                </TopicManager>
              </div>

              <div v-else>
                <v-text-field
                  v-model="form.unsplash_api_key"
                  label="Unsplash Access Key"
                  hint="Your app's Access Key — the Secret Key is not needed"
                  persistent-hint
                  type="password"
                  variant="outlined"
                  density="compact"
                  class="mb-4"
                ></v-text-field>

                <v-btn
                  color="primary"
                  :disabled="!form.unsplash_api_key"
                  @click="connectUnsplash"
                >
                  Connect
                </v-btn>
              </div>
            </v-card-text>
          </v-window-item>

          <!-- Pexels -->
          <v-window-item value="pexels">
            <v-card-text>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                Pexels provides free stock photos. Enter your Pexels API key,
                then add search topics — each topic becomes a synced album.
              </v-alert>

              <div v-if="pexelsConnected">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Connected to Pexels
                </v-alert>

                <TopicManager
                  title="Search topics"
                  :store="pexelsStore"
                  hint="Each topic becomes an album of matching photos."
                  :randomize-results="form.pexels_randomize_results"
                  :auto-sync-enabled="form.pexels_auto_sync_enabled"
                  :auto-sync-interval="form.pexels_auto_sync_interval_minutes"
                  :auto-sync-options="autoSyncIntervalOptions"
                  @update:randomize-results="
                    form.pexels_randomize_results = $event;
                    saveSettingsInternal();
                  "
                  @update:auto-sync-enabled="
                    form.pexels_auto_sync_enabled = $event;
                    saveSettingsInternal();
                  "
                  @update:auto-sync-interval="
                    form.pexels_auto_sync_interval_minutes = $event;
                    saveSettingsInternal();
                  "
                  @save="(topics) => saveTopics('pexels', topics)"
                  @sync="syncTopicSource('pexels')"
                  @clear="clearTopicSource('pexels')"
                >
                  <template #actions>
                    <v-btn
                      color="error"
                      variant="text"
                      @click="disconnectPexels"
                      >Disconnect</v-btn
                    >
                  </template>
                </TopicManager>
              </div>

              <div v-else>
                <v-text-field
                  v-model="form.pexels_api_key"
                  label="Pexels API Key"
                  type="password"
                  variant="outlined"
                  density="compact"
                  class="mb-4"
                ></v-text-field>

                <v-btn
                  color="primary"
                  :disabled="!form.pexels_api_key"
                  @click="connectPexels"
                >
                  Connect
                </v-btn>
              </div>
            </v-card-text>
          </v-window-item>

          <!-- Gallery -->
          <v-window-item value="gallery">
            <v-card-text>
              <h3 class="text-subtitle-1 font-weight-bold mb-1">
                Telegram Bot (Upload)
              </h3>
              <div class="text-caption text-grey mb-3">
                Optional. Configure a Telegram bot to upload photos into the
                gallery from your phone.
              </div>

              <div v-if="form.telegram_bot_token">
                <v-alert
                  type="success"
                  variant="tonal"
                  class="mb-4"
                  density="compact"
                  icon="mdi-check-circle"
                >
                  Telegram Bot Configured
                </v-alert>

                <v-divider class="my-4"></v-divider>

                <h3 class="text-subtitle-1 font-weight-bold mb-2">
                  Push to Device
                </h3>
                <div class="text-caption text-grey mb-2">
                  When enabled, photos uploaded via the Telegram bot are also
                  pushed to the selected devices immediately. They are added to
                  the gallery either way.
                </div>

                <v-checkbox
                  v-model="form.telegram_push_enabled"
                  label="Enable Push to Device"
                  color="primary"
                  hide-details
                  density="compact"
                  @update:model-value="saveSettingsInternal()"
                ></v-checkbox>

                <v-expand-transition>
                  <div v-if="form.telegram_push_enabled" class="mt-2">
                    <v-select
                      v-model="form.telegram_target_device_id"
                      :items="availableDevices"
                      item-title="name"
                      item-value="id"
                      label="Target Devices"
                      variant="outlined"
                      density="compact"
                      hint="Select the devices to display photos on"
                      persistent-hint
                      multiple
                      chips
                      closable-chips
                      @update:model-value="saveSettingsInternal()"
                    ></v-select>
                  </div>
                </v-expand-transition>

                <div class="d-flex flex-wrap ga-2 mt-4">
                  <v-btn
                    color="warning"
                    :loading="deletingAllPhotos"
                    @click="deleteAllPhotosForSource('gallery')"
                    >Clear All Photos</v-btn
                  >
                  <v-btn
                    color="error"
                    variant="text"
                    @click="disconnectTelegram"
                    >Disconnect</v-btn
                  >
                </div>
              </div>

              <div v-else>
                <v-text-field
                  v-model="form.telegram_bot_token"
                  label="Telegram Bot Token"
                  placeholder="Enter Bot Token"
                  variant="outlined"
                  hint="Send photos to your bot to add them to the gallery."
                  persistent-hint
                ></v-text-field>

                <v-btn color="primary" class="mt-4" @click="save"
                  >Save Token</v-btn
                >
              </div>
            </v-card-text>
          </v-window-item>

          <!-- AI Generation -->
          <v-window-item value="ai_generation">
            <v-card-text>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                Generate images using AI (OpenAI or Google Gemini). Configure
                API keys below, then set the prompt/model per-device in the Edit
                Device dialog.
              </v-alert>

              <v-text-field
                v-model="form.openai_api_key"
                label="OpenAI API Key"
                type="password"
                variant="outlined"
                class="mb-1"
                hint="sk-..."
                persistent-hint
              ></v-text-field>
              <div class="text-caption text-grey ml-2 mb-4">
                Get your API key at
                <a
                  href="https://platform.openai.com/api-keys"
                  target="_blank"
                  class="text-primary text-decoration-none"
                  >platform.openai.com</a
                >
              </div>

              <v-text-field
                v-model="form.google_api_key"
                label="Google Gemini API Key"
                type="password"
                variant="outlined"
                class="mb-1"
                persistent-hint
              ></v-text-field>
              <div class="text-caption text-grey ml-2 mb-4">
                Get your API key at
                <a
                  href="https://aistudio.google.com/app/apikey"
                  target="_blank"
                  class="text-primary text-decoration-none"
                  >aistudio.google.com</a
                >
              </div>

              <v-btn color="primary" @click="save">Save API Keys</v-btn>
            </v-card-text>
          </v-window-item>
        </v-window>
      </v-card-text>
    </v-card>

    <!-- Settings Card -->
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon icon="mdi-cog" class="mr-2" />
        Settings
      </v-card-title>

      <div
        v-if="store.loading"
        class="d-flex justify-center align-center pa-10"
      >
        <v-progress-circular
          indeterminate
          color="primary"
        ></v-progress-circular>
      </div>

      <div v-else>
        <v-tabs v-model="activeMainTab" color="primary" grow>
          <v-tab value="devices">Devices</v-tab>
          <v-tab value="security">System</v-tab>
        </v-tabs>

        <v-window v-model="activeMainTab" :touch="false">
          <!-- System Tab (server + security) -->
          <v-window-item value="security">
            <v-card-text>
              <h3 class="text-h6 mb-3">Server</h3>
              <v-text-field
                v-model="form.device_image_base_url"
                label="Server URL for devices"
                :placeholder="derivedServerBase"
                persistent-placeholder
                hint="Base address your photo frames use to reach this server. Leave empty to derive it from your browser's address. Set this for Tailscale, reverse proxies, or custom domains."
                persistent-hint
                variant="outlined"
                density="compact"
                clearable
                @update:model-value="saveSettingsInternal()"
              ></v-text-field>

              <v-divider class="my-6"></v-divider>

              <SecurityTab :devices="availableDevices" />
            </v-card-text>
          </v-window-item>
          <!-- Devices Tab -->
          <v-window-item value="devices">
            <v-card-text>
              <div class="d-flex justify-end mb-4">
                <v-btn
                  color="primary"
                  prepend-icon="mdi-plus"
                  @click="openAddDeviceDialog"
                  :loading="deviceListLoading"
                >
                  Add Device
                </v-btn>
              </div>

              <div
                v-if="deviceListLoading && availableDevices.length === 0"
                class="d-flex justify-center align-center pa-10"
              >
                <v-progress-circular
                  indeterminate
                  color="primary"
                ></v-progress-circular>
              </div>

              <template v-else>
                <!-- Table on tablet/desktop -->
                <v-table
                  v-if="!smAndDown"
                  density="comfortable"
                  class="border rounded"
                >
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Model</th>
                      <th>Host</th>
                      <th class="text-right">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="device in availableDevices" :key="device.id">
                      <td>{{ device.name }}</td>
                      <td>
                        {{
                          device.board_name ||
                          `${device.width}x${device.height}`
                        }}
                        <v-chip
                          v-if="device.display_type?.startsWith('gc')"
                          size="x-small"
                          class="ml-2"
                        >
                          Grayscale
                        </v-chip>
                      </td>
                      <td>
                        {{ device.host }}
                      </td>
                      <td class="text-right">
                        <v-btn
                          color="primary"
                          variant="text"
                          size="small"
                          icon="mdi-open-in-new"
                          title="Open Device"
                          :href="deviceURL(device)"
                          target="_blank"
                        ></v-btn>
                        <v-btn
                          color="primary"
                          variant="text"
                          size="small"
                          icon="mdi-pencil"
                          title="Edit Device"
                          @click="editDevice(device)"
                        ></v-btn>
                        <v-btn
                          color="error"
                          variant="text"
                          size="small"
                          icon="mdi-delete"
                          title="Delete Device"
                          @click="removeDevice(device.id)"
                        ></v-btn>
                      </td>
                    </tr>
                    <tr v-if="availableDevices.length === 0">
                      <td colspan="4" class="text-center text-grey py-4">
                        No devices added.
                      </td>
                    </tr>
                  </tbody>
                </v-table>

                <!-- Stacked list on phones (no horizontal scroll) -->
                <v-list
                  v-else-if="availableDevices.length"
                  class="border rounded"
                  density="comfortable"
                >
                  <template
                    v-for="(device, i) in availableDevices"
                    :key="device.id"
                  >
                    <v-divider v-if="i > 0"></v-divider>
                    <v-list-item>
                      <v-list-item-title class="text-truncate">
                        {{ device.name }}
                      </v-list-item-title>
                      <v-list-item-subtitle class="text-truncate">
                        {{
                          device.board_name ||
                          `${device.width}x${device.height}`
                        }}
                        <v-chip
                          v-if="device.display_type?.startsWith('gc')"
                          size="x-small"
                          class="ml-2"
                        >
                          Grayscale
                        </v-chip>
                      </v-list-item-subtitle>
                      <v-list-item-subtitle class="text-truncate">
                        {{ device.host }}
                      </v-list-item-subtitle>
                      <template #append>
                        <v-btn
                          color="primary"
                          variant="text"
                          size="small"
                          icon="mdi-open-in-new"
                          title="Open Device"
                          :href="deviceURL(device)"
                          target="_blank"
                        ></v-btn>
                        <v-btn
                          color="primary"
                          variant="text"
                          size="small"
                          icon="mdi-pencil"
                          title="Edit Device"
                          @click="editDevice(device)"
                        ></v-btn>
                        <v-btn
                          color="error"
                          variant="text"
                          size="small"
                          icon="mdi-delete"
                          title="Delete Device"
                          @click="removeDevice(device.id)"
                        ></v-btn>
                      </template>
                    </v-list-item>
                  </template>
                </v-list>
                <div v-else class="text-center text-grey py-4">
                  No devices added.
                </div>
              </template>

              <!-- Edit Device Dialog (tabbed like device webapp) -->
              <v-dialog
                v-model="showEditDeviceDialog"
                max-width="1100px"
                :fullscreen="smAndDown"
                scrollable
              >
                <v-card>
                  <v-card-title>{{
                    isAddingDevice
                      ? 'Add Device'
                      : editingDevice.name || 'Edit Device'
                  }}</v-card-title>
                  <v-tabs
                    v-if="!isAddingDevice"
                    v-model="deviceDialogTab"
                    density="compact"
                    show-arrows
                  >
                    <v-tab value="general">General</v-tab>
                    <v-tab value="autoRotate">Auto Rotate</v-tab>
                    <v-tab value="power">Power</v-tab>
                    <v-tab value="homeAssistant">Home Assistant</v-tab>
                    <v-tab value="processing">Processing</v-tab>
                    <v-tab value="ai">AI Generation</v-tab>
                    <v-tab v-if="!isGrayscale" value="palette">Palette</v-tab>
                    <v-tab v-if="isGrayscale" value="grayscale"
                      >Grayscale</v-tab
                    >
                  </v-tabs>
                  <v-card-text
                    :style="
                      isAddingDevice || smAndDown
                        ? ''
                        : 'height: 455px; overflow-y: auto'
                    "
                  >
                    <!-- Add Device: just host input -->
                    <div v-if="isAddingDevice" class="mt-2">
                      <v-text-field
                        v-model="editingDevice.host"
                        label="Device Host / IP"
                        variant="outlined"
                        hint="e.g., photoframe.local or 192.168.1.100"
                        persistent-hint
                        autofocus
                      ></v-text-field>
                    </div>

                    <!-- Edit Device: full tabbed UI -->
                    <v-tabs-window
                      v-if="!isAddingDevice"
                      v-model="deviceDialogTab"
                      :touch="false"
                    >
                      <!-- General Tab -->
                      <v-tabs-window-item value="general">
                        <v-row class="mt-1">
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model="editingDevice.name"
                              label="Device Name"
                              variant="outlined"
                              density="compact"
                              hide-details
                            ></v-text-field>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model="editingDevice.host"
                              label="Host / IP"
                              variant="outlined"
                              density="compact"
                              hide-details
                            ></v-text-field>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-select
                              v-model="deviceConfig.display_orientation"
                              :items="orientationOptions"
                              label="Display Orientation"
                              variant="outlined"
                              density="compact"
                            ></v-select>
                          </v-col>
                          <v-col cols="12" md="6">
                            <v-select
                              v-model="deviceConfig.display_rotation_deg"
                              :items="[
                                { title: '0°', value: 0 },
                                { title: '90°', value: 90 },
                                { title: '180°', value: 180 },
                                { title: '270°', value: 270 },
                              ]"
                              label="Display Rotation (deg)"
                              variant="outlined"
                              density="compact"
                            ></v-select>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model.number="deviceConfig.timezone_offset"
                              label="Timezone (UTC offset)"
                              type="number"
                              :min="-12"
                              :max="14"
                              :step="0.5"
                              variant="outlined"
                              density="compact"
                              hint="e.g., -8 for PST, +1 for CET, +8 for CST"
                              persistent-hint
                            ></v-text-field>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model="deviceConfig.ntp_server"
                              label="NTP Server"
                              variant="outlined"
                              density="compact"
                              hint="e.g., pool.ntp.org"
                              persistent-hint
                            ></v-text-field>
                          </v-col>
                        </v-row>
                      </v-tabs-window-item>

                      <!-- Auto Rotate Tab -->
                      <v-tabs-window-item value="autoRotate">
                        <v-switch
                          v-model="deviceConfig.auto_rotate"
                          label="Enable Auto-Rotate"
                          color="primary"
                          hide-details
                          class="mt-2 mb-2"
                        />
                        <div class="ml-10">
                          <RotationSchedule
                            v-model="deviceConfig.rotate_cron"
                            :sleep="
                              deviceSupportsCron ? null : sleepPreviewWindow
                            "
                            :disabled="!deviceConfig.auto_rotate"
                            :supports-cron="deviceSupportsCron"
                          />
                          <v-select
                            v-model="deviceConfig.rotation_mode"
                            :items="[
                              { title: 'Local Storage', value: 'storage' },
                              { title: 'URL', value: 'url' },
                            ]"
                            label="Rotation Mode"
                            variant="outlined"
                            density="compact"
                            class="mt-4 mb-2"
                            :disabled="!deviceConfig.auto_rotate"
                          />

                          <!-- URL source config (shown when rotation mode is URL) -->
                          <div v-if="deviceConfig.rotation_mode === 'url'">
                            <v-checkbox
                              v-model="useThisServer"
                              label="Use this server as image source"
                              color="primary"
                              hide-details
                              class="mb-2"
                              :disabled="!deviceConfig.auto_rotate"
                            />

                            <!-- This server: source dropdown -->
                            <v-select
                              v-if="useThisServer"
                              v-model="selectedSource"
                              :items="sourceOptions"
                              label="Image Source"
                              variant="outlined"
                              density="compact"
                              hide-details
                              class="mb-2 ml-8"
                              :disabled="!deviceConfig.auto_rotate"
                            ></v-select>

                            <!-- Per-frame album selection for the chosen source -->
                            <div
                              v-if="
                                useThisServer &&
                                selectedSource === 'immich' &&
                                immichConnected
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Albums for this frame — leave all unchecked to
                                use every synced Immich photo.
                              </div>
                              <AlbumPicker
                                v-model="deviceImmichAlbumIds"
                                :albums="deviceImmichAlbumOptions"
                                label-field="name"
                                :max-height="220"
                                :disabled="!deviceConfig.auto_rotate"
                                empty-text="No synced Immich albums. Add some in Data Sources → Immich first."
                              ></AlbumPicker>
                            </div>

                            <div
                              v-if="
                                useThisServer &&
                                selectedSource === 'synology_photos' &&
                                form.synology_sid
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Albums for this frame — leave all unchecked to
                                use every synced Synology photo.
                              </div>
                              <AlbumPicker
                                v-model="deviceSynologyAlbumIds"
                                :albums="deviceSynologyAlbumOptions"
                                label-field="name"
                                :max-height="220"
                                :disabled="!deviceConfig.auto_rotate"
                                empty-text="No synced Synology albums. Add some in Data Sources → Synology first."
                              ></AlbumPicker>
                            </div>

                            <div
                              v-if="
                                useThisServer && selectedSource === 'unsplash'
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Topics for this frame — leave all unchecked to
                                use every synced Unsplash photo.
                              </div>
                              <AlbumPicker
                                v-model="deviceUnsplashAlbumIds"
                                :albums="deviceUnsplashAlbumOptions"
                                label-field="name"
                                :max-height="220"
                                :disabled="!deviceConfig.auto_rotate"
                                empty-text="No synced Unsplash topics. Add some in Data Sources → Unsplash first."
                              ></AlbumPicker>
                            </div>

                            <div
                              v-if="
                                useThisServer && selectedSource === 'pexels'
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Topics for this frame — leave all unchecked to
                                use every synced Pexels photo.
                              </div>
                              <AlbumPicker
                                v-model="devicePexelsAlbumIds"
                                :albums="devicePexelsAlbumOptions"
                                label-field="name"
                                :max-height="220"
                                :disabled="!deviceConfig.auto_rotate"
                                empty-text="No synced Pexels topics. Add some in Data Sources → Pexels first."
                              ></AlbumPicker>
                            </div>

                            <!-- Custom URL -->
                            <v-text-field
                              v-if="!useThisServer"
                              v-model="deviceConfig.image_url"
                              label="Image URL"
                              variant="outlined"
                              density="compact"
                              hide-details
                              class="mb-2 ml-8"
                              :disabled="!deviceConfig.auto_rotate"
                            />

                            <v-checkbox
                              v-model="deviceConfig.save_downloaded_images"
                              label="Save downloaded images to Downloads album"
                              color="primary"
                              hide-details
                              class="mb-2"
                              :disabled="!deviceConfig.auto_rotate"
                            />
                          </div>
                        </div>

                        <!-- Sleep Schedule: legacy quiet-hours, only for
                             pre-cron firmware. Newer firmware expresses the
                             active window in the cron rules instead. -->
                        <template v-if="!deviceSupportsCron">
                          <v-divider class="my-3" />
                          <v-switch
                            v-model="deviceConfig.sleep_schedule_enabled"
                            label="Enable Sleep Schedule"
                            color="primary"
                            hide-details
                            class="mb-2"
                          />
                          <div
                            v-if="deviceConfig.sleep_schedule_enabled"
                            class="ml-10"
                          >
                            <v-row dense>
                              <v-col cols="12" sm="6">
                                <v-text-field
                                  v-model="deviceConfig.sleep_start_time"
                                  label="From"
                                  type="time"
                                  variant="outlined"
                                  density="compact"
                                  hide-details
                                />
                              </v-col>
                              <v-col cols="12" sm="6">
                                <v-text-field
                                  v-model="deviceConfig.sleep_end_time"
                                  label="To"
                                  type="time"
                                  variant="outlined"
                                  density="compact"
                                  hide-details
                                />
                              </v-col>
                            </v-row>
                          </div>
                        </template>

                        <v-divider class="my-4" />

                        <!-- Display Settings section header -->
                        <div class="text-body-1 font-weight-medium mb-4">
                          Display Settings
                        </div>
                        <div class="ml-10">
                          <v-row dense>
                            <v-col cols="12" md="6">
                              <v-select
                                v-model="editingDevice.display_mode"
                                :items="[
                                  {
                                    title: 'Cover (fill, may crop)',
                                    value: 'cover',
                                  },
                                  {
                                    title: 'Fit (show entire photo)',
                                    value: 'fit',
                                  },
                                ]"
                                label="Photo Display Mode"
                                variant="outlined"
                                density="compact"
                                hide-details
                              ></v-select>
                            </v-col>
                            <v-col
                              v-if="editingDevice.display_mode === 'fit'"
                              cols="12"
                              md="6"
                            >
                              <v-select
                                v-model="editingDevice.background_color"
                                :items="[
                                  { title: 'White', value: 'white' },
                                  { title: 'Black', value: 'black' },
                                  { title: 'Red', value: 'red' },
                                  { title: 'Green', value: 'green' },
                                  { title: 'Blue', value: 'blue' },
                                  { title: 'Yellow', value: 'yellow' },
                                ]"
                                label="Fit Background Color"
                                variant="outlined"
                                density="compact"
                                hide-details
                              ></v-select>
                            </v-col>
                          </v-row>
                          <v-checkbox
                            v-model="editingDevice.enable_collage"
                            label="Enable Collage Mode"
                            color="primary"
                            hide-details
                            class="mt-2 mb-1"
                          ></v-checkbox>
                        </div>

                        <v-divider class="my-4" />

                        <!-- Overlay section -->
                        <div class="text-body-1 font-weight-medium mb-4">
                          Overlay
                        </div>
                        <div class="ml-10">
                          <div class="d-flex ga-4 flex-wrap">
                            <v-checkbox
                              v-model="editingDevice.show_date"
                              label="Show Date"
                              color="primary"
                              hide-details
                            ></v-checkbox>
                            <v-checkbox
                              v-model="editingDevice.show_photo_date"
                              label="Show Photo Date"
                              color="primary"
                              hide-details
                            ></v-checkbox>
                            <v-checkbox
                              v-model="editingDevice.show_weather"
                              label="Show Weather"
                              color="primary"
                              hide-details
                            ></v-checkbox>
                          </div>
                          <v-select
                            v-if="editingDevice.show_date"
                            v-model="editingDevice.date_format"
                            :items="dateFormatOptions"
                            item-title="label"
                            item-value="value"
                            label="Date Format"
                            variant="outlined"
                            density="compact"
                            hide-details
                            class="mt-3"
                          ></v-select>
                          <v-row
                            v-if="editingDevice.show_weather"
                            dense
                            class="mt-3"
                          >
                            <v-col cols="12" sm="6">
                              <v-text-field
                                v-model.number="editingDevice.weather_lat"
                                label="Latitude"
                                variant="outlined"
                                density="compact"
                                hide-details
                                type="number"
                              ></v-text-field>
                            </v-col>
                            <v-col cols="12" sm="6">
                              <v-text-field
                                v-model.number="editingDevice.weather_lon"
                                label="Longitude"
                                variant="outlined"
                                density="compact"
                                hide-details
                                type="number"
                              ></v-text-field>
                            </v-col>
                          </v-row>
                          <v-tooltip
                            :disabled="
                              form.google_calendar_connected === 'true'
                            "
                            location="top"
                            text="Connect Google Calendar in Data Sources first"
                          >
                            <template #activator="{ props }">
                              <div v-bind="props" class="d-inline-flex">
                                <v-checkbox
                                  v-model="editingDevice.show_calendar"
                                  label="Show Google Calendar Events"
                                  color="primary"
                                  hide-details
                                  class="mt-2 mb-1"
                                  :disabled="
                                    form.google_calendar_connected !== 'true'
                                  "
                                ></v-checkbox>
                              </div>
                            </template>
                          </v-tooltip>
                          <v-select
                            v-if="
                              editingDevice.show_calendar &&
                              form.google_calendar_connected === 'true'
                            "
                            v-model="editingDevice.calendar_id"
                            :items="calendars"
                            item-title="summary"
                            item-value="id"
                            label="Select Calendar"
                            variant="outlined"
                            density="compact"
                            class="mt-2"
                            :loading="!calendarLoaded"
                          ></v-select>
                        </div>

                        <v-divider class="my-4" />

                        <!-- Layout section -->
                        <div class="text-body-1 font-weight-medium mb-4">
                          Layout
                        </div>
                        <div class="ml-10">
                          <div class="d-flex flex-wrap ga-3 mb-3">
                            <v-card
                              v-for="opt in filteredLayoutOptions"
                              :key="opt.value"
                              :variant="
                                editingDevice.layout === opt.value
                                  ? 'outlined'
                                  : 'flat'
                              "
                              :color="
                                editingDevice.layout === opt.value
                                  ? 'primary'
                                  : undefined
                              "
                              class="layout-preview-card pa-2 text-center"
                              style="width: 100px; cursor: pointer"
                              @click="editingDevice.layout = opt.value"
                            >
                              <div
                                class="layout-preview mb-1"
                                v-html="
                                  getLayoutPreviewSvg(
                                    opt.value,
                                    deviceConfig.display_orientation ||
                                      editingDevice.orientation ||
                                      'landscape'
                                  )
                                "
                              ></div>
                              <div
                                class="text-caption"
                                style="line-height: 1.2"
                              >
                                {{ opt.title }}
                              </div>
                            </v-card>
                          </div>
                        </div>
                      </v-tabs-window-item>

                      <!-- Power Tab -->
                      <v-tabs-window-item value="power">
                        <v-switch
                          v-model="deviceConfig.deep_sleep_enabled"
                          label="Enable Deep Sleep"
                          color="primary"
                          class="mt-2"
                          hide-details
                        />
                        <v-alert
                          type="info"
                          variant="tonal"
                          density="compact"
                          class="mt-4"
                        >
                          <strong>Power Consumption Notice</strong><br />
                          When deep sleep is enabled, the device sleeps between
                          image rotations to save power. WiFi is only active
                          during image fetch.
                        </v-alert>
                      </v-tabs-window-item>

                      <!-- Home Assistant Tab -->
                      <v-tabs-window-item value="homeAssistant">
                        <v-text-field
                          v-model="deviceConfig.ha_url"
                          label="Home Assistant URL"
                          variant="outlined"
                          density="compact"
                          class="mt-2"
                          hint="e.g., http://homeassistant.local:8123"
                          persistent-hint
                          placeholder="http://homeassistant.local:8123"
                        />
                      </v-tabs-window-item>

                      <!-- Processing Tab (matches device webapp ProcessingControls) -->
                      <v-tabs-window-item value="processing">
                        <v-row class="mt-1">
                          <v-col cols="12">
                            <v-card variant="outlined" class="mb-2">
                              <v-card-subtitle class="pt-3"
                                >Processing Preset</v-card-subtitle
                              >
                              <v-card-text>
                                <v-btn-toggle
                                  v-model="processingPreset"
                                  mandatory
                                  color="primary"
                                  variant="outlined"
                                  @update:model-value="applyProcessingPreset"
                                >
                                  <v-btn
                                    v-for="p in processingPresetOptions"
                                    :key="p.value"
                                    :value="p.value"
                                  >
                                    {{ p.title }}
                                  </v-btn>
                                </v-btn-toggle>
                              </v-card-text>
                            </v-card>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="4">
                            <v-select
                              v-model="deviceProcessing.ditherAlgorithm"
                              :items="ditherOptions"
                              item-title="title"
                              item-value="value"
                              label="Dithering Algorithm"
                              variant="outlined"
                              density="compact"
                            />
                          </v-col>
                          <v-col cols="12" md="4">
                            <v-select
                              v-model="deviceProcessing.colorMethod"
                              :items="[
                                { title: 'RGB', value: 'rgb' },
                                { title: 'LAB', value: 'lab' },
                              ]"
                              label="Color Matching"
                              variant="outlined"
                              density="compact"
                            />
                          </v-col>
                        </v-row>

                        <v-row>
                          <v-col cols="12" md="4">
                            <v-slider
                              v-model="deviceProcessing.exposure"
                              :min="0.5"
                              :max="2.0"
                              :step="0.01"
                              label="Exposure"
                              thumb-label
                              color="primary"
                            >
                              <template #append>
                                <span class="text-body-2">{{
                                  deviceProcessing.exposure.toFixed(2)
                                }}</span>
                              </template>
                            </v-slider>
                          </v-col>
                          <v-col v-if="!isGrayscale" cols="12" md="4">
                            <v-slider
                              v-model="deviceProcessing.saturation"
                              :min="0.5"
                              :max="2.0"
                              :step="0.01"
                              label="Saturation"
                              thumb-label
                              color="primary"
                            >
                              <template #append>
                                <span class="text-body-2">{{
                                  deviceProcessing.saturation.toFixed(2)
                                }}</span>
                              </template>
                            </v-slider>
                          </v-col>
                          <v-col cols="12" md="4">
                            <v-checkbox
                              v-model="deviceProcessing.compressDynamicRange"
                              label="Compress Dynamic Range"
                              hint="Map brightness to display's actual white point"
                              persistent-hint
                              color="primary"
                            />
                          </v-col>
                        </v-row>

                        <v-row>
                          <v-col cols="12" md="4">
                            <v-select
                              v-model="deviceProcessing.toneMode"
                              :items="[
                                { title: 'Contrast', value: 'contrast' },
                                { title: 'S-Curve', value: 'scurve' },
                              ]"
                              label="Tone Mapping"
                              variant="outlined"
                              density="compact"
                            />
                          </v-col>
                          <v-col
                            v-if="deviceProcessing.toneMode !== 'scurve'"
                            cols="12"
                            md="4"
                          >
                            <v-slider
                              v-model="deviceProcessing.contrast"
                              :min="0.5"
                              :max="2.0"
                              :step="0.01"
                              label="Contrast"
                              thumb-label
                              color="primary"
                            >
                              <template #append>
                                <span class="text-body-2">{{
                                  deviceProcessing.contrast.toFixed(2)
                                }}</span>
                              </template>
                            </v-slider>
                          </v-col>
                        </v-row>

                        <v-expand-transition>
                          <v-card
                            v-if="deviceProcessing.toneMode === 'scurve'"
                            variant="tonal"
                            class="mt-2"
                          >
                            <v-card-subtitle class="pt-3"
                              >S-Curve Parameters</v-card-subtitle
                            >
                            <v-card-text>
                              <v-row>
                                <v-col cols="12" md="6">
                                  <v-slider
                                    v-model="deviceProcessing.strength"
                                    :min="0"
                                    :max="1"
                                    :step="0.01"
                                    label="Strength"
                                    thumb-label
                                    color="primary"
                                  >
                                    <template #append
                                      ><span class="text-body-2">{{
                                        deviceProcessing.strength.toFixed(2)
                                      }}</span></template
                                    >
                                  </v-slider>
                                </v-col>
                                <v-col cols="12" md="6">
                                  <v-slider
                                    v-model="deviceProcessing.shadowBoost"
                                    :min="0"
                                    :max="1"
                                    :step="0.01"
                                    label="Shadow Boost"
                                    thumb-label
                                    color="primary"
                                  >
                                    <template #append
                                      ><span class="text-body-2">{{
                                        deviceProcessing.shadowBoost.toFixed(2)
                                      }}</span></template
                                    >
                                  </v-slider>
                                </v-col>
                                <v-col cols="12" md="6">
                                  <v-slider
                                    v-model="deviceProcessing.highlightCompress"
                                    :min="0.5"
                                    :max="5"
                                    :step="0.01"
                                    label="Highlight Compress"
                                    thumb-label
                                    color="primary"
                                  >
                                    <template #append
                                      ><span class="text-body-2">{{
                                        deviceProcessing.highlightCompress.toFixed(
                                          2
                                        )
                                      }}</span></template
                                    >
                                  </v-slider>
                                </v-col>
                                <v-col cols="12" md="6">
                                  <v-slider
                                    v-model="deviceProcessing.midpoint"
                                    :min="0.3"
                                    :max="0.7"
                                    :step="0.01"
                                    label="Midpoint"
                                    thumb-label
                                    color="primary"
                                  >
                                    <template #append
                                      ><span class="text-body-2">{{
                                        deviceProcessing.midpoint.toFixed(2)
                                      }}</span></template
                                    >
                                  </v-slider>
                                </v-col>
                              </v-row>
                            </v-card-text>
                          </v-card>
                        </v-expand-transition>
                      </v-tabs-window-item>

                      <!-- AI Generation Tab -->
                      <v-tabs-window-item value="ai">
                        <v-alert
                          type="info"
                          variant="tonal"
                          density="compact"
                          class="mt-2 mb-4"
                        >
                          API keys are stored on the device for client-side AI
                          image generation. Server-side AI provider/model/prompt
                          are used when the image source is set to AI
                          Generation.
                        </v-alert>

                        <v-text-field
                          v-model="deviceConfig.openai_api_key"
                          label="OpenAI API Key"
                          variant="outlined"
                          type="password"
                          hint="sk-..."
                          persistent-hint
                          class="mb-2"
                        />
                        <v-text-field
                          v-model="deviceConfig.google_api_key"
                          label="Google Gemini API Key"
                          variant="outlined"
                          type="password"
                          class="mb-4"
                        />

                        <v-divider class="mb-4" />
                        <div class="text-subtitle-2 mb-2">
                          Server-Side AI Generation
                        </div>

                        <v-select
                          v-model="editingDevice.ai_provider"
                          :items="[
                            { title: 'None', value: '' },
                            { title: 'OpenAI', value: 'openai' },
                            { title: 'Google Gemini', value: 'google' },
                          ]"
                          label="AI Provider"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-3"
                        ></v-select>
                        <v-select
                          v-if="editingDevice.ai_provider"
                          v-model="editingDevice.ai_model"
                          :items="
                            aiModelOptionsForProvider(editingDevice.ai_provider)
                          "
                          label="Model"
                          variant="outlined"
                          density="compact"
                          hide-details
                          class="mb-3"
                        ></v-select>
                        <v-textarea
                          v-if="editingDevice.ai_provider"
                          v-model="editingDevice.ai_prompt"
                          label="Prompt"
                          variant="outlined"
                          density="compact"
                          rows="3"
                          placeholder="A beautiful landscape painting..."
                          hide-details
                        ></v-textarea>
                      </v-tabs-window-item>

                      <!-- Palette Tab (matches device webapp PaletteCalibration) -->
                      <v-tabs-window-item value="palette">
                        <v-row class="mt-2">
                          <v-col
                            v-for="colorName in paletteColors"
                            :key="colorName"
                            cols="6"
                            md="4"
                            lg="2"
                          >
                            <v-card variant="outlined">
                              <div
                                class="color-swatch"
                                :style="{
                                  backgroundColor: `rgb(${devicePalette[colorName].r}, ${devicePalette[colorName].g}, ${devicePalette[colorName].b})`,
                                }"
                              />
                              <v-card-text class="pa-2">
                                <div
                                  class="text-subtitle-2 text-capitalize mb-2"
                                >
                                  {{ colorName }}
                                </div>
                                <v-text-field
                                  v-model.number="devicePalette[colorName].r"
                                  label="R"
                                  type="number"
                                  :min="0"
                                  :max="255"
                                  density="compact"
                                  variant="outlined"
                                  class="mb-1"
                                />
                                <v-text-field
                                  v-model.number="devicePalette[colorName].g"
                                  label="G"
                                  type="number"
                                  :min="0"
                                  :max="255"
                                  density="compact"
                                  variant="outlined"
                                  class="mb-1"
                                />
                                <v-text-field
                                  v-model.number="devicePalette[colorName].b"
                                  label="B"
                                  type="number"
                                  :min="0"
                                  :max="255"
                                  density="compact"
                                  variant="outlined"
                                />
                              </v-card-text>
                            </v-card>
                          </v-col>
                        </v-row>
                        <v-btn
                          variant="text"
                          color="error"
                          size="small"
                          class="mt-2"
                          @click="
                            Object.assign(devicePalette, {
                              black: { r: 2, g: 2, b: 2 },
                              white: { r: 190, g: 200, b: 200 },
                              yellow: { r: 205, g: 202, b: 0 },
                              red: { r: 135, g: 19, b: 0 },
                              blue: { r: 5, g: 64, b: 158 },
                              green: { r: 39, g: 102, b: 60 },
                            })
                          "
                          >Reset to Defaults</v-btn
                        >
                      </v-tabs-window-item>

                      <!-- Grayscale Calibration Tab (mirrors device webapp GrayscaleCalibration) -->
                      <v-tabs-window-item value="grayscale">
                        <v-alert
                          type="info"
                          variant="tonal"
                          density="compact"
                          class="mt-2 mb-4"
                        >
                          Calibrate the measured luminance of your grayscale
                          (GC16) panel. These two values are the relative
                          luminance (Y, 0&ndash;1) of full black and full white
                          as displayed by your specific panel. They drive the
                          grayscale dithering and dynamic-range compression.
                        </v-alert>

                        <p class="text-body-2 mb-4">
                          Keeping <strong>black luminance at 0</strong> renders
                          shadows as pure black for a punchier, higher-contrast
                          result. Raising it toward your panel's real black
                          makes the on-screen preview match what the panel
                          actually shows (WYSIWYG). You can measure these by
                          displaying a full-black image, then a full-white
                          image, and metering the panel's luminance.
                        </p>

                        <v-row>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model.number="grayscaleCal.black_y"
                              label="Black luminance (Y)"
                              type="number"
                              :min="0"
                              :max="1"
                              :step="0.001"
                              variant="outlined"
                              density="compact"
                              hint="Measured relative luminance of full black (0–1). 0 keeps shadows pure black."
                              persistent-hint
                            />
                          </v-col>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model.number="grayscaleCal.white_y"
                              label="White luminance (Y)"
                              type="number"
                              :min="0"
                              :max="1"
                              :step="0.01"
                              variant="outlined"
                              density="compact"
                              hint="Measured relative luminance of full white (0–1). Default 0.65."
                              persistent-hint
                            />
                          </v-col>
                          <v-col cols="12" md="6">
                            <v-text-field
                              v-model.number="grayscaleCal.gamma"
                              label="Mid-tone gamma"
                              type="number"
                              :min="0.2"
                              :max="4"
                              :step="0.05"
                              variant="outlined"
                              density="compact"
                              hint="Mid-level shaping; 1.0 = neutral, >1 darkens mid-tones. Default 1.42."
                              persistent-hint
                            />
                          </v-col>
                        </v-row>

                        <v-btn
                          variant="text"
                          color="error"
                          size="small"
                          class="mt-2"
                          @click="
                            Object.assign(grayscaleCal, {
                              black_y: 0.009,
                              white_y: 0.65,
                              gamma: 1.42,
                            })
                          "
                          >Reset to Defaults</v-btn
                        >
                      </v-tabs-window-item>
                    </v-tabs-window>
                  </v-card-text>
                  <v-card-actions>
                    <v-btn
                      v-if="!isAddingDevice"
                      color="info"
                      variant="text"
                      size="small"
                      :loading="syncingFromDevice"
                      @click="syncFromDevice"
                    >
                      <v-icon start>mdi-sync</v-icon>
                      Sync from Device
                    </v-btn>
                    <v-spacer></v-spacer>
                    <v-btn
                      color="grey"
                      variant="text"
                      @click="showEditDeviceDialog = false"
                      >Cancel</v-btn
                    >
                    <v-btn
                      color="primary"
                      @click="saveDevice"
                      :loading="savingDeviceConfig"
                      >{{ isAddingDevice ? 'Add' : 'Save' }}</v-btn
                    >
                  </v-card-actions>
                </v-card>
              </v-dialog>
            </v-card-text>
          </v-window-item>
        </v-window>
      </div>

      <ConfirmDialog ref="confirmDialog" />
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, nextTick, reactive, ref, computed, watch } from 'vue';
import { useDisplay } from 'vuetify';
import { useSettingsStore } from '../stores/settings';
import { useSynologyStore } from '../stores/synology';
import { useImmichStore } from '../stores/immich';
import { useUnsplashStore, usePexelsStore } from '../stores/createSourceStore';
import { useGalleryStore } from '../stores/gallery';
import { useSnackbar } from '../composables/useSnackbar';
import {
  api,
  listDevices,
  addDevice,
  deleteDevice,
  updateDevice,
  refreshDevice,
  type Device,
  createURLSource,
  updateURLSource,
  listURLSources,
  deleteURLSource,
  getDeviceConfig,
  updateDeviceConfig,
  listCalendars,
  googleCalendarLogin,
  googleCalendarLogout,
} from '../api';
import Gallery from './Gallery.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import AlbumPicker from './AlbumPicker.vue';
import TopicManager from './TopicManager.vue';
import SecurityTab from './SecurityTab.vue';
import RotationSchedule from './RotationSchedule.vue';
import { intervalToCron, cronToInterval, isValidCron } from '../utils/cron';

const { smAndDown } = useDisplay(); // true on phones / small tablets
const store = useSettingsStore();
const synologyStore = useSynologyStore();
const immichStore = useImmichStore();
const unsplashStore = useUnsplashStore();
const pexelsStore = usePexelsStore();
const immichConnected = ref(false);
const unsplashConnected = ref(false);
const pexelsConnected = ref(false);

// Topic-source store lookup by source key (unsplash/pexels).
const topicStores: Record<string, any> = {
  unsplash: unsplashStore,
  pexels: pexelsStore,
};

// Immich "albums to sync" selection (Part A)
const syncAlbumIds = ref<string[]>([]);
const syncFavorites = ref(false);
const syncAll = ref(false);
const syncMemories = ref(false);
const savingSyncSelection = ref(false);

// Synology "albums to sync" selection (mirror of Immich, no virtual modes)
const synologySyncAlbumIds = ref<string[]>([]);
const savingSynologySyncSelection = ref(false);

// Album selection auto-saves (debounced) but does NOT trigger an import — the
// user syncs manually or is prompted on leaving. Suppress the auto-save while
// applying persisted state programmatically (load / post-save refresh).
// *SyncDirty marks selection changed since the last actual sync.
let suppressSyncAutoSave = false;
let immichSyncSaveTimer: number | null = null;
let synologySyncSaveTimer: number | null = null;
const immichSyncDirty = ref(false);
const synologySyncDirty = ref(false);
// Topic sources (Unsplash/Pexels): topics auto-save on add/remove but do not
// import until the user syncs or is prompted on leaving the source.
const topicSyncDirty = reactive<Record<string, boolean>>({
  unsplash: false,
  pexels: false,
});

// Per-device Immich album bindings (Part B)
const deviceImmichAlbumIds = ref<number[]>([]);
// Per-device Synology album bindings (Part C)
const deviceSynologyAlbumIds = ref<number[]>([]);
// Per-device topic-source (unsplash/pexels) album bindings.
const deviceUnsplashAlbumIds = ref<number[]>([]);
const devicePexelsAlbumIds = ref<number[]>([]);
const galleryStore = useGalleryStore();
const activeMainTab = ref('devices');
// Unified source selector for the top card (per-source gallery + settings).
const sourceTab = ref('gallery');
const sourceHasGallery = computed(() =>
  [
    'gallery',
    'immich',
    'synology_photos',
    'google_photos',
    'unsplash',
    'pexels',
  ].includes(sourceTab.value)
);
const confirmDialog = ref();

// Image Source Binding State
const useThisServer = ref(true);
const selectedSource = ref('immich');
const sourceOptions = [
  { title: 'Gallery', value: 'gallery' },
  { title: 'Immich', value: 'immich' },
  { title: 'Google Photos', value: 'google_photos' },
  { title: 'Synology Photos', value: 'synology_photos' },
  { title: 'Unsplash', value: 'unsplash' },
  { title: 'Pexels', value: 'pexels' },
  { title: 'URL Proxy', value: 'url_proxy' },
  { title: 'AI Generation', value: 'ai_generation' },
  { title: 'Fractal (Mandelbrot zoom)', value: 'fractal' },
  { title: 'DLA (diffusion-limited aggregation)', value: 'dla' },
];

// URL Proxy State
const urlSources = ref<any[]>([]); // Renamed from urlImages
const showAddURLDialog = ref(false);
const isEditingURL = ref(false);
const editingURLId = ref<number | null>(null);
const newURL = reactive({
  url: '',
  device_ids: [] as number[],
});

// URL Proxy Functions
const loadURLSources = async () => {
  try {
    const res = await listURLSources();
    urlSources.value = res;
  } catch (e) {
    console.error('Failed to load URL sources', e);
  }
};

const openAddURLDialog = () => {
  isEditingURL.value = false;
  editingURLId.value = null;
  newURL.url = '';
  newURL.device_ids = [];
  showAddURLDialog.value = true;
};

const openEditURLDialog = (src: any) => {
  isEditingURL.value = true;
  editingURLId.value = src.id;
  newURL.url = src.url;
  // device_ids might come as objects or ids depending on API? API returns list of uints.
  newURL.device_ids = src.device_ids || [];
  showAddURLDialog.value = true;
};

const saveURLSource = async () => {
  if (!newURL.url) {
    showMessage('URL is required', true);
    return;
  }
  try {
    if (isEditingURL.value && editingURLId.value) {
      await updateURLSource(editingURLId.value, newURL.url, newURL.device_ids);
      showMessage('URL source updated');
    } else {
      await createURLSource(newURL.url, newURL.device_ids);
      showMessage('URL source added');
    }
    showAddURLDialog.value = false;
    await loadURLSources();
  } catch (e: any) {
    showMessage(
      'Failed to save URL source: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const deleteURLSourceWrapper = async (id: number) => {
  if (!(await confirmDialog.value.open('Delete this URL Source?'))) return;
  try {
    await deleteURLSource(id);
    showMessage('URL source deleted');
    await loadURLSources();
  } catch (e: any) {
    showMessage('Failed to delete URL source', true);
  }
};

// Calendar State
const calendars = ref<any[]>([]);
const calendarConnected = ref(false);
const calendarLoaded = ref(false);

const loadCalendars = async () => {
  if (form.google_calendar_connected !== 'true') {
    calendarLoaded.value = true;
    return;
  }
  try {
    const cals = await listCalendars();
    calendars.value = cals;
    calendarConnected.value = true;
  } catch (e: any) {
    if (e.response?.status === 403) {
      calendarConnected.value = false;
    } else {
      console.error('Failed to load calendars', e);
    }
  } finally {
    calendarLoaded.value = true;
  }
};

// Edit Device State
const showEditDeviceDialog = ref(false);
const editingDevice = reactive<Partial<Device>>({});
const deviceDialogTab = ref('general');

// Grayscale devices ("gc16", and any future "gc8"/"gc4") hide the color-only
// controls — the 6-color palette calibration and saturation don't apply.
const isGrayscale = computed(() =>
  (editingDevice.display_type ?? '').startsWith('gc')
);
// Don't strand the user on a tab that's hidden for the current device type:
// the Palette tab is color-only; the Grayscale tab is grayscale-only.
watch(isGrayscale, (gray) => {
  if (gray && deviceDialogTab.value === 'palette') {
    deviceDialogTab.value = 'general';
  } else if (!gray && deviceDialogTab.value === 'grayscale') {
    deviceDialogTab.value = 'general';
  }
});
const savingDeviceConfig = ref(false);
const syncingFromDevice = ref(false);

// Device config (synced remotely to device)
const deviceConfig = reactive<Record<string, any>>({
  auto_rotate: false,
  rotate_cron: ['0 */12 *'],
  rotation_mode: 'storage',
  image_url: '',
  save_downloaded_images: true,
  sleep_schedule_enabled: false,
  sleep_start_time: '23:00',
  sleep_end_time: '07:00',
  display_orientation: 'landscape',
  display_rotation_deg: 180,
  timezone_offset: 0,
  ntp_server: 'pool.ntp.org',
  deep_sleep_enabled: true,
  ha_url: '',
  openai_api_key: '',
  google_api_key: '',
});

// Whether the device firmware understands rotate_cron. Old firmware reports
// only rotate_interval and silently ignores a cron schedule. Best-effort:
// derived from the loaded config, which is the server's stored copy.
const deviceSupportsCron = ref(true);

// Device processing settings (synced remotely)
const deviceProcessing = reactive({
  exposure: 1.0,
  saturation: 1.0,
  toneMode: 'contrast',
  contrast: 1.0,
  strength: 0.5,
  shadowBoost: 0.0,
  highlightCompress: 0.0,
  midpoint: 0.5,
  colorMethod: 'rgb',
  ditherAlgorithm: 'floyd-steinberg',
  compressDynamicRange: true,
});

// Processing presets from epaper-image-convert library
import {
  getPresetOptions,
  getPreset,
  getDitherOptions,
} from '@aitjcize/epaper-image-convert';

const processingPreset = ref('custom');
const processingPresetOptions = [
  ...getPresetOptions(),
  { value: 'custom', title: 'Custom' },
];
const processingPresets: Record<string, Record<string, any>> = {};
for (const opt of getPresetOptions()) {
  const p = getPreset(opt.value);
  if (p) processingPresets[opt.value] = p;
}
const ditherOptions = getDitherOptions();

const applyProcessingPreset = (name: string) => {
  const preset = processingPresets[name];
  if (preset) {
    Object.assign(deviceProcessing, preset);
  }
  // 'custom' just keeps current values
};

// Detect current preset on load
// Match preset detection logic from device webapp: only compare keys present in the preset
const presetKeys = [
  'exposure',
  'saturation',
  'toneMode',
  'contrast',
  'strength',
  'shadowBoost',
  'highlightCompress',
  'midpoint',
  'colorMethod',
  'ditherAlgorithm',
  'compressDynamicRange',
];

const detectProcessingPreset = () => {
  for (const [name, preset] of Object.entries(processingPresets)) {
    const matches = presetKeys.every((k) => {
      if (!(k in preset)) return true; // Skip keys not in this preset
      const pv = (preset as any)[k];
      const dv = (deviceProcessing as any)[k];
      // Tolerant number compare: the device persists floats as 32-bit, so e.g.
      // 1.4 round-trips as 1.39999998 and exact matching would miss the preset.
      if (typeof pv === 'number' && typeof dv === 'number') {
        return Math.abs(pv - dv) < 1e-3;
      }
      return pv === dv;
    });
    if (matches) {
      processingPreset.value = name;
      return;
    }
  }
  processingPreset.value = 'custom';
};

// Re-detect preset when processing params change
watch(
  deviceProcessing,
  () => {
    detectProcessingPreset();
  },
  { deep: true }
);

// Device color palette (synced remotely)
const paletteColors = [
  'black',
  'white',
  'yellow',
  'red',
  'blue',
  'green',
] as const;
const devicePalette = reactive<
  Record<string, { r: number; g: number; b: number }>
>({
  black: { r: 2, g: 2, b: 2 },
  white: { r: 190, g: 200, b: 200 },
  yellow: { r: 205, g: 202, b: 0 },
  red: { r: 135, g: 19, b: 0 },
  blue: { r: 5, g: 64, b: 158 },
  green: { r: 39, g: 102, b: 60 },
});

// Grayscale (GC16) calibration — the panel-measured relative luminance (Y, 0..1)
// of full black / full white. Synced to the device through the same per-device
// palette path as the 6-color palette (color_palette: { black_y, white_y }).
const grayscaleCal = reactive<{
  black_y: number;
  white_y: number;
  gamma: number;
}>({
  black_y: 0.009,
  white_y: 0.65,
  gamma: 1.42,
});

// Auto-update mDNS hostname when device name changes
// Matches firmware's sanitize_hostname: lowercase, non-alnum → hyphen, no leading/trailing/consecutive hyphens
function deviceNameToHostname(name: string): string {
  let result = '';
  let lastWasHyphen = false;
  for (const c of name) {
    if (/[a-zA-Z0-9]/.test(c)) {
      result += c.toLowerCase();
      lastWasHyphen = false;
    } else if (!lastWasHyphen && result.length > 0) {
      result += '-';
      lastWasHyphen = true;
    }
  }
  // Remove trailing hyphen
  if (result.endsWith('-')) result = result.slice(0, -1);
  return result || 'photoframe';
}

watch(
  () => editingDevice.name,
  (newName) => {
    // Only auto-update if current host is an mDNS name
    if (!editingDevice.host?.endsWith('.local')) return;
    if (!newName) return;
    editingDevice.host = deviceNameToHostname(newName) + '.local';
  }
);

// Auto-fill weather coordinates from first device that has them
watch(
  () => editingDevice.show_weather,
  (enabled) => {
    if (!enabled) return;
    // Only fill if lat/lon are empty
    if (editingDevice.weather_lat && editingDevice.weather_lon) return;
    const donor = availableDevices.value.find(
      (d: Device) =>
        d.show_weather &&
        d.weather_lat &&
        d.weather_lon &&
        d.id !== editingDevice.id
    );
    if (donor) {
      editingDevice.weather_lat = donor.weather_lat;
      editingDevice.weather_lon = donor.weather_lon;
    }
  }
);

const orientationOptions = computed(() => {
  const w = editingDevice.width || 800;
  const h = editingDevice.height || 480;
  const wide = Math.max(w, h);
  const narrow = Math.min(w, h);
  return [
    { title: `Landscape (${wide}\u00d7${narrow})`, value: 'landscape' },
    { title: `Portrait (${narrow}\u00d7${wide})`, value: 'portrait' },
  ];
});

// Quiet-hours window (minutes since midnight) for the schedule preview.
const sleepPreviewWindow = computed(() => {
  const toMin = (hhmm: string) => {
    const [h, m] = (hhmm || '0:0').split(':').map(Number);
    return (h || 0) * 60 + (m || 0);
  };
  return {
    enabled: deviceConfig.sleep_schedule_enabled,
    start: toMin(deviceConfig.sleep_start_time),
    end: toMin(deviceConfig.sleep_end_time),
  };
});

const loadDeviceConfig = async (deviceId: number) => {
  try {
    const data = await getDeviceConfig(deviceId);
    const parse = (v: any) =>
      (typeof v === 'string' && v !== '{}' ? JSON.parse(v) : v) || {};

    // Config
    const cfg = parse(data.config);
    // Update device name from device config if available
    if (cfg.device_name) {
      editingDevice.name = cfg.device_name;
    }
    Object.assign(deviceConfig, {
      auto_rotate: cfg.auto_rotate ?? false,
      // Prefer rotate_cron; fall back to a legacy rotate_interval (older
      // firmware) so the schedule still shows correctly, else the default.
      rotate_cron:
        Array.isArray(cfg.rotate_cron) && cfg.rotate_cron.length
          ? cfg.rotate_cron
          : typeof cfg.rotate_interval === 'number'
            ? [intervalToCron(cfg.rotate_interval)]
            : ['0 */12 *'],
      rotation_mode: cfg.rotation_mode ?? 'storage',
      image_url: cfg.image_url ?? '',
      save_downloaded_images: cfg.save_downloaded_images ?? true,
    });
    // Old firmware reports rotate_interval and no rotate_cron.
    deviceSupportsCron.value = Array.isArray(cfg.rotate_cron);

    // Prefer the device's persisted `source` field. Fall back to parsing the
    // image_url for back-compat with devices configured before `source` existed.
    if (editingDevice.source) {
      useThisServer.value = true;
      selectedSource.value = editingDevice.source;
    } else {
      // Detect if image_url points to this server
      const imgUrl = cfg.image_url || '';
      let isThisServer = false;
      if (imgUrl.includes('/image')) {
        try {
          const imgHost = new URL(imgUrl).hostname;
          const serverHost = window.location.hostname;
          isThisServer = imgHost === serverHost;
        } catch {
          isThisServer = false;
        }
      }
      useThisServer.value = isThisServer;
      if (isThisServer) {
        const match = imgUrl.match(/\/image\/([^/?]+)/);
        if (match) {
          selectedSource.value = match[1];
        }
      }
    }

    Object.assign(deviceConfig, {
      sleep_schedule_enabled: cfg.sleep_schedule_enabled ?? false,
      display_orientation:
        cfg.display_orientation ?? deviceConfig.display_orientation,
      display_rotation_deg: cfg.display_rotation_deg ?? 180,
      deep_sleep_enabled: cfg.deep_sleep_enabled ?? true,
      ha_url: cfg.ha_url ?? '',
      ntp_server: cfg.ntp_server ?? 'pool.ntp.org',
      openai_api_key: cfg.openai_api_key ?? '',
      google_api_key: cfg.google_api_key ?? '',
    });
    const startMin = cfg.sleep_schedule_start ?? 1380;
    deviceConfig.sleep_start_time = `${String(Math.floor(startMin / 60)).padStart(2, '0')}:${String(startMin % 60).padStart(2, '0')}`;
    const endMin = cfg.sleep_schedule_end ?? 420;
    deviceConfig.sleep_end_time = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;

    // Parse POSIX timezone (e.g., "UTC-8" → 8, "UTC+1" → -1, POSIX sign is inverted)
    const tz = cfg.timezone || 'UTC0';
    const tzMatch = tz.match(/UTC([+-]?)(\d+)(?::(\d+))?/);
    if (tzMatch) {
      const sign = tzMatch[1] === '-' ? 1 : -1;
      const hours = parseInt(tzMatch[2]) || 0;
      const minutes = parseInt(tzMatch[3]) || 0;
      deviceConfig.timezone_offset = sign * (hours + minutes / 60);
    } else {
      deviceConfig.timezone_offset = 0;
    }

    // Processing settings
    const proc = parse(data.processing_settings);
    if (Object.keys(proc).length > 0) {
      Object.assign(deviceProcessing, {
        exposure: proc.exposure ?? 1.0,
        saturation: proc.saturation ?? 1.0,
        toneMode: proc.toneMode ?? 'contrast',
        contrast: proc.contrast ?? 1.0,
        strength: proc.strength ?? 0.5,
        shadowBoost: proc.shadowBoost ?? 0.0,
        highlightCompress: proc.highlightCompress ?? 0.0,
        midpoint: proc.midpoint ?? 0.5,
        colorMethod: proc.colorMethod ?? 'rgb',
        ditherAlgorithm: proc.ditherAlgorithm ?? 'floyd-steinberg',
        compressDynamicRange: proc.compressDynamicRange ?? true,
      });
    } else if (isGrayscale.value) {
      // New grayscale frame (no saved settings): default to the grayscale preset.
      applyProcessingPreset('grayscale');
    }
    detectProcessingPreset();

    // Color palette
    const pal = parse(data.color_palette);
    for (const color of paletteColors) {
      if (pal[color]) {
        devicePalette[color] = {
          r: pal[color].r ?? 0,
          g: pal[color].g ?? 0,
          b: pal[color].b ?? 0,
        };
      }
    }

    // Grayscale calibration shares the color_palette payload: a calibrated GC16
    // panel stores just { black_y, white_y }. Fall back to GC16 defaults so the
    // inputs always bind to a number. Round to 3 digits -- the firmware stores Y
    // as a float32, so a saved 0.90 round-trips as 0.899999976...
    const round3 = (v: number) => Math.round(v * 1000) / 1000;
    grayscaleCal.black_y = round3(
      typeof pal.black_y === 'number' ? pal.black_y : 0.009
    );
    grayscaleCal.white_y = round3(
      typeof pal.white_y === 'number' ? pal.white_y : 0.65
    );
    grayscaleCal.gamma = round3(
      typeof pal.gamma === 'number' ? pal.gamma : 1.42
    );
  } catch {
    // No config saved yet, use defaults
  }
};

const syncFromDevice = async () => {
  if (!editingDevice.id) return;
  syncingFromDevice.value = true;
  try {
    await refreshDevice(editingDevice.id);
    await loadDevices();
    // Re-load the updated device into the dialog
    const updated = availableDevices.value.find(
      (d: Device) => d.id === editingDevice.id
    );
    if (updated) Object.assign(editingDevice, updated);
    // Reload device config to reflect synced values
    await loadDeviceConfig(editingDevice.id!);
    showMessage('Settings synced from device');
  } catch (e: any) {
    showMessage(
      'Failed to sync: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    syncingFromDevice.value = false;
  }
};

const allLayoutOptions = [
  {
    title: 'Full Photo + Overlay',
    value: 'photo_overlay',
    orientations: ['portrait', 'landscape'],
  },
  {
    title: 'Photo + Info Strip',
    value: 'photo_info',
    orientations: ['portrait'],
  },
  { title: 'Side Panel', value: 'side_panel', orientations: ['landscape'] },
];

const filteredLayoutOptions = computed(() => {
  const orientation =
    deviceConfig.display_orientation ||
    editingDevice.orientation ||
    'landscape';
  return allLayoutOptions.filter((opt) =>
    opt.orientations.includes(orientation)
  );
});

// Auto-select first layout if current layout is not valid for orientation
watch(
  () => deviceConfig.display_orientation,
  () => {
    const valid = filteredLayoutOptions.value.map((o) => o.value);
    if (editingDevice.layout && !valid.includes(editingDevice.layout)) {
      editingDevice.layout = valid[0] || 'photo_overlay';
    }
  }
);

const getLayoutPreviewSvg = (layout: string, orientation: string) => {
  const isPortrait = orientation === 'portrait';
  const w = isPortrait ? 50 : 80;
  const h = isPortrait ? 70 : 50;
  const stroke = '#888';
  const photoFill = '#4a90d9';
  const infoFill = '#333';
  switch (layout) {
    case 'photo_info': {
      const photoH = Math.round(h * 0.6);
      return `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
        <rect width="${w}" height="${photoH}" fill="${photoFill}" rx="3"/>
        <rect y="${photoH}" width="${w}" height="${h - photoH}" fill="${infoFill}" rx="3"/>
        <line x1="4" y1="${photoH + 8}" x2="${w * 0.6}" y2="${photoH + 8}" stroke="#aaa" stroke-width="1.5"/>
        <line x1="4" y1="${photoH + 14}" x2="${w * 0.4}" y2="${photoH + 14}" stroke="#666" stroke-width="1"/>
      </svg>`;
    }
    case 'photo_overlay':
      return `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
        <rect width="${w}" height="${h}" fill="${photoFill}" rx="3"/>
        <defs><linearGradient id="og" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="transparent"/>
          <stop offset="100%" stop-color="rgba(0,0,0,0.7)"/>
        </linearGradient></defs>
        <rect y="${h * 0.5}" width="${w}" height="${h * 0.5}" fill="url(#og)" rx="3"/>
        <line x1="6" y1="${h - 12}" x2="${w * 0.55}" y2="${h - 12}" stroke="#fff" stroke-width="1.5" opacity="0.8"/>
        <line x1="6" y1="${h - 6}" x2="${w * 0.35}" y2="${h - 6}" stroke="#fff" stroke-width="1" opacity="0.5"/>
      </svg>`;
    case 'side_panel': {
      const photoW = Math.round(w * 0.65);
      return `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
        <rect width="${photoW}" height="${h}" fill="${photoFill}" rx="3"/>
        <rect x="${photoW}" width="${w - photoW}" height="${h}" fill="${infoFill}" rx="3"/>
        <line x1="${photoW + 3}" y1="10" x2="${w - 4}" y2="10" stroke="#aaa" stroke-width="1.5"/>
        <line x1="${photoW + 3}" y1="18" x2="${w - 6}" y2="18" stroke="#666" stroke-width="1"/>
        <line x1="${photoW + 3}" y1="24" x2="${w - 8}" y2="24" stroke="#666" stroke-width="1"/>
      </svg>`;
    }
    default:
      return `<svg width="${w}" height="${h}"><rect width="${w}" height="${h}" fill="${stroke}" rx="3"/></svg>`;
  }
};

const dateFormatOptions = [
  { label: 'Mon, Jan 02 (Default)', value: '' },
  { label: 'Monday, January 02, 2006', value: 'Monday, January 02, 2006' },
  { label: 'DD/MM/YYYY', value: '02/01/2006' },
  { label: 'MM/DD/YYYY', value: '01/02/2006' },
  { label: 'DD.MM.YYYY', value: '02.01.2006' },
  { label: 'DD-MM-YYYY', value: '02-01-2006' },
  { label: 'YYYY-MM-DD', value: '2006-01-02' },
  { label: 'YYYY.MM.DD', value: '2006.01.02' },
];

const aiModelOptionsForProvider = (provider: string | undefined) => {
  if (provider === 'openai') {
    return [
      { title: 'GPT Image 1.5', value: 'gpt-image-1.5' },
      { title: 'GPT Image 1', value: 'gpt-image-1' },
      { title: 'GPT Image 1 Mini', value: 'gpt-image-1-mini' },
    ];
  } else if (provider === 'google') {
    return [
      {
        title: 'Gemini 3.1 Flash Image',
        value: 'gemini-3.1-flash-image-preview',
      },
      { title: 'Gemini 3 Pro Image', value: 'gemini-3-pro-image-preview' },
      { title: 'Gemini 2.5 Flash Image', value: 'gemini-2.5-flash-image' },
    ];
  }
  return [];
};

const getDeviceName = (id: number) => {
  const dev = availableDevices.value.find((d) => d.id === id);
  return dev ? dev.name : `Device ${id}`;
};

watch(sourceTab, (val) => {
  // Drive the top gallery for gallery-capable sources.
  if (sourceHasGallery.value) {
    galleryStore.setSource(
      val as 'gallery' | 'immich' | 'synology_photos' | 'google_photos'
    );
  }
  // Lazy-load per-source data the first time its tab is opened.
  if (val === 'url') {
    loadURLSources();
  } else if (val === 'google_photos') {
    loadCalendars();
  }
  // Persist the active source tab in the URL hash so a refresh restores it.
  if (val) window.location.hash = 'gtab=' + val;
});

// Devices State
const availableDevices = ref<Device[]>([]);
const deviceListLoading = ref(false);

// Load calendars when the edit dialog opens (if not yet loaded)
watch(showEditDeviceDialog, (open) => {
  if (open && !calendarLoaded.value) {
    loadCalendars();
  }
});

// Reset AI model when provider changes
watch(
  () => editingDevice.ai_provider,
  (newProvider, oldProvider) => {
    if (newProvider !== oldProvider && oldProvider !== undefined) {
      // Set default model for the new provider
      if (newProvider === 'openai') {
        editingDevice.ai_model = 'gpt-image-1.5';
      } else if (newProvider === 'google') {
        editingDevice.ai_model = 'gemini-3.1-flash-image-preview';
      } else {
        editingDevice.ai_model = '';
      }
    }
  }
);

const isAddingDevice = ref(false);

const openAddDeviceDialog = () => {
  Object.assign(editingDevice, {
    id: undefined,
    name: '',
    host: '',
    width: 0,
    height: 0,
    orientation: '',
    enable_collage: false,
    show_date: false,
    show_photo_date: false,
    show_weather: false,
    weather_lat: null,
    weather_lon: null,
    ai_provider: '',
    ai_model: '',
    ai_prompt: '',
    layout: 'photo_overlay',
    display_mode: 'cover',
    background_color: 'white',
    show_calendar: false,
    calendar_id: '',
    date_format: '',
    source: '',
  });
  Object.assign(deviceConfig, {
    auto_rotate: false,
    rotate_cron: ['0 */12 *'],
    rotation_mode: 'storage',
    image_url: '',
    save_downloaded_images: true,
    sleep_schedule_enabled: false,
    sleep_start_time: '23:00',
    sleep_end_time: '07:00',
    display_orientation: 'landscape',
    deep_sleep_enabled: true,
  });
  isAddingDevice.value = true;
  deviceDialogTab.value = 'general';
  showEditDeviceDialog.value = true;
};

const deviceURL = (device: Device) => {
  return /^https?:\/\//.test(device.host)
    ? device.host
    : `http://${device.host}`;
};

const editDevice = async (device: Device) => {
  Object.assign(editingDevice, device);
  if (!editingDevice.background_color) {
    editingDevice.background_color = 'white';
  }
  // Initialize display_orientation from device's orientation
  deviceConfig.display_orientation = device.orientation || 'landscape';
  isAddingDevice.value = false;
  deviceDialogTab.value = 'general';
  showEditDeviceDialog.value = true;
  deviceImmichAlbumIds.value = [];
  deviceSynologyAlbumIds.value = [];
  deviceUnsplashAlbumIds.value = [];
  devicePexelsAlbumIds.value = [];
  // Load device remote config
  await loadDeviceConfig(device.id);
  // Load Immich album options + this device's bindings (best-effort)
  if (immichConnected.value) {
    try {
      await immichStore.fetchSyncedAlbums();
      const res = await api.get(`/devices/${device.id}/albums?source=immich`);
      deviceImmichAlbumIds.value = res.data?.album_ids || [];
    } catch (e) {
      // Non-fatal: leave bindings empty
    }
  }
  // Load Synology album options + this device's bindings (best-effort)
  if (form.synology_sid) {
    try {
      await synologyStore.fetchSyncedAlbums();
      const res = await api.get(
        `/devices/${device.id}/albums?source=synology_photos`
      );
      deviceSynologyAlbumIds.value = res.data?.album_ids || [];
    } catch (e) {
      // Non-fatal: leave bindings empty
    }
  }
  // Load topic-source album options + this device's bindings (best-effort).
  const topicBindingRefs: Record<string, typeof deviceUnsplashAlbumIds> = {
    unsplash: deviceUnsplashAlbumIds,
    pexels: devicePexelsAlbumIds,
  };
  await Promise.all(
    Object.keys(topicStores).map(async (source) => {
      try {
        await topicStores[source].fetchSyncedAlbums();
        const res = await api.get(
          `/devices/${device.id}/albums?source=${source}`
        );
        topicBindingRefs[source].value = res.data?.album_ids || [];
      } catch (e) {
        // Non-fatal: leave bindings empty
      }
    })
  );
};

const saveDevice = async () => {
  if (!editingDevice.host) {
    showMessage('Host is required', true);
    return;
  }
  if (editingDevice.show_weather) {
    if (
      editingDevice.weather_lat === null ||
      editingDevice.weather_lat === undefined ||
      isNaN(editingDevice.weather_lat) ||
      editingDevice.weather_lon === null ||
      editingDevice.weather_lon === undefined ||
      isNaN(editingDevice.weather_lon)
    ) {
      showMessage('Latitude and Longitude are required for weather', true);
      return;
    }
  }
  savingDeviceConfig.value = true;
  try {
    if (isAddingDevice.value) {
      const newDevice = await addDevice({
        host: editingDevice.host!,
        enable_collage: editingDevice.enable_collage!,
        show_date: editingDevice.show_date!,
        show_photo_date: editingDevice.show_photo_date || false,
        show_weather: editingDevice.show_weather!,
        weather_lat: editingDevice.weather_lat || 0,
        weather_lon: editingDevice.weather_lon || 0,
        layout: editingDevice.layout || 'photo_overlay',
        display_mode: editingDevice.display_mode || 'cover',
        show_calendar: editingDevice.show_calendar || false,
        calendar_id: editingDevice.calendar_id || '',
        date_format: editingDevice.date_format || '',
      });
      await loadDevices();
      showMessage('Device added. Fetched settings from device.');
      // Re-open in edit mode with fetched config
      const added = availableDevices.value.find(
        (d: Device) => d.id === newDevice.id
      );
      if (added) {
        savingDeviceConfig.value = false;
        await editDevice(added);
        return;
      }
    } else {
      if (!editingDevice.id) return;
      // Per-device source: stored on the device record when this server is the
      // URL image source; empty otherwise.
      const deviceSource =
        useThisServer.value && deviceConfig.rotation_mode === 'url'
          ? selectedSource.value
          : '';
      // Save server-side device fields
      await updateDevice(
        editingDevice.id,
        editingDevice.name!,
        editingDevice.host!,
        deviceConfig.display_orientation || editingDevice.orientation!,
        editingDevice.enable_collage!,
        editingDevice.show_date!,
        editingDevice.show_photo_date || false,
        editingDevice.show_weather!,
        editingDevice.weather_lat || 0,
        editingDevice.weather_lon || 0,
        editingDevice.ai_provider || '',
        editingDevice.ai_model || '',
        editingDevice.ai_prompt || '',
        editingDevice.layout || 'photo_overlay',
        editingDevice.display_mode || 'cover',
        editingDevice.show_calendar || false,
        editingDevice.calendar_id || '',
        editingDevice.date_format || '',
        deviceSource,
        editingDevice.background_color || ''
      );

      // Save device remote config (config + processing + palette)
      const [startH, startM] = deviceConfig.sleep_start_time
        .split(':')
        .map(Number);
      const [endH, endM] = deviceConfig.sleep_end_time.split(':').map(Number);

      // Convert UTC offset to POSIX timezone format (sign is inverted)
      const offsetVal = deviceConfig.timezone_offset || 0;
      let timezone = 'UTC0';
      if (offsetVal !== 0) {
        const absOff = Math.abs(offsetVal);
        const h = Math.floor(absOff);
        const m = Math.round((absOff - h) * 60);
        const sign = offsetVal > 0 ? '-' : '+';
        timezone =
          m === 0
            ? `UTC${sign}${h}`
            : `UTC${sign}${h}:${String(m).padStart(2, '0')}`;
      }

      // Compute image URL: use server URL if "use this server" is checked.
      // getImageUrl() targets the direct add-on port, so the URL works when
      // the ESP32 reaches the server straight (ingress port 8123 cannot serve
      // /image/*).
      let imageUrl = deviceConfig.image_url;
      if (useThisServer.value && deviceConfig.rotation_mode === 'url') {
        imageUrl = getImageUrl();
      }

      // Reconcile the schedule with firmware capability.
      // - New firmware: send rotate_cron, plus a derived rotate_interval when
      //   the schedule is a simple every-day interval (harmless; ignored in
      //   favour of rotate_cron).
      // - Old firmware: it only understands rotate_interval and silently
      //   ignores rotate_cron. Send interval-only so we don't store a phantom
      //   cron the device isn't running; refuse a schedule that can't be
      //   reduced to an interval instead of losing the edit.
      // The device rejects the whole config request on any invalid rule or a
      // set over the 7-rule budget — refuse to save one it won't accept.
      const cronRules: string[] = deviceConfig.rotate_cron || [];
      if (
        cronRules.length < 1 ||
        cronRules.length > 7 ||
        !cronRules.every((r: string) => isValidCron(r))
      ) {
        showMessage(
          'The rotation schedule is invalid (a rule is malformed or there are ' +
            'more than 7 rules). Fix it before saving.',
          true
        );
        return;
      }
      const legacyInterval = cronToInterval(deviceConfig.rotate_cron);
      if (!deviceSupportsCron.value && legacyInterval === null) {
        showMessage(
          "This device's firmware only supports a simple repeating interval. " +
            'Update the firmware to use day-of-week or specific-time schedules.',
          true
        );
        return;
      }
      const rotateFields = deviceSupportsCron.value
        ? {
            rotate_cron: deviceConfig.rotate_cron,
            ...(legacyInterval !== null
              ? { rotate_interval: legacyInterval }
              : {}),
          }
        : { rotate_interval: legacyInterval };

      // Quiet hours only exist on pre-cron firmware; cron firmware bounds the
      // active hours in the rules instead, so don't send these to it.
      const sleepFields = deviceSupportsCron.value
        ? {}
        : {
            sleep_schedule_enabled: deviceConfig.sleep_schedule_enabled,
            sleep_schedule_start: startH * 60 + startM,
            sleep_schedule_end: endH * 60 + endM,
          };

      const result = await updateDeviceConfig(editingDevice.id, {
        config: {
          device_name: editingDevice.name,
          auto_rotate: deviceConfig.auto_rotate,
          ...rotateFields,
          rotation_mode: deviceConfig.rotation_mode,
          image_url: imageUrl,
          save_downloaded_images: deviceConfig.save_downloaded_images,
          ...sleepFields,
          display_orientation: deviceConfig.display_orientation,
          display_rotation_deg: deviceConfig.display_rotation_deg,
          timezone: timezone,
          ntp_server: deviceConfig.ntp_server,
          deep_sleep_enabled: deviceConfig.deep_sleep_enabled,
          ha_url: deviceConfig.ha_url,
          openai_api_key: deviceConfig.openai_api_key,
          google_api_key: deviceConfig.google_api_key,
        },
        processing_settings: { ...deviceProcessing },
        // Grayscale (GC16) panels are calibrated by two luminance endpoints;
        // color panels by the 6 named colors. Both ride the same color_palette
        // field, which the server stores and the device pulls via X-Config-Payload.
        color_palette: isGrayscale.value
          ? {
              black_y: grayscaleCal.black_y,
              white_y: grayscaleCal.white_y,
              gamma: grayscaleCal.gamma,
            }
          : { ...devicePalette },
      });

      // Persist per-device Immich album bindings (best-effort).
      if (immichConnected.value) {
        try {
          await api.put(`/devices/${editingDevice.id}/albums`, {
            source: 'immich',
            album_ids: deviceImmichAlbumIds.value,
          });
        } catch (e: any) {
          showMessage(
            'Device saved, but failed to save Immich album bindings: ' +
              (e.response?.data?.error || e.message),
            true
          );
        }
      }

      // Persist per-device Synology album bindings (best-effort).
      if (form.synology_sid) {
        try {
          await api.put(`/devices/${editingDevice.id}/albums`, {
            source: 'synology_photos',
            album_ids: deviceSynologyAlbumIds.value,
          });
        } catch (e: any) {
          showMessage(
            'Device saved, but failed to save Synology album bindings: ' +
              (e.response?.data?.error || e.message),
            true
          );
        }
      }

      // Persist per-device topic-source album bindings (best-effort).
      const topicBindings: Record<string, number[]> = {
        unsplash: deviceUnsplashAlbumIds.value,
        pexels: devicePexelsAlbumIds.value,
      };
      for (const [source, album_ids] of Object.entries(topicBindings)) {
        try {
          await api.put(`/devices/${editingDevice.id}/albums`, {
            source,
            album_ids,
          });
        } catch (e: any) {
          showMessage(
            `Device saved, but failed to save ${source} album bindings: ` +
              (e.response?.data?.error || e.message),
            true
          );
        }
      }

      if (result.push_result === 'synced') {
        showMessage('Device saved and config pushed to device.');
      } else {
        showMessage(
          'Device saved. Device is offline — config will sync on next image fetch.'
        );
      }
    }
    await loadDevices();
    showEditDeviceDialog.value = false;
  } catch (e: any) {
    showMessage(
      'Failed to save device: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    savingDeviceConfig.value = false;
  }
};

const loadDevices = async () => {
  deviceListLoading.value = true;
  try {
    availableDevices.value = await listDevices();
  } catch (e) {
    console.error('Failed to list devices', e);
  } finally {
    deviceListLoading.value = false;
  }
};

const removeDevice = async (id: number) => {
  const response = await confirmDialog.value.open(
    'Remove Device',
    'Are you sure you want to remove this device?'
  );

  if (!response) return;

  try {
    await deleteDevice(id);
    await loadDevices();
    showMessage('Device removed');
  } catch (e) {
    showMessage('Failed to remove device', true);
  }
};

const SOURCE_TABS = [
  'gallery',
  'immich',
  'synology_photos',
  'google_photos',
  'unsplash',
  'pexels',
  'url',
  'ai_generation',
];
// Restore the active source tab from the URL hash (#gtab=<tab>) on load.
const restoreGalleryTabFromHash = () => {
  const m = window.location.hash.match(/gtab=([\w-]+)/);
  if (m && SOURCE_TABS.includes(m[1])) {
    sourceTab.value = m[1];
  }
};

const { showMessage } = useSnackbar();

const form = reactive({
  Orientation: 'landscape',
  DisplayWidth: 800,
  DisplayHeight: 480,
  CollageMode: false,
  show_date: true,
  show_weather: true,
  weather_lat: '',
  weather_lon: '',
  google_connected: 'false',
  google_calendar_connected: 'false',
  google_client_id: '',
  google_client_secret: '',
  synology_sid: '',
  synology_url: '',
  synology_account: '',
  synology_password: '',
  synology_skip_cert: false,
  synology_otp_code: '',
  synology_album_id: '',
  synology_auto_sync_enabled: false,
  synology_auto_sync_interval_minutes: 60,
  albums: [] as any[],
  immich_url: '',
  immich_api_key: '',
  immich_source_mode: 'album',
  immich_memory_mode: 'all',
  immich_album_id: '',
  immich_auto_sync_enabled: false,
  immich_auto_sync_interval_minutes: 60,
  immich_albums: [] as any[],
  telegram_bot_token: '',
  telegram_push_enabled: false,
  telegram_target_device_id: [] as number[],
  openai_api_key: '',
  google_api_key: '',
  // Topic sources.
  unsplash_api_key: '',
  pexels_api_key: '',
  unsplash_auto_sync_enabled: false,
  unsplash_auto_sync_interval_minutes: 60,
  unsplash_randomize_results: false,
  pexels_auto_sync_enabled: false,
  pexels_auto_sync_interval_minutes: 60,
  pexels_randomize_results: false,
  // Device-facing base URL for image requests (e.g. http://homeassistant.local:9608).
  // Empty = derive from the browser's location.
  device_image_base_url: '',
  device_host: '', // Keep for backward compatibility/display? Or remove. Remove from form, keep in store maybe?
});

// Synced (persisted) Immich albums available for per-device binding.
// Items use the internal album row id (number) as the value.
const deviceImmichAlbumOptions = computed(() => {
  return (immichStore.syncedAlbums || [])
    .filter((a: any) => a.sync_enabled)
    .map((a: any) => ({ id: a.id, name: a.name }));
});

// Synced (persisted) Synology albums available for per-device binding.
// Items use the internal album row id (number) as the value.
const deviceSynologyAlbumOptions = computed(() => {
  return (synologyStore.syncedAlbums || [])
    .filter((a: any) => a.sync_enabled)
    .map((a: any) => ({ id: a.id, name: a.name }));
});

// Synced topic albums available for per-device binding (topic sources).
const topicAlbumOptions = (store: any) =>
  (store.syncedAlbums || [])
    .filter((a: any) => a.sync_enabled)
    .map((a: any) => ({ id: a.id, name: a.name }));
const deviceUnsplashAlbumOptions = computed(() =>
  topicAlbumOptions(unsplashStore)
);
const devicePexelsAlbumOptions = computed(() => topicAlbumOptions(pexelsStore));

// Album search + checked-first sorting now live in <AlbumPicker>.

// Derive the Synology checkbox selection from the persisted synced albums.
// External ids are matched against String(album.id) of live Synology albums.
const applySynologySyncedAlbumState = () => {
  suppressSyncAutoSave = true;
  synologySyncAlbumIds.value = (synologyStore.syncedAlbums || [])
    .filter((a: any) => a.sync_enabled)
    .map((a: any) => a.external_id);
  nextTick(() => {
    suppressSyncAutoSave = false;
  });
};

// Virtual sentinel external_ids used by the backend for non-album sources.
const IMMICH_FAVORITES_ID = '__favorites__';
const IMMICH_ALL_ID = '__all__';
const IMMICH_MEMORIES_ID = '__memories__';

// Derive the checkbox selection state from the persisted synced albums.
const applySyncedAlbumState = () => {
  suppressSyncAutoSave = true;
  const synced = (immichStore.syncedAlbums || []).filter(
    (a: any) => a.sync_enabled
  );
  syncFavorites.value = synced.some(
    (a: any) => a.external_id === IMMICH_FAVORITES_ID
  );
  syncAll.value = synced.some((a: any) => a.external_id === IMMICH_ALL_ID);
  syncMemories.value = synced.some(
    (a: any) => a.external_id === IMMICH_MEMORIES_ID
  );
  syncAlbumIds.value = synced
    .filter(
      (a: any) =>
        a.kind === 'album' &&
        a.external_id !== IMMICH_FAVORITES_ID &&
        a.external_id !== IMMICH_ALL_ID &&
        a.external_id !== IMMICH_MEMORIES_ID
    )
    .map((a: any) => a.external_id);
  nextTick(() => {
    suppressSyncAutoSave = false;
  });
};

// Persist the album selection (debounced) without importing — the user syncs
// manually or is prompted on leaving. Suppressed during programmatic apply.
watch(
  [syncAlbumIds, syncFavorites, syncAll, syncMemories],
  () => {
    if (suppressSyncAutoSave) return;
    immichSyncDirty.value = true;
    if (immichSyncSaveTimer != null) clearTimeout(immichSyncSaveTimer);
    immichSyncSaveTimer = window.setTimeout(() => saveSyncSelection(), 600);
  },
  { deep: true }
);
watch(
  synologySyncAlbumIds,
  () => {
    if (suppressSyncAutoSave) return;
    synologySyncDirty.value = true;
    if (synologySyncSaveTimer != null) clearTimeout(synologySyncSaveTimer);
    synologySyncSaveTimer = window.setTimeout(
      () => saveSynologySyncSelection(),
      600
    );
  },
  { deep: true }
);

// When the user leaves the Immich/Synology config (sub-tab or the whole Data
// Sources tab) with unsynced selection changes, offer to sync now.
const maybePromptSync = async (source: 'immich' | 'synology_photos') => {
  const dirty = source === 'immich' ? immichSyncDirty : synologySyncDirty;
  if (!dirty.value) return;
  dirty.value = false; // ask once
  const ok = await confirmDialog.value.open(
    'You changed which albums to sync. Sync now to apply the changes?'
  );
  if (!ok) return;
  if (source === 'immich') await syncImmich();
  else await syncSynology();
};
// When the user leaves a topic source (Unsplash/Pexels) after changing topics,
// offer to sync now to import photos for them.
const maybePromptTopicSync = async (source: string) => {
  if (!topicSyncDirty[source]) return;
  topicSyncDirty[source] = false; // ask once
  const ok = await confirmDialog.value.open(
    'You changed topics. Sync now to import photos for them?'
  );
  if (!ok) return;
  await syncTopicSource(source);
};
watch(sourceTab, (_newVal, oldVal) => {
  if (oldVal === 'immich') maybePromptSync('immich');
  if (oldVal === 'synology_photos') maybePromptSync('synology_photos');
  if (oldVal === 'unsplash' || oldVal === 'pexels')
    maybePromptTopicSync(oldVal);
});
watch(activeMainTab, (_newVal, oldVal) => {
  if (oldVal === 'datasources') {
    maybePromptSync('immich');
    maybePromptSync('synology_photos');
    maybePromptTopicSync('unsplash');
    maybePromptTopicSync('pexels');
  }
});

const immichMemoryModeOptions = [
  { title: 'All years', value: 'all' },
  { title: 'Most recent year only', value: 'latest' },
];

const autoSyncIntervalOptions = [
  { title: 'Every 15 minutes', value: 15 },
  { title: 'Every 30 minutes', value: 30 },
  { title: 'Every 1 hour', value: 60 },
  { title: 'Every 3 hours', value: 180 },
  { title: 'Every 6 hours', value: 360 },
  { title: 'Every 12 hours', value: 720 },
  { title: 'Every 24 hours', value: 1440 },
];

// Delete all photos for a given image source. Also disables that source's
// album sync on the backend, so refresh the sync-selection UI afterwards.
const deletingAllPhotos = ref(false);
const deleteAllPhotosForSource = async (
  source: 'immich' | 'synology_photos' | 'google_photos' | 'gallery'
) => {
  if (
    !(await confirmDialog.value.open(
      'Delete all photos for this source? This also turns off its album sync.'
    ))
  )
    return;

  deletingAllPhotos.value = true;
  try {
    await api.delete('/gallery/photos?source=' + source);

    if (source === 'immich') {
      await immichStore.fetchSyncedAlbums();
      try {
        await immichStore.fetchAlbums();
        form.immich_albums = immichStore.albums;
      } catch (e) {
        // Non-fatal: keep whatever albums we already have.
      }
      applySyncedAlbumState();
      await immichStore.fetchCount();
    } else if (source === 'synology_photos') {
      await synologyStore.fetchSyncedAlbums();
      try {
        await synologyStore.fetchAlbums();
        form.albums = synologyStore.albums;
      } catch (e) {
        // Non-fatal.
      }
      applySynologySyncedAlbumState();
      await synologyStore.fetchCount();
    }

    showMessage('All photos deleted.');

    // Refresh the gallery view (photos + album chips).
    galleryStore.triggerRefresh();
  } catch (e: any) {
    showMessage(
      'Failed to delete photos: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    deletingAllPhotos.value = false;
  }
};

onMounted(async () => {
  restoreGalleryTabFromHash();
  await store.fetchSettings();
  Object.assign(form, {
    Orientation: store.settings.orientation || 'landscape',
    DisplayWidth: parseInt(store.settings.display_width || '800'),
    DisplayHeight: parseInt(store.settings.display_height || '480'),
    CollageMode: store.settings.collage_mode === 'true',
    show_date: store.settings.show_date !== 'false',
    show_weather: store.settings.show_weather !== 'false',
    google_client_id: store.settings.google_client_id || '',
    google_client_secret: store.settings.google_client_secret || '',
    google_connected: store.settings.google_connected || 'false',
    google_calendar_connected:
      store.settings.google_calendar_connected || 'false',
    telegram_bot_token: store.settings.telegram_bot_token || '',
    telegram_push_enabled: store.settings.telegram_push_enabled === 'true',
    telegram_target_device_id: store.settings.telegram_target_device_id
      ? store.settings.telegram_target_device_id
          .split(',')
          .filter((id) => id)
          .map((id) => parseInt(id))
      : [],
    weather_lat: store.settings.weather_lat || '',
    weather_lon: store.settings.weather_lon || '',
    synology_url: store.settings.synology_url || '',
    synology_account: store.settings.synology_account || '',
    synology_password: store.settings.synology_password || '',
    synology_skip_cert: store.settings.synology_skip_cert === 'true',
    synology_album_id: store.settings.synology_album_id
      ? parseInt(store.settings.synology_album_id)
      : '',
    synology_auto_sync_enabled:
      store.settings.synology_auto_sync_enabled === 'true',
    synology_auto_sync_interval_minutes: parseInt(
      store.settings.synology_auto_sync_interval_minutes || '60'
    ),
    synology_sid: store.settings.synology_sid || '',
    immich_url: store.settings.immich_url || '',
    immich_api_key: store.settings.immich_api_key || '',
    immich_source_mode: store.settings.immich_source_mode || 'album',
    immich_memory_mode: store.settings.immich_memory_mode || 'all',
    immich_album_id: store.settings.immich_album_id || '',
    immich_auto_sync_enabled:
      store.settings.immich_auto_sync_enabled === 'true',
    immich_auto_sync_interval_minutes: parseInt(
      store.settings.immich_auto_sync_interval_minutes || '60'
    ),
    device_image_base_url: store.settings.device_image_base_url || '',
    openai_api_key: store.settings.openai_api_key || '',
    google_api_key: store.settings.google_api_key || '',
    unsplash_api_key: store.settings.unsplash_api_key || '',
    pexels_api_key: store.settings.pexels_api_key || '',
    unsplash_auto_sync_enabled:
      store.settings.unsplash_auto_sync_enabled === 'true',
    unsplash_auto_sync_interval_minutes: parseInt(
      store.settings.unsplash_auto_sync_interval || '60'
    ),
    unsplash_randomize_results:
      store.settings.unsplash_randomize_results === 'true',
    pexels_auto_sync_enabled:
      store.settings.pexels_auto_sync_enabled === 'true',
    pexels_auto_sync_interval_minutes: parseInt(
      store.settings.pexels_auto_sync_interval || '60'
    ),
    pexels_randomize_results:
      store.settings.pexels_randomize_results === 'true',
  });

  // Load cached albums if available
  if (store.settings.synology_albums_cache) {
    try {
      form.albums = JSON.parse(store.settings.synology_albums_cache);
    } catch (e) {
      console.error('Failed to parse cached albums', e);
    }
  }

  // Run independent fetches in parallel
  const parallelFetches: Promise<void>[] = [loadDevices()];

  // Fetch Synology photo count + albums if connected
  if (form.synology_sid) {
    parallelFetches.push(
      (async () => {
        await synologyStore.fetchCount();
        try {
          await synologyStore.fetchAlbums();
          form.albums = synologyStore.albums;
        } catch (e) {
          // Non-fatal: use cached albums until user refreshes
        }
        try {
          await synologyStore.fetchSyncedAlbums();
          applySynologySyncedAlbumState();
        } catch (e) {
          // Non-fatal: checkboxes start unchecked until user refreshes
        }
      })()
    );
  }

  // Fetch Immich photo count and albums if connected
  if (form.immich_url && form.immich_api_key) {
    immichConnected.value = true;
    parallelFetches.push(
      (async () => {
        await immichStore.fetchCount();
        try {
          await immichStore.fetchAlbums();
          form.immich_albums = immichStore.albums;
        } catch (e) {
          // Non-fatal: album names will be shown as UUIDs until user clicks Refresh
        }
        try {
          await immichStore.fetchSyncedAlbums();
          applySyncedAlbumState();
        } catch (e) {
          // Non-fatal: checkboxes start unchecked until user refreshes
        }
      })()
    );
  }

  // Topic-source connection state (key present => connected).
  unsplashConnected.value = !!store.settings.unsplash_api_key;
  pexelsConnected.value = !!store.settings.pexels_api_key;

  // Topic sources: load counts + synced topics (best-effort, non-blocking).
  for (const source of Object.keys(topicStores)) {
    parallelFetches.push(
      (async () => {
        try {
          await topicStores[source].fetchCount();
          await topicStores[source].fetchSyncedAlbums();
        } catch (e) {
          // Non-fatal
        }
      })()
    );
  }

  await Promise.all(parallelFetches);

  // Parse URL params for deep linking (e.g. from OAuth callback)
  const params = new URLSearchParams(window.location.search);
  const tab = params.get('tab');
  const source = params.get('source');

  if (tab && tab !== 'datasources') {
    activeMainTab.value = tab;
  }
  if (source) {
    sourceTab.value = source === 'google' ? 'google_photos' : source;
  }

  // Clean up URL if params were present
  if (tab || source) {
    window.history.replaceState({}, '', '/');
  }
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
    telegram_target_device_id: Array.isArray(form.telegram_target_device_id)
      ? form.telegram_target_device_id.join(',')
      : form.telegram_target_device_id,
    weather_lat: form.weather_lat,
    weather_lon: form.weather_lon,
    synology_url: form.synology_url,
    synology_account: form.synology_account,
    synology_password: form.synology_password,
    synology_skip_cert: String(form.synology_skip_cert),
    synology_album_id: String(form.synology_album_id),
    synology_auto_sync_enabled: String(form.synology_auto_sync_enabled),
    synology_auto_sync_interval_minutes: String(
      form.synology_auto_sync_interval_minutes
    ),
    immich_url: form.immich_url,
    immich_api_key: form.immich_api_key,
    immich_source_mode: form.immich_source_mode,
    immich_memory_mode: form.immich_memory_mode,
    immich_album_id: form.immich_album_id,
    immich_auto_sync_enabled: String(form.immich_auto_sync_enabled),
    immich_auto_sync_interval_minutes: String(
      form.immich_auto_sync_interval_minutes
    ),
    openai_api_key: form.openai_api_key,
    google_api_key: form.google_api_key,
    unsplash_api_key: form.unsplash_api_key,
    pexels_api_key: form.pexels_api_key,
    unsplash_auto_sync_enabled: String(form.unsplash_auto_sync_enabled),
    unsplash_auto_sync_interval: String(
      form.unsplash_auto_sync_interval_minutes
    ),
    unsplash_randomize_results: String(form.unsplash_randomize_results),
    pexels_auto_sync_enabled: String(form.pexels_auto_sync_enabled),
    pexels_auto_sync_interval: String(form.pexels_auto_sync_interval_minutes),
    pexels_randomize_results: String(form.pexels_randomize_results),
    device_image_base_url: form.device_image_base_url,
  });
};

const save = async () => {
  try {
    await saveSettingsInternal();
    showMessage('Settings saved successfully');
  } catch (err: any) {
    showMessage(err.message || 'Failed to save settings', true);
  }
};

const connectGoogle = async () => {
  try {
    await saveSettingsInternal();
    const res = await api.get('/auth/google/login');
    window.location.href = res.data.url;
  } catch (e) {
    showMessage('Failed to connect: ' + e, true);
  }
};

const logoutGoogle = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Google Photos?'
    ))
  )
    return;
  try {
    await api.post('/auth/google/logout');
    form.google_connected = 'false';
    showMessage('Disconnected Google Photos.');
    await store.fetchSettings();
  } catch (e) {
    showMessage('Error disconnecting: ' + e, true);
  }
};

const connectGoogleCalendar = async () => {
  try {
    await saveSettingsInternal();
    const res = await googleCalendarLogin();
    window.location.href = res.url;
  } catch (e) {
    showMessage('Failed to connect Google Calendar: ' + e, true);
  }
};

const logoutGoogleCalendar = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Google Calendar?'
    ))
  )
    return;
  try {
    await googleCalendarLogout();
    form.google_calendar_connected = 'false';
    calendarConnected.value = false;
    calendars.value = [];
    showMessage('Disconnected Google Calendar.');
    await store.fetchSettings();
  } catch (e) {
    showMessage('Error disconnecting: ' + e, true);
  }
};

const testSynology = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.testConnection(form.synology_otp_code);
    showMessage('Connection Successful!');
    form.synology_otp_code = '';
    // Store updates settings internally, but we need to update form
    form.synology_sid = store.settings.synology_sid;
    // Auto-load the album list so the user can pick albums right away.
    await loadAlbums();
  } catch (e: any) {
    const err = e.response?.data?.error || 'Unknown error';
    if (err.includes('code: 403')) {
      showMessage(
        '2FA Required! Please enter OTP code and Test Connection again.',
        true
      );
    } else {
      showMessage('Connection Failed: ' + err, true);
    }
  }
};

const logoutSynology = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Synology?'
    ))
  )
    return;
  try {
    await synologyStore.logout();
    form.synology_sid = '';
    form.albums = [];
    synologySyncAlbumIds.value = [];
    showMessage('Logged out from Synology.');
  } catch (e) {
    showMessage('Error logging out: ' + e, true);
  }
};

const loadAlbums = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.fetchAlbums();
    form.albums = synologyStore.albums;
    try {
      await synologyStore.fetchSyncedAlbums();
      applySynologySyncedAlbumState();
    } catch (e) {
      // Non-fatal
    }
    showMessage('Albums loaded!');
  } catch (e: any) {
    if (
      e.message === 'Session expired' ||
      (e.response && e.response.status === 401)
    ) {
      showMessage(
        'Session expired or Unauthorized. Please check login/settings.',
        true
      );
    } else {
      showMessage(
        'Failed to load albums: ' + (e.response?.data?.error || e.message),
        true
      );
    }
  }
};

const saveSynologySyncSelection = async () => {
  savingSynologySyncSelection.value = true;
  try {
    await synologyStore.saveSyncAlbums(synologySyncAlbumIds.value);
    applySynologySyncedAlbumState();
    // Selection persisted only; no import here (manual Sync / prompt triggers it).
  } catch (e: any) {
    showMessage(
      'Failed to save sync selection: ' +
        (e.response?.data?.error || e.message),
      true
    );
  } finally {
    savingSynologySyncSelection.value = false;
  }
};

const syncSynology = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.sync();
    synologySyncDirty.value = false;
    showMessage('Sync started — importing in the background.');
    galleryStore.triggerRefresh();
    // Refresh the gallery preview above so the freshly synced photos show.
    // Switching the source triggers a fetch; if it's already on synology the
    // watch won't fire, so refetch explicitly.
    if (sourceTab.value === 'synology_photos') {
      await galleryStore.fetchPhotos();
    } else {
      sourceTab.value = 'synology_photos';
    }
  } catch (e: any) {
    if (e.response && e.response.status === 401) {
      showMessage('Session expired. Please reconnect.', true);
    } else {
      showMessage(
        'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
        true
      );
    }
  }
};

// Clear All Photos = delete photos AND turn off album sync (so they don't
// re-import) — same behavior as the per-source Delete All.
const clearSynology = () => deleteAllPhotosForSource('synology_photos');

const testImmich = async () => {
  await saveSettingsInternal();
  try {
    await immichStore.testConnection();
    immichConnected.value = true;
    showMessage('Connection Successful!');
    // Auto-load the album list so the user can pick albums right away.
    await loadImmichAlbums();
  } catch (e: any) {
    showMessage(
      'Connection Failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const disconnectTelegram = async () => {
  if (
    !(await confirmDialog.value.open(
      'Disconnect the Telegram bot? You can reconnect by entering a token again.'
    ))
  )
    return;
  form.telegram_bot_token = '';
  form.telegram_push_enabled = false;
  await saveSettingsInternal();
  showMessage('Telegram bot disconnected.');
};

const disconnectImmich = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Immich?'
    ))
  )
    return;
  form.immich_url = '';
  form.immich_api_key = '';
  form.immich_source_mode = 'album';
  form.immich_album_id = '';
  form.immich_albums = [];
  await saveSettingsInternal();
  immichConnected.value = false;
  immichStore.count = 0;
  immichStore.albums = [];
  showMessage('Disconnected from Immich.');
};

const connectUnsplash = async () => {
  // The test endpoint validates the currently-saved key, so persist first.
  await saveSettingsInternal();
  try {
    await api.post('/unsplash/test');
    unsplashConnected.value = true;
    showMessage('Connection Successful!');
  } catch (e: any) {
    showMessage(
      'Connection Failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const disconnectUnsplash = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Unsplash?'
    ))
  )
    return;
  form.unsplash_api_key = '';
  await saveSettingsInternal();
  unsplashConnected.value = false;
  showMessage('Disconnected from Unsplash.');
};

const connectPexels = async () => {
  // The test endpoint validates the currently-saved key, so persist first.
  await saveSettingsInternal();
  try {
    await api.post('/pexels/test');
    pexelsConnected.value = true;
    showMessage('Connection Successful!');
  } catch (e: any) {
    showMessage(
      'Connection Failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const disconnectPexels = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to disconnect Pexels?'
    ))
  )
    return;
  form.pexels_api_key = '';
  await saveSettingsInternal();
  pexelsConnected.value = false;
  showMessage('Disconnected from Pexels.');
};

const loadImmichAlbums = async () => {
  await saveSettingsInternal();
  try {
    await immichStore.fetchAlbums();
    form.immich_albums = immichStore.albums;
    try {
      await immichStore.fetchSyncedAlbums();
      applySyncedAlbumState();
    } catch (e) {
      // Non-fatal
    }
    showMessage('Albums loaded!');
  } catch (e: any) {
    showMessage(
      'Failed to load albums: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const saveSyncSelection = async () => {
  savingSyncSelection.value = true;
  try {
    // Persist the memory-mode setting alongside the sync selection.
    await saveSettingsInternal();
    await immichStore.saveSyncAlbums({
      album_ids: syncAlbumIds.value,
      favorites: syncFavorites.value,
      all: syncAll.value,
      memories: syncMemories.value,
    });
    applySyncedAlbumState();
    // Selection persisted only; no import here (manual Sync / prompt triggers it).
  } catch (e: any) {
    showMessage(
      'Failed to save sync selection: ' +
        (e.response?.data?.error || e.message),
      true
    );
  } finally {
    savingSyncSelection.value = false;
  }
};

const syncImmich = async () => {
  await saveSettingsInternal();
  try {
    await immichStore.sync();
    immichSyncDirty.value = false;
    showMessage('Sync started — importing in the background.');
    galleryStore.triggerRefresh();
    // Refresh the gallery preview above so the freshly synced photos show.
    // Switching the source triggers a fetch; if it's already on immich the
    // watch won't fire, so refetch explicitly.
    if (sourceTab.value === 'immich') {
      await galleryStore.fetchPhotos();
    } else {
      sourceTab.value = 'immich';
    }
  } catch (e: any) {
    showMessage(
      'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
      true
    );
  }
};

const clearImmich = () => deleteAllPhotosForSource('immich');

// --- Topic sources (Unsplash / Pexels) ---
// Replace the synced topic set, then offer to sync (topics have no photos
// until imported).
const saveTopics = async (source: string, topics: string[]) => {
  const st = topicStores[source];
  try {
    await st.setSyncTopics(topics);
    topicSyncDirty[source] = true;
  } catch (e: any) {
    showMessage(
      'Failed to save topics: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const syncTopicSource = async (source: string) => {
  const st = topicStores[source];
  topicSyncDirty[source] = false;
  await saveSettingsInternal();
  try {
    await st.sync();
    showMessage('Sync started — importing in the background.');
    galleryStore.triggerRefresh();
    if (sourceTab.value === source) {
      await galleryStore.fetchPhotos();
    } else {
      sourceTab.value = source;
    }
  } catch (e: any) {
    showMessage(
      'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
      true
    );
  }
};

const clearTopicSource = async (source: string) => {
  const st = topicStores[source];
  if (
    !(await confirmDialog.value.open(
      'Delete all photos for this source? Your topics stay, but their photos are removed.'
    ))
  )
    return;
  try {
    await st.clear();
    showMessage('All photos deleted.');
    galleryStore.triggerRefresh();
  } catch (e: any) {
    showMessage(
      'Failed to delete photos: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Get image endpoint URL
// Always use direct add-on port for device access (ESP32 devices access directly, not via ingress)
// Base URL derived from the browser's location — what /image resolves to
// when no explicit "Server URL for devices" is set. Shown as the field
// placeholder so the user sees the effective value.
const derivedServerBase = computed(() => {
  const protocol = window.location.protocol;
  const hostname = window.location.hostname;
  // Use configurable port via env var, default to 9607 for production
  const addonPort = import.meta.env.VITE_ADDON_PORT || '9607';
  return `${protocol}//${hostname}:${addonPort}`;
});

const getImageUrl = (source?: string) => {
  // Prefer the admin-configured device-facing base URL. The browser's
  // window.location is unreliable for the URL pushed to frames (Tailscale,
  // ingress, reverse proxies all change the hostname the browser used).
  const origin =
    (form.device_image_base_url || '').trim().replace(/\/+$/, '') ||
    derivedServerBase.value;
  return `${origin}/image${source ? '/' + source : ''}`;
};
</script>

<style scoped>
.color-swatch {
  height: 60px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.12);
}
</style>
