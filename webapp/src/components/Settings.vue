```html
<template>
  <div class="pa-4">
    <!-- Gallery Card -->
    <v-card class="mb-6">
      <v-tabs v-model="galleryTab" color="primary">
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
          <v-tab value="security">Security</v-tab>
        </v-tabs>

        <v-window v-model="activeMainTab">
          <!-- Data Sources Tab -->
          <v-window-item value="datasources">
            <v-tabs
              v-model="activeDataSourceTab"
              color="primary"
              density="compact"
              class="mb-4"
            >
              <v-tab value="google">Google</v-tab>
              <v-tab value="synology_photos">Synology</v-tab>
              <v-tab value="telegram">Telegram</v-tab>
              <v-tab value="url">URL Proxy</v-tab>
              <v-tab value="ai_generation">AI Generation</v-tab>
            </v-tabs>

            <v-window v-model="activeDataSourceTab">
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

                  <v-text-field
                    :model-value="getImageUrl('url_proxy')"
                    label="Image Endpoint URL (for firmware config)"
                    readonly
                    variant="outlined"
                    density="compact"
                    append-inner-icon="mdi-content-copy"
                    @click:append-inner="
                      copyToClipboard(getImageUrl('url_proxy'))
                    "
                    class="mb-4"
                  ></v-text-field>

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

                    <v-text-field
                      :model-value="getImageUrl('google_photos')"
                      label="Image Endpoint URL (for firmware config)"
                      readonly
                      variant="outlined"
                      density="compact"
                      append-inner-icon="mdi-content-copy"
                      @click:append-inner="
                        copyToClipboard(getImageUrl('google_photos'))
                      "
                    ></v-text-field>

                    <v-btn color="error" variant="text" @click="logoutGoogle">
                      Disconnect Google Photos
                    </v-btn>
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

                    <v-text-field
                      :model-value="getImageUrl('synology_photos')"
                      label="Image Endpoint URL (for firmware config)"
                      readonly
                      variant="outlined"
                      density="compact"
                      append-inner-icon="mdi-content-copy"
                      @click:append-inner="
                        copyToClipboard(getImageUrl('synology_photos'))
                      "
                    ></v-text-field>

                    <v-row class="mt-2">
                      <v-col cols="12" sm="8">
                        <v-select
                          v-model="form.synology_album_id"
                          :items="synologyAlbumOptions"
                          item-title="name"
                          item-value="id"
                          label="Sync Album"
                          variant="outlined"
                          density="compact"
                          hint="Select an album to limit sync"
                          persistent-hint
                        ></v-select>
                      </v-col>
                      <v-col cols="12" sm="4">
                        <v-btn block variant="outlined" @click="loadAlbums"
                          >Refresh Albums</v-btn
                        >
                      </v-col>
                    </v-row>

                    <v-select
                      v-model="form.synology_space"
                      :items="[
                        { title: 'Personal Space', value: 'personal' },
                        { title: 'Shared Space', value: 'shared' },
                      ]"
                      label="Photo Space"
                      variant="outlined"
                      density="compact"
                      class="mt-4"
                    ></v-select>

                    <v-btn color="grey-darken-1" class="mt-2 mb-4" @click="save"
                      >Save Album Selection</v-btn
                    >

                    <div class="d-flex flex-wrap ga-2">
                      <v-btn color="primary" @click="syncSynology"
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
                      @click="testSynology"
                    >
                      Test Connection & Login
                    </v-btn>
                  </div>
                </v-card-text>
              </v-window-item>

              <!-- Telegram -->
              <v-window-item value="telegram">
                <v-card-text>
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
                      :model-value="getImageUrl('telegram')"
                      label="Image Endpoint URL (for firmware config)"
                      readonly
                      variant="outlined"
                      density="compact"
                      append-inner-icon="mdi-content-copy"
                      @click:append-inner="
                        copyToClipboard(getImageUrl('telegram'))
                      "
                    ></v-text-field>

                    <v-text-field
                      v-model="form.telegram_bot_token"
                      label="Telegram Bot Token"
                      variant="outlined"
                      class="mt-4"
                    ></v-text-field>

                    <v-divider class="my-4"></v-divider>

                    <h3 class="text-subtitle-1 font-weight-bold mb-2">
                      Push to Device
                    </h3>
                    <div class="text-caption text-grey mb-2">
                      Enable to push generic images directly to the device
                      display when sent to the bot.
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
                      hint="Send photos to your bot to display them. Only the last photo will be shown."
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
                    :model-value="getImageUrl('ai_generation')"
                    label="Image Endpoint URL (for firmware config)"
                    readonly
                    variant="outlined"
                    density="compact"
                    append-inner-icon="mdi-content-copy"
                    @click:append-inner="
                      copyToClipboard(getImageUrl('ai_generation'))
                    "
                    class="mb-4"
                  ></v-text-field>

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

          <!-- Security Tab -->
          <v-window-item value="security">
            <v-card-text>
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

              <v-card variant="outlined" class="mb-6">
                <v-card-title class="text-subtitle-1"
                  >Generate New Token</v-card-title
                >
                <v-card-text>
                  <div class="d-flex ga-2 align-center">
                    <v-text-field
                      v-model="newTokenName"
                      label="Token Name (e.g. Living Room Frame)"
                      variant="outlined"
                      density="compact"
                      hide-details
                      class="flex-grow-1"
                    ></v-text-field>
                    <v-btn color="primary" @click="generateToken"
                      >Generate</v-btn
                    >
                  </div>
                </v-card-text>
              </v-card>

              <h4 class="text-subtitle-2 mb-2">Active Tokens</h4>
              <v-table density="comfortable" class="border rounded">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Created At</th>
                    <th class="text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="token in authStore.tokens" :key="token.id">
                    <td>{{ token.name }}</td>
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
                    <td colspan="3" class="text-center text-grey py-4">
                      No active tokens found. Create one above to connect a
                      device.
                    </td>
                  </tr>
                </tbody>
              </v-table>
            </v-card-text>
          </v-window-item>
          <!-- Devices Tab -->
          <v-window-item value="devices">
            <v-card-text>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                Manage your ESP32 PhotoFrame devices here. These devices will be
                available for direct push from the Gallery.
              </v-alert>

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

              <v-table density="comfortable" class="border rounded">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Resolution</th>
                    <th>Host</th>
                    <th class="text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="device in availableDevices" :key="device.id">
                    <td>{{ device.name }}</td>
                    <td>
                      {{ device.width }}x{{ device.height }} ({{
                        device.orientation
                      }})
                    </td>
                    <td>
                      {{ device.host }}
                      <v-chip
                        v-if="device.use_device_parameter"
                        size="x-small"
                        color="info"
                        class="ml-2"
                        >Auto-Param</v-chip
                      >
                    </td>
                    <td class="text-right">
                      <v-btn
                        color="primary"
                        variant="text"
                        size="small"
                        icon="mdi-pencil"
                        @click="editDevice(device)"
                      ></v-btn>
                      <v-btn
                        v-if="device.use_device_parameter"
                        color="info"
                        variant="text"
                        size="small"
                        icon="mdi-refresh"
                        title="Refresh Device Parameters"
                        @click="refreshDeviceParams(device)"
                      ></v-btn>
                      <v-btn
                        color="secondary"
                        variant="text"
                        size="small"
                        icon="mdi-link"
                        title="Bind Image Source"
                        @click="openBindSourceDialog(device)"
                      ></v-btn>
                      <v-btn
                        color="error"
                        variant="text"
                        size="small"
                        icon="mdi-delete"
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

              <!-- Edit Device Dialog -->
              <v-dialog v-model="showEditDeviceDialog" max-width="500px">
                <v-card>
                  <v-card-title>{{
                    isAddingDevice ? 'Add Device' : 'Edit Device'
                  }}</v-card-title>
                  <v-card-text>
                    <v-expansion-panels
                      v-model="deviceDialogPanels"
                      multiple
                      variant="accordion"
                    >
                      <!-- General -->
                      <v-expansion-panel value="general">
                        <v-expansion-panel-title>
                          <div class="d-flex align-center ga-2">
                            <v-icon size="small">mdi-cog</v-icon>
                            <span class="text-subtitle-2">General</span>
                          </div>
                        </v-expansion-panel-title>
                        <v-expansion-panel-text>
                          <div class="d-flex ga-2 mt-2">
                            <v-text-field
                              v-model="editingDevice.name"
                              label="Name"
                              variant="outlined"
                              density="compact"
                              hide-details
                            ></v-text-field>
                          </div>
                          <v-text-field
                            v-model="editingDevice.host"
                            label="Host / IP"
                            variant="outlined"
                            density="compact"
                            class="mt-3"
                            hide-details
                          ></v-text-field>
                          <v-checkbox
                            v-model="editingDevice.use_device_parameter"
                            label="Fetch parameters from device"
                            color="primary"
                            density="compact"
                            hide-details
                            class="mt-2"
                          ></v-checkbox>
                          <v-checkbox
                            v-model="editingDevice.enable_collage"
                            label="Enable Collage Mode"
                            color="primary"
                            density="compact"
                            hide-details
                          ></v-checkbox>
                          <v-select
                            v-model="editingDevice.display_mode"
                            :items="[
                              {
                                title: 'Cover (fill, may crop)',
                                value: 'cover',
                              },
                              {
                                title: 'Contain (show entire photo)',
                                value: 'contain',
                              },
                            ]"
                            label="Photo Display Mode"
                            variant="outlined"
                            density="compact"
                            class="mt-3"
                            hide-details
                          ></v-select>
                        </v-expansion-panel-text>
                      </v-expansion-panel>

                      <!-- Overlay -->
                      <v-expansion-panel value="overlay">
                        <v-expansion-panel-title>
                          <div class="d-flex align-center ga-2">
                            <v-icon size="small">mdi-image-text</v-icon>
                            <span class="text-subtitle-2">Overlay</span>
                            <span class="text-caption text-grey ml-2">
                              {{
                                [
                                  editingDevice.show_date ? 'Date' : '',
                                  editingDevice.show_weather ? 'Weather' : '',
                                ]
                                  .filter(Boolean)
                                  .join(' · ') || 'None'
                              }}
                            </span>
                          </div>
                        </v-expansion-panel-title>
                        <v-expansion-panel-text>
                          <div class="d-flex ga-4 mt-2">
                            <v-checkbox
                              v-model="editingDevice.show_date"
                              label="Show Date"
                              color="primary"
                              density="compact"
                              hide-details
                            ></v-checkbox>
                            <v-checkbox
                              v-model="editingDevice.show_weather"
                              label="Show Weather"
                              color="primary"
                              density="compact"
                              hide-details
                            ></v-checkbox>
                          </div>
                          <div
                            v-if="editingDevice.show_weather"
                            class="d-flex ga-2 mt-3"
                          >
                            <v-text-field
                              v-model.number="editingDevice.weather_lat"
                              label="Latitude"
                              variant="outlined"
                              density="compact"
                              hide-details
                              type="number"
                            ></v-text-field>
                            <v-text-field
                              v-model.number="editingDevice.weather_lon"
                              label="Longitude"
                              variant="outlined"
                              density="compact"
                              hide-details
                              type="number"
                            ></v-text-field>
                          </div>
                        </v-expansion-panel-text>
                      </v-expansion-panel>

                      <!-- Layout & Calendar -->
                      <v-expansion-panel value="layout">
                        <v-expansion-panel-title>
                          <div class="d-flex align-center ga-2">
                            <v-icon size="small"
                              >mdi-view-dashboard-outline</v-icon
                            >
                            <span class="text-subtitle-2"
                              >Layout & Calendar</span
                            >
                            <span class="text-caption text-grey ml-2">
                              {{
                                filteredLayoutOptions.find(
                                  (o) => o.value === editingDevice.layout
                                )?.title || 'Photo Overlay'
                              }}{{
                                editingDevice.show_calendar ? ' · Calendar' : ''
                              }}
                            </span>
                          </div>
                        </v-expansion-panel-title>
                        <v-expansion-panel-text>
                          <div class="d-flex flex-wrap ga-3 mb-3 mt-2">
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
                              style="width: 110px; cursor: pointer"
                              @click="editingDevice.layout = opt.value"
                            >
                              <div
                                class="layout-preview mb-1"
                                v-html="
                                  getLayoutPreviewSvg(
                                    opt.value,
                                    editingDevice.orientation || 'landscape'
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
                          <div
                            class="text-caption text-grey ml-2 mb-3"
                            v-if="editingDevice.layout"
                          >
                            {{ layoutDescriptions[editingDevice.layout] }}
                          </div>
                          <v-checkbox
                            v-model="editingDevice.show_calendar"
                            label="Show Google Calendar Events"
                            color="primary"
                            density="compact"
                            hide-details
                          ></v-checkbox>
                          <v-alert
                            v-if="
                              editingDevice.show_calendar &&
                              form.google_calendar_connected !== 'true'
                            "
                            type="warning"
                            variant="tonal"
                            density="compact"
                            class="mt-2"
                          >
                            Google Calendar not connected. Connect in Data
                            Sources &rarr; Google to enable calendar.
                          </v-alert>
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
                        </v-expansion-panel-text>
                      </v-expansion-panel>

                      <!-- AI Generation -->
                      <v-expansion-panel value="ai">
                        <v-expansion-panel-title>
                          <div class="d-flex align-center ga-2">
                            <v-icon size="small">mdi-creation</v-icon>
                            <span class="text-subtitle-2">AI Generation</span>
                            <span class="text-caption text-grey ml-2">
                              {{
                                editingDevice.ai_provider
                                  ? (editingDevice.ai_provider === 'openai'
                                      ? 'OpenAI'
                                      : 'Gemini') +
                                    (editingDevice.ai_model
                                      ? ' · ' + editingDevice.ai_model
                                      : '')
                                  : 'Off'
                              }}
                            </span>
                          </div>
                        </v-expansion-panel-title>
                        <v-expansion-panel-text>
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
                            class="mt-2 mb-3"
                            hide-details
                          ></v-select>

                          <v-alert
                            v-if="
                              editingDevice.ai_provider === 'openai' &&
                              !form.openai_api_key
                            "
                            type="warning"
                            variant="tonal"
                            density="compact"
                            class="mb-3"
                          >
                            OpenAI API Key not configured. Please add it in Data
                            Sources → AI Generation.
                          </v-alert>

                          <v-alert
                            v-if="
                              editingDevice.ai_provider === 'google' &&
                              !form.google_api_key
                            "
                            type="warning"
                            variant="tonal"
                            density="compact"
                            class="mb-3"
                          >
                            Google API Key not configured. Please add it in Data
                            Sources → AI Generation.
                          </v-alert>

                          <v-select
                            v-if="editingDevice.ai_provider"
                            v-model="editingDevice.ai_model"
                            :items="
                              aiModelOptionsForProvider(
                                editingDevice.ai_provider
                              )
                            "
                            label="Model"
                            variant="outlined"
                            density="compact"
                            class="mb-3"
                            hide-details
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
                        </v-expansion-panel-text>
                      </v-expansion-panel>
                    </v-expansion-panels>
                  </v-card-text>
                  <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                      color="grey"
                      variant="text"
                      @click="showEditDeviceDialog = false"
                      >Cancel</v-btn
                    >
                    <v-btn color="primary" @click="saveDevice">{{
                      isAddingDevice ? 'Add' : 'Save'
                    }}</v-btn>
                  </v-card-actions>
                </v-card>
              </v-dialog>

              <!-- Bind Source Dialog -->
              <v-dialog v-model="showBindSourceDialog" max-width="500px">
                <v-card>
                  <v-card-title>Bind Image Source</v-card-title>
                  <v-card-text>
                    <v-alert
                      type="info"
                      variant="tonal"
                      class="mb-4"
                      density="compact"
                    >
                      This will configure the device to fetch images from the
                      selected source.
                      <br />
                      <strong>Note:</strong> This updates the device's
                      configuration immediately.
                    </v-alert>
                    <v-select
                      v-model="selectedSource"
                      :items="sourceOptions"
                      label="Select Source"
                      variant="outlined"
                    ></v-select>
                  </v-card-text>
                  <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                      color="grey"
                      variant="text"
                      @click="showBindSourceDialog = false"
                      >Cancel</v-btn
                    >
                    <v-btn
                      color="primary"
                      @click="bindDeviceSource"
                      :loading="isBinding"
                      >Bind & Configure</v-btn
                    >
                  </v-card-actions>
                </v-card>
              </v-dialog>
            </v-card-text>
          </v-window-item>
        </v-window>
      </div>

      <!-- Global Snackbar for Messages -->
      <v-snackbar
        v-model="snackbar.show"
        :color="snackbar.color"
        :timeout="3000"
        location="bottom right"
      >
        {{ snackbar.message }}
        <template v-slot:actions>
          <v-btn variant="text" @click="snackbar.show = false">Close</v-btn>
        </template>
      </v-snackbar>

      <ConfirmDialog ref="confirmDialog" />
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, computed, watch } from 'vue';
import { useSettingsStore } from '../stores/settings';
import { useSynologyStore } from '../stores/synology';
import { useAuthStore } from '../stores/auth';
import { useGalleryStore } from '../stores/gallery';
import {
  api,
  listDevices,
  addDevice,
  deleteDevice,
  updateDevice,
  type Device,
  createURLSource,
  updateURLSource,
  listURLSources,
  deleteURLSource,
  configureDeviceSource,
  updateAccount,
  listSessions,
  revokeSession,
  listCalendars,
  googleCalendarLogin,
  googleCalendarLogout,
} from '../api';
import Gallery from './Gallery.vue';
import ConfirmDialog from './ConfirmDialog.vue';

const store = useSettingsStore();
const synologyStore = useSynologyStore();
const authStore = useAuthStore();
const galleryStore = useGalleryStore();
const activeMainTab = ref('devices');
const activeDataSourceTab = ref('google');
const galleryTab = ref('google_photos');
const confirmDialog = ref();

// Device Binding State
const showBindSourceDialog = ref(false);
const bindingDevice = ref<Device | null>(null);
const selectedSource = ref('google_photos');
const sourceOptions = [
  { title: 'Google Photos', value: 'google_photos' },
  { title: 'Synology Photos', value: 'synology_photos' },
  { title: 'Telegram', value: 'telegram' },
  { title: 'URL Proxy', value: 'url_proxy' },
  { title: 'AI Generation', value: 'ai_generation' },
];
const isBinding = ref(false);

const openBindSourceDialog = (device: Device) => {
  bindingDevice.value = device;
  selectedSource.value = 'google_photos';
  showBindSourceDialog.value = true;
};

const bindDeviceSource = async () => {
  if (!bindingDevice.value) return;
  isBinding.value = true;
  try {
    const res = await configureDeviceSource(
      bindingDevice.value.id,
      selectedSource.value
    );
    showMessage(
      `Device configured to use source: ${selectedSource.value}. Image URL: ${res.url}`
    );
    showBindSourceDialog.value = false;
  } catch (e: any) {
    showMessage(
      'Failed to bind source: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    isBinding.value = false;
  }
};

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

// Edit Device State (declared here because computed/watch below reference editingDevice)
const showEditDeviceDialog = ref(false);
const editingDevice = reactive<Partial<Device>>({});

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
  const orientation = editingDevice.orientation || 'landscape';
  return allLayoutOptions.filter((opt) =>
    opt.orientations.includes(orientation)
  );
});

// Auto-select first layout if current layout is not valid for orientation
watch(
  () => editingDevice.orientation,
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

const layoutDescriptions: Record<string, string> = {
  photo_info:
    'Photo on top with a dedicated info strip showing date, weather, and calendar events.',
  photo_overlay:
    'Full-screen photo with a semi-transparent overlay showing date, weather, and events.',
  side_panel:
    'Photo with a side panel (landscape) or bottom panel (portrait) showing weather and events.',
};

const aiModelOptionsForProvider = (provider: string | undefined) => {
  if (provider === 'openai') {
    return [
      { title: 'GPT Image 1.5', value: 'gpt-image-1.5' },
      { title: 'GPT Image 1', value: 'gpt-image-1' },
      { title: 'GPT Image 1 Mini', value: 'gpt-image-1-mini' },
      { title: 'DALL-E 3', value: 'dall-e-3' },
      { title: 'DALL-E 2', value: 'dall-e-2' },
    ];
  } else if (provider === 'google') {
    return [
      { title: 'Gemini 2.5 Flash Image', value: 'gemini-2.5-flash-image' },
      { title: 'Gemini 3 Pro Image', value: 'gemini-3-pro-image-preview' },
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
        editingDevice.ai_model = 'gemini-2.5-flash-image';
      } else {
        editingDevice.ai_model = '';
      }
    }
  }
);

const isAddingDevice = ref(false);
const deviceDialogPanels = ref<string[]>(['general']);

const openAddDeviceDialog = () => {
  // Reset editingDevice to defaults for a new device
  Object.assign(editingDevice, {
    id: undefined,
    name: '',
    host: '',
    width: 0,
    height: 0,
    orientation: '',
    use_device_parameter: false,
    enable_collage: false,
    show_date: true,
    show_weather: true,
    weather_lat: null,
    weather_lon: null,
    ai_provider: '',
    ai_model: '',
    ai_prompt: '',
    layout: 'photo_overlay',
    display_mode: 'cover',
    show_calendar: false,
    calendar_id: '',
  });
  isAddingDevice.value = true;
  deviceDialogPanels.value = ['general'];
  showEditDeviceDialog.value = true;
};

const editDevice = (device: Device) => {
  Object.assign(editingDevice, device);
  isAddingDevice.value = false;
  deviceDialogPanels.value = ['general'];
  showEditDeviceDialog.value = true;
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
  try {
    if (isAddingDevice.value) {
      await addDevice({
        host: editingDevice.host!,
        use_device_parameter: editingDevice.use_device_parameter!,
        enable_collage: editingDevice.enable_collage!,
        show_date: editingDevice.show_date!,
        show_weather: editingDevice.show_weather!,
        weather_lat: editingDevice.weather_lat || 0,
        weather_lon: editingDevice.weather_lon || 0,
        layout: editingDevice.layout || 'photo_overlay',
        display_mode: editingDevice.display_mode || 'cover',
        show_calendar: editingDevice.show_calendar || false,
        calendar_id: editingDevice.calendar_id || '',
      });
      showMessage('Device added successfully');
    } else {
      if (!editingDevice.id) return;
      await updateDevice(
        editingDevice.id,
        editingDevice.name!,
        editingDevice.host!,
        editingDevice.width!,
        editingDevice.height!,
        editingDevice.orientation!,
        editingDevice.use_device_parameter!,
        editingDevice.enable_collage!,
        editingDevice.show_date!,
        editingDevice.show_weather!,
        editingDevice.weather_lat || 0,
        editingDevice.weather_lon || 0,
        editingDevice.ai_provider || '',
        editingDevice.ai_model || '',
        editingDevice.ai_prompt || '',
        editingDevice.layout || 'photo_overlay',
        editingDevice.display_mode || 'cover',
        editingDevice.show_calendar || false,
        editingDevice.calendar_id || ''
      );
      showMessage('Device updated successfully');
    }
    await loadDevices();
    showEditDeviceDialog.value = false;
  } catch (e: any) {
    showMessage('Failed to save device: ' + e.message, true);
  }
};

const refreshDeviceParams = async (device: Device) => {
  deviceListLoading.value = true;
  try {
    // Trigger refresh by sending empty/0 values with use_device_parameter=true
    await updateDevice(
      device.id,
      '', // Empty name triggers fetch
      device.host,
      0, // Width 0 triggers fetch
      0, // Height 0 triggers fetch
      '', // Empty orientation triggers fetch
      true, // Ensure enabled
      device.enable_collage,
      device.show_date!,
      device.show_weather!,
      device.weather_lat || 0,
      device.weather_lon || 0,
      device.ai_provider || '',
      device.ai_model || '',
      device.ai_prompt || '',
      device.layout || 'photo_overlay',
      device.display_mode || 'cover',
      device.show_calendar || false,
      device.calendar_id || ''
    );
    await loadDevices();
    showMessage('Device parameters refreshed from device');
  } catch (e: any) {
    showMessage('Failed to refresh parameters: ' + e.message, true);
  } finally {
    deviceListLoading.value = false;
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
  }
});

const snackbar = reactive({
  show: false,
  message: '',
  color: 'success',
});

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
  synology_space: 'personal',
  albums: [] as any[],
  telegram_bot_token: '',
  telegram_push_enabled: false,
  telegram_target_device_id: [] as number[],
  openai_api_key: '',
  google_api_key: '',
  device_host: '', // Keep for backward compatibility/display? Or remove. Remove from form, keep in store maybe?
});

const synologyAlbumOptions = computed(() => {
  return [{ id: '', name: 'All Photos' }, ...form.albums];
});

// Helper to show snackbar
const showMessage = (msg: string, isError = false) => {
  snackbar.message = msg;
  snackbar.color = isError ? 'error' : 'success';
  snackbar.show = true;
};

onMounted(async () => {
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
    synology_space: store.settings.synology_space || 'personal',
    synology_album_id: store.settings.synology_album_id
      ? parseInt(store.settings.synology_album_id)
      : '',
    synology_sid: store.settings.synology_sid || '',
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

  // Fetch Synology photo count if connected
  if (form.synology_sid) {
    await synologyStore.fetchCount();
  }

  await authStore.fetchTokens();
  await loadDevices();

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
    synology_space: form.synology_space,
    synology_album_id: String(form.synology_album_id),
    openai_api_key: form.openai_api_key,
    google_api_key: form.google_api_key,
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

const syncSynology = async () => {
  await saveSettingsInternal();
  try {
    await synologyStore.sync();
    showMessage('Sync started/completed successfully!');
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

const clearSynology = async () => {
  if (
    !(await confirmDialog.value.open(
      'Are you sure you want to clear all Synology photo references? Local files will not be deleted.'
    ))
  )
    return;

  try {
    await api.post('/synology/clear');
    showMessage('All Synology photos cleared from database.');
    await synologyStore.fetchCount();
  } catch (e: any) {
    showMessage(
      'Clear Failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Token Management
const generatedToken = ref('');
const newTokenName = ref('');

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
    const token = await authStore.generateToken(newTokenName.value);
    generatedToken.value = token;
    newTokenName.value = '';
    showMessage('Token generated!');
  } catch (e: any) {
    showMessage(
      'Failed to generate token: ' + (e.response?.data?.error || e.message),
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
const getImageUrl = (source: string) => {
  const hostname = window.location.hostname;
  const protocol = window.location.protocol;
  // Use configurable port via env var, default to 9607 for production
  const addonPort = import.meta.env.VITE_ADDON_PORT || '9607';
  return `${protocol}//${hostname}:${addonPort}/image/${source}`;
};

// Copy to clipboard
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    showMessage('URL copied to clipboard!');
  } catch (e) {
    showMessage('Failed to copy to clipboard', true);
  }
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
