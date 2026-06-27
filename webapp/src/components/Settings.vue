```html
<template>
  <div class="pa-1 pa-sm-4">
    <!-- Gallery Card -->
    <v-card class="mb-6">
      <v-tabs v-model="galleryTab" color="primary">
        <v-tab value="gallery">Gallery</v-tab>
        <v-tab value="immich">Immich</v-tab>
        <v-tab value="google_photos">Google Photos</v-tab>
        <v-tab value="synology_photos">Synology</v-tab>
      </v-tabs>
      <v-card-text>
        <Gallery />
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
          <v-tab value="datasources">Data Sources</v-tab>
          <v-tab value="security">System</v-tab>
        </v-tabs>

        <v-window v-model="activeMainTab" :touch="false">
          <!-- Data Sources Tab -->
          <v-window-item value="datasources">
            <v-tabs
              v-model="activeDataSourceTab"
              color="primary"
              density="compact"
              class="mb-4"
            >
              <v-tab value="gallery">Gallery</v-tab>
              <v-tab value="immich">Immich</v-tab>
              <v-tab value="google">Google</v-tab>
              <v-tab value="synology_photos">Synology</v-tab>
              <v-tab value="url">URL Proxy</v-tab>
              <v-tab value="ai_generation">AI Generation</v-tab>
            </v-tabs>

            <v-window v-model="activeDataSourceTab" :touch="false">
              <!-- URL Proxy -->
              <v-window-item value="url">
                <v-card-text>
                  <v-alert
                    type="info"
                    variant="tonal"
                    class="mb-4"
                    density="compact"
                  >
                    Add external image URLs to be served by the photoframe. You
                    can bind URLs to specific devices or leave them global.
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
                          <span v-else class="text-grey text-caption"
                            >Global</span
                          >
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
              <v-window-item value="google">
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
                      <a
                        href="https://console.cloud.google.com/"
                        target="_blank"
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
                  <h3 class="text-subtitle-1 font-weight-bold mb-3">
                    Calendar
                  </h3>

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
                      Connect a Google account for Calendar integration. This
                      can be a different account than Google Photos.
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
                      Connected to Synology Photos ({{
                        form.synology_account
                      }}
                      @ {{ form.synology_url }})
                      <div
                        v-if="synologyStore.count !== null"
                        class="text-caption mt-1"
                      >
                        {{ synologyStore.count }} photo{{
                          synologyStore.count !== 1 ? 's' : ''
                        }}
                        synced
                      </div>
                    </v-alert>

                    <v-row class="mt-2">
                      <v-col cols="12">
                        <div
                          class="d-flex align-center justify-space-between mb-1"
                        >
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
                          Choose which Synology Photos albums to pull photos
                          from. Saving replaces the entire sync set.
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
                      <v-btn
                        color="error"
                        variant="text"
                        @click="logoutSynology"
                        >Log Out</v-btn
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
                      <div
                        v-if="immichStore.count !== null"
                        class="text-caption mt-1"
                      >
                        {{ immichStore.count }} photo{{
                          immichStore.count !== 1 ? 's' : ''
                        }}
                        synced
                      </div>
                    </v-alert>

                    <v-row class="mt-2">
                      <v-col cols="12">
                        <div
                          class="d-flex align-center justify-space-between mb-1"
                        >
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
                          Choose which Immich albums and collections to pull
                          photos from. Saving replaces the entire sync set.
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
                      <v-btn
                        color="error"
                        variant="text"
                        @click="disconnectImmich"
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

              <!-- Gallery -->
              <v-window-item value="gallery">
                <v-card-text>
                  <v-alert
                    type="info"
                    variant="tonal"
                    class="mb-4"
                    density="compact"
                  >
                    Photos in the gallery live on this server. Add them from the
                    Gallery tab above, or send them to the Telegram bot
                    configured below.
                  </v-alert>

                  <div class="d-flex mb-2">
                    <v-btn
                      color="error"
                      variant="outlined"
                      prepend-icon="mdi-delete"
                      :loading="deletingAllPhotos"
                      @click="deleteAllPhotosForSource('gallery')"
                      >Delete all photos</v-btn
                    >
                  </div>

                  <v-divider class="my-4"></v-divider>

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

                    <v-text-field
                      v-model="form.telegram_bot_token"
                      label="Telegram Bot Token"
                      variant="outlined"
                    ></v-text-field>

                    <v-divider class="my-4"></v-divider>

                    <h3 class="text-subtitle-1 font-weight-bold mb-2">
                      Push to Device
                    </h3>
                    <div class="text-caption text-grey mb-2">
                      When enabled, photos uploaded via the Telegram bot are
                      also pushed to the selected devices immediately. They are
                      added to the gallery either way.
                    </div>

                    <v-checkbox
                      v-model="form.telegram_push_enabled"
                      label="Enable Push to Device"
                      color="primary"
                      hide-details
                      density="compact"
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
                        ></v-select>
                      </div>
                    </v-expand-transition>

                    <v-btn color="primary" class="mt-4" @click="save"
                      >Update Settings</v-btn
                    >
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
                    Generate images using AI (OpenAI or Google Gemini).
                    Configure API keys below, then set the prompt/model
                    per-device in the Edit Device dialog.
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
          </v-window-item>

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

              <div class="d-flex justify-space-between align-center mb-4">
                <h3 class="text-h6">Admin Account</h3>
                <v-btn
                  variant="tonal"
                  size="small"
                  @click="showAccountForm = !showAccountForm"
                >
                  {{ showAccountForm ? 'Cancel' : 'Edit Account' }}
                </v-btn>
              </div>

              <v-expand-transition>
                <v-card v-if="showAccountForm" variant="outlined" class="mb-6">
                  <v-card-text>
                    <v-alert
                      type="info"
                      variant="tonal"
                      class="mb-4"
                      density="compact"
                    >
                      Leave new password fields blank if you only want to change
                      the username. Current password is required for any change.
                    </v-alert>
                    <v-text-field
                      v-model="accountForm.newUsername"
                      label="New Username (Optional)"
                      placeholder="Leave empty to keep current"
                      variant="outlined"
                      density="compact"
                      class="mb-2"
                    ></v-text-field>

                    <v-divider class="my-4"></v-divider>

                    <v-text-field
                      v-model="accountForm.newPassword"
                      label="New Password"
                      type="password"
                      variant="outlined"
                      density="compact"
                      class="mb-2"
                    ></v-text-field>
                    <v-text-field
                      v-model="accountForm.confirmPassword"
                      label="Confirm New Password"
                      type="password"
                      variant="outlined"
                      density="compact"
                      class="mb-4"
                    ></v-text-field>

                    <v-divider class="my-4"></v-divider>

                    <v-text-field
                      v-model="accountForm.oldPassword"
                      label="Current Password (Required)"
                      type="password"
                      variant="outlined"
                      density="compact"
                      class="mb-4"
                    ></v-text-field>
                    <v-btn color="primary" @click="updateAccountSettings"
                      >Update Account</v-btn
                    >
                  </v-card-text>
                </v-card>
              </v-expand-transition>

              <v-divider class="mb-6"></v-divider>

              <h3 class="text-h6 mb-4">Active Sessions</h3>
              <v-list density="compact" class="bg-grey-lighten-4 rounded mb-6">
                <v-list-item
                  v-for="session in sessions"
                  :key="session.id"
                  :title="getDeviceFromUA(session.user_agent)"
                  :subtitle="`${session.ip} - Expires: ${new Date(session.expires_at).toLocaleDateString()}`"
                >
                  <template v-slot:append>
                    <div class="d-flex align-center">
                      <v-btn
                        icon="mdi-delete"
                        variant="text"
                        color="error"
                        size="small"
                        @click="revokeSessionHandler(session.id)"
                      ></v-btn>
                    </div>
                  </template>
                </v-list-item>
                <v-list-item v-if="sessions.length === 0">
                  <v-list-item-title class="text-grey text-center"
                    >No active sessions found</v-list-item-title
                  >
                </v-list-item>
              </v-list>

              <v-divider class="mb-6"></v-divider>

              <h3 class="text-h6 mb-4">Device Access Tokens</h3>

              <v-alert
                v-if="generatedToken"
                type="success"
                variant="tonal"
                class="mb-4"
                closable
                @click:close="generatedToken = ''"
              >
                <div class="font-weight-bold mb-1">Token Generated!</div>
                <div class="text-caption mb-2">
                  Copy this token securely. It will not be shown again.
                </div>
                <v-text-field
                  :model-value="generatedToken"
                  readonly
                  variant="outlined"
                  density="compact"
                  hide-details
                  bg-color="white"
                  append-inner-icon="mdi-content-copy"
                  @click:append-inner="copyToken"
                ></v-text-field>
              </v-alert>

              <v-card variant="flat" class="border rounded mb-6">
                <v-card-title class="text-subtitle-1"
                  >Generate New Token</v-card-title
                >
                <v-card-text>
                  <div
                    class="d-flex flex-column flex-sm-row ga-2 align-stretch align-sm-center"
                  >
                    <v-text-field
                      v-model="newTokenName"
                      label="Token Name (e.g. Living Room Frame)"
                      variant="outlined"
                      density="compact"
                      hide-details
                      class="flex-grow-1"
                    ></v-text-field>
                    <v-select
                      v-model="newTokenDeviceId"
                      :items="[
                        { title: 'None', value: null },
                        ...availableDevices.map((d: any) => ({
                          title: d.name,
                          value: d.id,
                        })),
                      ]"
                      label="Bind to Device"
                      variant="outlined"
                      density="compact"
                      hide-details
                      :style="smAndDown ? '' : 'max-width: 220px'"
                    ></v-select>
                    <v-btn
                      color="primary"
                      :block="smAndDown"
                      @click="generateToken"
                      >Generate</v-btn
                    >
                  </div>
                </v-card-text>
              </v-card>

              <h4 class="text-subtitle-2 mb-2">Active Tokens</h4>
              <!-- Table on tablet/desktop -->
              <v-table
                v-if="!smAndDown"
                density="comfortable"
                class="border rounded"
              >
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Bound Device</th>
                    <th>Created At</th>
                    <th class="text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="token in authStore.tokens" :key="token.id">
                    <td>{{ token.name }}</td>
                    <td>
                      <v-select
                        :model-value="token.device_id"
                        :items="[
                          { title: 'None', value: null },
                          ...availableDevices.map((d: any) => ({
                            title: d.name,
                            value: d.id,
                          })),
                        ]"
                        variant="plain"
                        density="compact"
                        hide-details
                        style="max-width: 180px; font-size: inherit"
                        @update:model-value="
                          (val: any) => updateTokenDevice(token.id, val)
                        "
                      ></v-select>
                    </td>
                    <td>{{ new Date(token.created_at).toLocaleString() }}</td>
                    <td class="text-right">
                      <v-btn
                        color="error"
                        variant="text"
                        size="small"
                        @click="revokeToken(token.id)"
                      >
                        Revoke
                      </v-btn>
                    </td>
                  </tr>
                  <tr v-if="authStore.tokens.length === 0">
                    <td colspan="4" class="text-center text-grey py-4">
                      No active tokens found. Create one above to connect a
                      device.
                    </td>
                  </tr>
                </tbody>
              </v-table>

              <!-- Stacked cards on phones (no horizontal scroll) -->
              <template v-else>
                <v-card
                  v-for="token in authStore.tokens"
                  :key="token.id"
                  variant="flat"
                  class="border rounded mb-2"
                >
                  <v-card-text class="pa-3">
                    <div class="d-flex align-center">
                      <div class="flex-grow-1 text-truncate">
                        <div class="font-weight-medium text-truncate">
                          {{ token.name }}
                        </div>
                        <div class="text-caption text-grey">
                          {{ new Date(token.created_at).toLocaleString() }}
                        </div>
                      </div>
                      <v-btn
                        color="error"
                        variant="text"
                        size="small"
                        @click="revokeToken(token.id)"
                      >
                        Revoke
                      </v-btn>
                    </div>
                    <v-select
                      :model-value="token.device_id"
                      :items="[
                        { title: 'None', value: null },
                        ...availableDevices.map((d: any) => ({
                          title: d.name,
                          value: d.id,
                        })),
                      ]"
                      label="Bound device"
                      variant="outlined"
                      density="compact"
                      hide-details
                      class="mt-2"
                      @update:model-value="
                        (val: any) => updateTokenDevice(token.id, val)
                      "
                    ></v-select>
                  </v-card-text>
                </v-card>
                <div
                  v-if="authStore.tokens.length === 0"
                  class="text-center text-grey py-4"
                >
                  No active tokens found. Create one above to connect a device.
                </div>
              </template>
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
                      </td>
                      <td>
                        {{ device.host }}
                      </td>
                      <td class="text-right">
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
                      </v-list-item-subtitle>
                      <v-list-item-subtitle class="text-truncate">
                        {{ device.host }}
                      </v-list-item-subtitle>
                      <template #append>
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
                    <v-tab value="palette">Palette</v-tab>
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
                          <v-select
                            v-model="deviceConfig.rotate_interval"
                            :items="rotateIntervalOptions"
                            label="Rotation Interval"
                            variant="outlined"
                            density="compact"
                            hide-details
                            class="mb-2"
                            :disabled="!deviceConfig.auto_rotate"
                          />
                          <v-checkbox
                            v-model="deviceConfig.auto_rotate_aligned"
                            label="Align rotation to clock boundaries"
                            hide-details
                            class="mb-2"
                            :disabled="!deviceConfig.auto_rotate"
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

                        <v-divider class="my-3" />

                        <!-- Sleep Schedule (device config) -->
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
                          <v-col cols="12" md="4">
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
import { useAuthStore } from '../stores/auth';
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
  updateAccount,
  listSessions,
  revokeSession,
  listCalendars,
  googleCalendarLogin,
  googleCalendarLogout,
} from '../api';
import Gallery from './Gallery.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import AlbumPicker from './AlbumPicker.vue';

const { smAndDown } = useDisplay(); // true on phones / small tablets
const store = useSettingsStore();
const synologyStore = useSynologyStore();
const immichStore = useImmichStore();
const immichConnected = ref(false);

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

// Per-device Immich album bindings (Part B)
const deviceImmichAlbumIds = ref<number[]>([]);
// Per-device Synology album bindings (Part C)
const deviceSynologyAlbumIds = ref<number[]>([]);
const authStore = useAuthStore();
const galleryStore = useGalleryStore();
const activeMainTab = ref('devices');
const activeDataSourceTab = ref('gallery');
const galleryTab = ref('gallery');
const confirmDialog = ref();

// Image Source Binding State
const useThisServer = ref(true);
const selectedSource = ref('immich');
const sourceOptions = [
  { title: 'Gallery', value: 'gallery' },
  { title: 'Immich', value: 'immich' },
  { title: 'Google Photos', value: 'google_photos' },
  { title: 'Synology Photos', value: 'synology_photos' },
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
const savingDeviceConfig = ref(false);
const syncingFromDevice = ref(false);

// Device config (synced remotely to device)
const deviceConfig = reactive<Record<string, any>>({
  auto_rotate: false,
  rotate_interval: 3600,
  auto_rotate_aligned: true,
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

const rotateIntervalOptions = [
  { title: '5 minutes', value: 300 },
  { title: '15 minutes', value: 900 },
  { title: '30 minutes', value: 1800 },
  { title: '1 hour', value: 3600 },
  { title: '2 hours', value: 7200 },
  { title: '4 hours', value: 14400 },
  { title: '6 hours', value: 21600 },
  { title: '12 hours', value: 43200 },
  { title: '24 hours', value: 86400 },
];

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
      rotate_interval: cfg.rotate_interval ?? 3600,
      auto_rotate_aligned: cfg.auto_rotate_aligned ?? true,
      rotation_mode: cfg.rotation_mode ?? 'storage',
      image_url: cfg.image_url ?? '',
      save_downloaded_images: cfg.save_downloaded_images ?? true,
    });

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

watch(activeDataSourceTab, (val) => {
  if (val === 'url') {
    loadURLSources();
  } else if (val === 'google') {
    loadCalendars();
  }
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
    show_calendar: false,
    calendar_id: '',
    date_format: '',
    source: '',
  });
  Object.assign(deviceConfig, {
    auto_rotate: false,
    rotate_interval: 3600,
    auto_rotate_aligned: true,
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

const editDevice = async (device: Device) => {
  Object.assign(editingDevice, device);
  // Initialize display_orientation from device's orientation
  deviceConfig.display_orientation = device.orientation || 'landscape';
  isAddingDevice.value = false;
  deviceDialogTab.value = 'general';
  showEditDeviceDialog.value = true;
  deviceImmichAlbumIds.value = [];
  deviceSynologyAlbumIds.value = [];
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
        deviceSource
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

      const result = await updateDeviceConfig(editingDevice.id, {
        config: {
          device_name: editingDevice.name,
          auto_rotate: deviceConfig.auto_rotate,
          rotate_interval: deviceConfig.rotate_interval,
          auto_rotate_aligned: deviceConfig.auto_rotate_aligned,
          rotation_mode: deviceConfig.rotation_mode,
          image_url: imageUrl,
          save_downloaded_images: deviceConfig.save_downloaded_images,
          sleep_schedule_enabled: deviceConfig.sleep_schedule_enabled,
          sleep_schedule_start: startH * 60 + startM,
          sleep_schedule_end: endH * 60 + endM,
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
        color_palette: { ...devicePalette },
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

watch(galleryTab, (val) => {
  if (val === 'google_photos') {
    galleryStore.setSource('google_photos');
  } else if (val === 'synology_photos') {
    galleryStore.setSource('synology_photos');
  } else if (val === 'immich') {
    galleryStore.setSource('immich');
  } else if (val === 'gallery') {
    galleryStore.setSource('gallery');
  }
  // Persist the active gallery tab in the URL hash so a refresh restores it.
  if (val) window.location.hash = 'gtab=' + val;
});

const GALLERY_TABS = ['gallery', 'immich', 'google_photos', 'synology_photos'];
// Restore the gallery tab from the URL hash (#gtab=<tab>) on load.
const restoreGalleryTabFromHash = () => {
  const m = window.location.hash.match(/gtab=([\w-]+)/);
  if (m && GALLERY_TABS.includes(m[1])) {
    galleryTab.value = m[1];
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
watch(activeDataSourceTab, (_newVal, oldVal) => {
  if (oldVal === 'immich') maybePromptSync('immich');
  if (oldVal === 'synology_photos') maybePromptSync('synology_photos');
});
watch(activeMainTab, (_newVal, oldVal) => {
  if (oldVal === 'datasources') {
    maybePromptSync('immich');
    maybePromptSync('synology_photos');
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
  loadSessions();
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
  const parallelFetches: Promise<void>[] = [
    authStore.fetchTokens(),
    loadDevices(),
  ];

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

  await Promise.all(parallelFetches);

  // Parse URL params for deep linking (e.g. from OAuth callback)
  const params = new URLSearchParams(window.location.search);
  const tab = params.get('tab');
  const source = params.get('source');

  if (tab) {
    activeMainTab.value = tab;
  }
  if (source) {
    activeDataSourceTab.value = source;
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
    if (galleryTab.value === 'synology_photos') {
      await galleryStore.fetchPhotos();
    } else {
      galleryTab.value = 'synology_photos';
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
  } catch (e: any) {
    showMessage(
      'Connection Failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
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
    if (galleryTab.value === 'immich') {
      await galleryStore.fetchPhotos();
    } else {
      galleryTab.value = 'immich';
    }
  } catch (e: any) {
    showMessage(
      'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
      true
    );
  }
};

const clearImmich = () => deleteAllPhotosForSource('immich');

// Token Management
const generatedToken = ref('');
const newTokenName = ref('');
const newTokenDeviceId = ref<number | null>(null);

const copyToken = async () => {
  try {
    await navigator.clipboard.writeText(generatedToken.value);
    showMessage('Token copied to clipboard!');
  } catch (e) {
    // Fallback for non-secure contexts could be implemented here given time
    showMessage(
      'Failed to copy token automatically. Please copy manually.',
      true
    );
  }
};

// Password Change
const showAccountForm = ref(false);
const accountForm = reactive({
  oldPassword: '',
  newUsername: '',
  newPassword: '',
  confirmPassword: '',
});

const generateToken = async () => {
  if (!newTokenName.value) {
    showMessage('Please enter a name for the token.', true);
    return;
  }
  try {
    const token = await authStore.generateToken(
      newTokenName.value,
      newTokenDeviceId.value ?? undefined
    );
    generatedToken.value = token;
    newTokenName.value = '';
    newTokenDeviceId.value = null;
    showMessage('Token generated!');
  } catch (e: any) {
    showMessage(
      'Failed to generate token: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const updateTokenDevice = async (tokenId: number, deviceId: number | null) => {
  try {
    await authStore.updateTokenDevice(tokenId, deviceId);
    showMessage('Token device binding updated');
  } catch (e: any) {
    showMessage(
      'Failed to update token: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const revokeToken = async (id: number) => {
  if (
    !(await confirmDialog.value.open(
      'Revoke this token? Device will lose access.'
    ))
  )
    return;
  try {
    await authStore.revokeToken(id);
    showMessage('Token revoked.');
  } catch (e: any) {
    showMessage('Failed: ' + e.message, true);
  }
};

const updateAccountSettings = async () => {
  if (!accountForm.oldPassword) {
    showMessage('Current password is required.', true);
    return;
  }
  if (!accountForm.newUsername && !accountForm.newPassword) {
    showMessage('Please provide a new username or password.', true);
    return;
  }
  if (accountForm.newPassword) {
    if (accountForm.newPassword !== accountForm.confirmPassword) {
      showMessage('New passwords do not match.', true);
      return;
    }
    if (accountForm.newPassword.length < 6) {
      showMessage('New password must be at least 6 characters.', true);
      return;
    }
  }

  try {
    await updateAccount(
      accountForm.oldPassword,
      accountForm.newUsername,
      accountForm.newPassword
    );
    accountForm.oldPassword = '';
    accountForm.newUsername = '';
    accountForm.newPassword = '';
    accountForm.confirmPassword = '';
    showMessage('Account updated successfully!');
  } catch (e: any) {
    showMessage('Failed: ' + (e.response?.data?.error || e.message), true);
  }
};

// Sessions
const sessions = ref<any[]>([]);

const loadSessions = async () => {
  try {
    sessions.value = await listSessions();
  } catch (e) {
    console.error('Failed to load sessions', e);
  }
};

const revokeSessionHandler = async (id: number) => {
  if (!confirm('Are you sure you want to revoke this session?')) return;
  try {
    await revokeSession(id);
    await loadSessions();
    showMessage('Session revoked');
  } catch (e: any) {
    showMessage('Failed: ' + (e.response?.data?.error || e.message), true);
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

const getDeviceFromUA = (ua: string) => {
  if (!ua) return 'Unknown Device';
  if (ua.includes('iPhone')) return 'iPhone';
  if (ua.includes('iPad')) return 'iPad';
  if (ua.includes('Macintosh')) return 'Mac';
  if (ua.includes('Windows')) return 'Windows';
  if (ua.includes('Android')) return 'Android';
  if (ua.includes('Linux')) return 'Linux';
  return 'Other Device';
};
</script>

<style scoped>
.color-swatch {
  height: 60px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.12);
}
</style>
