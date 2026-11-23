<template>
  <div class="pa-1 pa-sm-4">
    <!-- Sources Card: one source at a time, picked from the dropdown. Photos
         on top, that source's settings (a connection's card, the gallery's
         Telegram bot, the URL list, the AI keys) below. -->
    <v-card class="mb-6">
      <v-card-text>
        <SourcePicker
          v-model="sourceKey"
          class="mb-4"
          @add="showAddConnection = true"
        ></SourcePicker>

        <v-alert
          v-if="
            connectionsStore.loaded && connectionsStore.connections.length === 0
          "
          type="info"
          variant="tonal"
          density="compact"
          class="mb-4"
        >
          No photo sources connected yet. Click <b>+</b> to add an Immich
          server, a Synology Photos or Google account, or an Unsplash or Pexels
          key.
        </v-alert>

        <!-- A connection: its photos first, its card (credentials, albums or
             topics, sync) below, as the per-source tabs had it. -->
        <template v-if="activeConnection">
          <Gallery />
          <v-divider class="my-6"></v-divider>
          <component
            :is="cardComponentFor(activeConnection.source)"
            :key="activeConnection.id"
            ref="activeCard"
            v-bind="activeCardProps"
            :connection="activeConnection"
            :confirm="confirmPrompt"
            :inform="informPrompt"
            :show-message="showMessage"
            @synced="onConnectionSynced"
            @deleted="loadDevices"
          ></component>
        </template>

        <template v-else>
          <div v-if="sourceHasGallery">
            <Gallery />
            <v-divider class="my-6"></v-divider>
          </div>
          <v-window v-model="sourceKey" :touch="false">
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

            <!-- Gallery -->
            <v-window-item value="gallery">
              <v-card-text>
                <h3 class="text-subtitle-1 font-weight-bold mb-1">Albums</h3>
                <div class="text-caption text-grey mb-3">
                  Folders inside the gallery. Select an album above to upload
                  into it, and pick albums per frame in the device dialog.
                  Photos in no album still belong to the whole gallery.
                </div>

                <v-list
                  v-if="galleryStore.albums.length > 0"
                  density="compact"
                  class="border rounded mb-3 py-0"
                >
                  <v-list-item
                    v-for="album in galleryStore.albums"
                    :key="album.id"
                  >
                    <template #prepend>
                      <v-icon icon="mdi-folder-outline"></v-icon>
                    </template>
                    <v-list-item-title v-if="renamingAlbumId !== album.id">
                      {{ album.name }}
                    </v-list-item-title>
                    <v-text-field
                      v-else
                      v-model="renamingAlbumName"
                      density="compact"
                      variant="outlined"
                      hide-details
                      autofocus
                      class="my-1"
                      style="max-width: 320px"
                      @keyup.enter="saveAlbumName(album)"
                      @keyup.esc="renamingAlbumId = null"
                      @blur="saveAlbumName(album)"
                    ></v-text-field>
                    <v-list-item-subtitle>
                      {{ album.asset_count }} photo{{
                        album.asset_count === 1 ? '' : 's'
                      }}
                    </v-list-item-subtitle>
                    <template #append>
                      <v-btn
                        icon="mdi-pencil"
                        size="x-small"
                        variant="text"
                        title="Rename"
                        @click="startRenameAlbum(album)"
                      ></v-btn>
                      <v-menu>
                        <template #activator="{ props: menu }">
                          <v-btn
                            v-bind="menu"
                            icon="mdi-delete-outline"
                            size="small"
                            variant="text"
                            color="error"
                            title="Delete album"
                          ></v-btn>
                        </template>
                        <v-list density="compact">
                          <v-list-item
                            title="Delete album, keep its photos"
                            subtitle="The photos stay in the gallery, unfiled"
                            @click="deleteGalleryAlbum(album, false)"
                          ></v-list-item>
                          <v-list-item
                            title="Delete album and its photos"
                            base-color="error"
                            @click="deleteGalleryAlbum(album, true)"
                          ></v-list-item>
                        </v-list>
                      </v-menu>
                    </template>
                  </v-list-item>
                </v-list>

                <div class="d-flex align-center ga-2 mb-6">
                  <v-text-field
                    v-model="newAlbumName"
                    label="New album"
                    placeholder="e.g. Family, Art"
                    variant="outlined"
                    density="compact"
                    hide-details
                    style="max-width: 320px"
                    @keyup.enter="createGalleryAlbum"
                  ></v-text-field>
                  <v-btn
                    color="primary"
                    variant="tonal"
                    prepend-icon="mdi-plus"
                    :disabled="!newAlbumName.trim()"
                    :loading="creatingAlbum"
                    @click="createGalleryAlbum"
                    >Add</v-btn
                  >
                </div>

                <v-divider class="mb-6"></v-divider>

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
                    pushed to the selected devices immediately. They are added
                    to the gallery either way.
                  </div>

                  <v-select
                    v-if="galleryStore.albums.length > 0"
                    v-model="form.telegram_album_id"
                    :items="telegramAlbumOptions"
                    item-title="name"
                    item-value="id"
                    label="Upload to album"
                    variant="outlined"
                    density="compact"
                    hint="Photos sent to the bot are filed under this album"
                    persistent-hint
                    class="mb-4"
                    @update:model-value="saveSettingsInternal()"
                  ></v-select>

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
                  API keys below, then set the prompt/model per-device in the
                  Edit Device dialog.
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
        </template>
      </v-card-text>
    </v-card>

    <AddConnectionDialog
      v-model="showAddConnection"
      :google-ready="googleClientReady"
      :before-google-connect="saveSettingsInternal"
      @created="onConnectionCreated"
      @open-system="openSystemTab"
    ></AddConnectionDialog>
    <AddConnectionDialog
      v-model="showAddCalendar"
      :sources="[CALENDAR_SOURCE]"
      :google-ready="googleClientReady"
      :before-google-connect="saveSettingsInternal"
      @open-system="openSystemTab"
    ></AddConnectionDialog>

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
          <v-tab value="calendar">Calendar</v-tab>
          <v-tab value="security">System</v-tab>
        </v-tabs>

        <v-window v-model="activeMainTab" :touch="false">
          <!-- Calendar Tab: the Google accounts whose calendars frames can show -->
          <v-window-item value="calendar">
            <v-card-text>
              <div class="text-caption text-grey mb-4">
                Sign in with the Google accounts whose calendars your frames
                should show. Each frame picks an account and one of its
                calendars in the device dialog (<b>Devices</b> → edit →
                <b>Show Google Calendar Events</b>).
              </div>

              <v-alert
                v-if="!googleClientReady"
                type="warning"
                variant="tonal"
                density="compact"
                class="mb-4"
              >
                Enter the Google API client ID and secret under
                <b>System → Google</b> first.
                <template #append>
                  <v-btn
                    size="small"
                    variant="text"
                    @click="activeMainTab = 'security'"
                    >Open</v-btn
                  >
                </template>
              </v-alert>

              <v-alert
                v-else-if="
                  connectionsStore.loaded && calendarConnections.length === 0
                "
                type="info"
                variant="tonal"
                density="compact"
                class="mb-4"
              >
                No Google account connected for calendars yet.
              </v-alert>

              <CalendarConnectionCard
                v-for="c in calendarConnections"
                :key="c.id"
                :connection="c"
                :confirm="confirmPrompt"
                :inform="informPrompt"
                :show-message="showMessage"
                :before-connect="saveSettingsInternal"
                :focus="c.id === focusConnectionId"
                @deleted="loadDevices"
              ></CalendarConnectionCard>

              <v-btn
                color="primary"
                variant="tonal"
                prepend-icon="mdi-plus"
                :disabled="!googleClientReady"
                @click="showAddCalendar = true"
              >
                {{
                  calendarConnections.length
                    ? 'Add another Google account'
                    : 'Add Google account'
                }}
              </v-btn>
            </v-card-text>
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

              <h3 class="text-h6 mb-3">Google</h3>
              <v-alert
                type="info"
                variant="tonal"
                class="mb-4"
                density="compact"
              >
                <div class="text-body-2">
                  One OAuth client serves every Google Photos and Google
                  Calendar account. Create a project in
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
                      <th>Version</th>
                      <th>Battery</th>
                      <th class="text-right">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="device in availableDevices" :key="device.id">
                      <td>
                        {{ device.name }}
                        <v-chip
                          v-if="device.auth_required"
                          size="x-small"
                          color="warning"
                          variant="flat"
                          class="ml-2"
                          prepend-icon="mdi-lock-alert"
                          title="The frame refuses the server's password. Click to enter it."
                          @click="promptFramePassword(device)"
                        >
                          Password required
                        </v-chip>
                      </td>
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
                        <span v-if="device.firmware_version">
                          {{ device.firmware_version }}
                        </span>
                        <span v-else class="text-medium-emphasis">&mdash;</span>
                      </td>
                      <td>
                        <span
                          v-if="(device.battery_level ?? -1) >= 0"
                          :title="
                            'Last reported ' +
                            formatReportedAt(device.battery_reported_at)
                          "
                        >
                          <v-icon
                            size="small"
                            :color="
                              device.battery_level! <= 20 ? 'error' : undefined
                            "
                            :icon="batteryIcon(device.battery_level!)"
                          ></v-icon>
                          {{ device.battery_level }}%
                        </span>
                        <span v-else class="text-medium-emphasis">&mdash;</span>
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
                      <td colspan="5" class="text-center text-grey py-4">
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
                      <v-list-item-subtitle v-if="device.auth_required">
                        <v-chip
                          size="x-small"
                          color="warning"
                          variant="flat"
                          prepend-icon="mdi-lock-alert"
                          @click="promptFramePassword(device)"
                        >
                          Password required
                        </v-chip>
                      </v-list-item-subtitle>
                      <v-list-item-subtitle
                        v-if="device.firmware_version"
                        class="text-truncate"
                      >
                        {{ device.firmware_version }}
                      </v-list-item-subtitle>
                      <v-list-item-subtitle
                        v-if="(device.battery_level ?? -1) >= 0"
                        :title="
                          'Last reported ' +
                          formatReportedAt(device.battery_reported_at)
                        "
                      >
                        <v-icon
                          size="x-small"
                          :color="
                            device.battery_level! <= 20 ? 'error' : undefined
                          "
                          :icon="batteryIcon(device.battery_level!)"
                        ></v-icon>
                        {{ device.battery_level }}%
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
                    <!-- The frame refuses the server: say so up front, since
                         nothing saved here reaches it until its password is
                         entered (the field is under Advanced network settings). -->
                    <v-alert
                      v-if="!isAddingDevice && editingDevice.auth_required"
                      type="warning"
                      variant="tonal"
                      density="compact"
                      class="mb-3"
                    >
                      <div class="d-flex flex-wrap align-center ga-2">
                        <span
                          >This frame refuses the password the server has for
                          it, so settings saved here reach it only with its next
                          image fetch.</span
                        >
                        <v-btn
                          size="small"
                          variant="tonal"
                          @click="promptForEditingDevice"
                          >Enter password</v-btn
                        >
                      </div>
                    </v-alert>
                    <!-- Add Device: host, plus a password if the frame needs one -->
                    <div v-if="isAddingDevice" class="mt-2">
                      <v-text-field
                        v-model="editingDevice.host"
                        label="Device Host / IP"
                        variant="outlined"
                        hint="e.g., photoframe.local or 192.168.1.100"
                        persistent-hint
                        autofocus
                      ></v-text-field>
                      <v-checkbox
                        v-model="deviceNeedsPassword"
                        label="This frame requires a password"
                        color="primary"
                        density="compact"
                        class="mt-2"
                        hide-details
                      ></v-checkbox>
                      <v-text-field
                        v-if="deviceNeedsPassword"
                        v-model="deviceHttpPassword"
                        label="Frame password"
                        type="password"
                        maxlength="63"
                        variant="outlined"
                        hint="Set on the frame's own web interface. Without it the frame cannot be read, and placeholder settings are used until you supply it."
                        persistent-hint
                        class="mt-2"
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
                        <!-- Time zone (#128). The frame stores a POSIX TZ
                             rule and applies it with tzset(), so a zone
                             with DST works; the picker names the rule,
                             the advanced field takes any rule verbatim. -->
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-autocomplete
                              v-model="tzZone"
                              :items="tzItems"
                              label="Time zone"
                              variant="outlined"
                              density="compact"
                              auto-select-first
                              :hint="tzHint"
                              persistent-hint
                              @focusin="selectTzSearch"
                            ></v-autocomplete>
                            <div
                              v-if="tzCaveat !== null"
                              class="text-caption text-warning mt-1"
                            >
                              {{ tzCaveat }}
                            </div>
                          </v-col>
                          <v-col
                            cols="12"
                            md="6"
                            class="d-flex flex-wrap align-center ga-2"
                          >
                            <v-btn
                              size="small"
                              variant="tonal"
                              :disabled="
                                browserZoneRule === null ||
                                deviceConfig.timezone === browserZoneRule
                              "
                              @click="useBrowserZone"
                            >
                              Use this browser's time zone
                            </v-btn>
                            <span
                              v-if="browserZone !== null"
                              class="text-caption text-medium-emphasis"
                            >
                              {{ browserZone
                              }}{{
                                browserZoneRule === null
                                  ? ' is not in the list'
                                  : ''
                              }}
                            </span>
                          </v-col>
                        </v-row>
                        <v-row>
                          <v-col cols="12" md="6">
                            <v-checkbox
                              v-model="tzRuleShown"
                              label="Advanced: POSIX TZ rule"
                              density="compact"
                              hide-details
                              :disabled="tzChoice.kind !== 'zone'"
                            ></v-checkbox>
                            <v-text-field
                              v-if="tzRuleShown"
                              v-model="deviceConfig.timezone"
                              label="POSIX TZ rule"
                              variant="outlined"
                              density="compact"
                              spellcheck="false"
                              autocapitalize="off"
                              :error-messages="tzProblem ? [tzProblem] : []"
                              hint="Any rule the frame's tzset() takes, e.g. EST5EDT,M3.2.0,M11.1.0. Fixed offsets count hours west: UTC-8 is UTC+8."
                              persistent-hint
                            ></v-text-field>
                          </v-col>
                        </v-row>
                        <!-- Advanced network settings (#43), collapsed by
                             default. Static IP / DNS render only when the
                             firmware reports ip_mode; NTP exists everywhere. -->
                        <v-expansion-panels class="mt-2" variant="accordion">
                          <v-expansion-panel
                            title="Advanced network settings"
                            elevation="0"
                          >
                            <v-expansion-panel-text>
                              <v-row>
                                <v-col cols="12" md="6">
                                  <v-text-field
                                    v-model="deviceConfig.ntp_server"
                                    label="NTP Server"
                                    variant="outlined"
                                    density="compact"
                                    hint="e.g., pool.ntp.org, or a local IP"
                                    persistent-hint
                                  ></v-text-field>
                                </v-col>
                                <v-col
                                  v-if="deviceSupportsStaticIp"
                                  cols="12"
                                  md="6"
                                >
                                  <v-select
                                    v-model="deviceConfig.ip_mode"
                                    :items="[
                                      {
                                        title: 'Automatic (DHCP)',
                                        value: 'dhcp',
                                      },
                                      { title: 'Static IP', value: 'static' },
                                    ]"
                                    label="IP Configuration"
                                    variant="outlined"
                                    density="compact"
                                    hint="Applied on the device's next boot / wake"
                                    persistent-hint
                                  ></v-select>
                                </v-col>
                              </v-row>
                              <v-row
                                v-if="
                                  deviceSupportsStaticIp &&
                                  deviceConfig.ip_mode === 'static'
                                "
                              >
                                <v-col cols="12" md="4">
                                  <v-text-field
                                    v-model="deviceConfig.static_ip"
                                    label="IP Address"
                                    variant="outlined"
                                    density="compact"
                                    placeholder="192.168.1.50"
                                  ></v-text-field>
                                </v-col>
                                <v-col cols="12" md="4">
                                  <v-text-field
                                    v-model="deviceConfig.static_netmask"
                                    label="Netmask"
                                    variant="outlined"
                                    density="compact"
                                  ></v-text-field>
                                </v-col>
                                <v-col cols="12" md="4">
                                  <v-text-field
                                    v-model="deviceConfig.static_gateway"
                                    label="Gateway"
                                    variant="outlined"
                                    density="compact"
                                    placeholder="192.168.1.1"
                                  ></v-text-field>
                                </v-col>
                              </v-row>
                              <v-row v-if="deviceSupportsStaticIp">
                                <v-col cols="12" md="6">
                                  <v-text-field
                                    v-model="deviceConfig.dns_server"
                                    label="DNS Server"
                                    variant="outlined"
                                    density="compact"
                                    :hint="
                                      deviceConfig.ip_mode === 'static'
                                        ? 'Leave empty to use the gateway'
                                        : 'Optional override; leave empty for DHCP-provided DNS'
                                    "
                                    persistent-hint
                                  ></v-text-field>
                                </v-col>
                              </v-row>
                              <!-- Frame password (#130), where the frame's own web
                                   interface keeps it. Two different things: the
                                   field records the password the frame already
                                   has; the card below changes the frame itself. -->
                              <div class="text-subtitle-2 mt-6">
                                Frame password
                              </div>
                              <v-row>
                                <v-col cols="12" md="6">
                                  <v-text-field
                                    v-model="deviceHttpPassword"
                                    :label="
                                      editingDevice.http_password_set
                                        ? 'Frame password (set — blank keeps it)'
                                        : 'Frame password (only if enabled on the frame)'
                                    "
                                    type="password"
                                    maxlength="63"
                                    variant="outlined"
                                    density="compact"
                                    hint="The password the frame already has, so this server can reach it. Saving it here does not change the frame."
                                    persistent-hint
                                  ></v-text-field>
                                  <v-btn
                                    v-if="editingDevice.http_password_set"
                                    size="small"
                                    variant="text"
                                    color="error"
                                    class="mt-1"
                                    :disabled="framePasswordBusy"
                                    @click="clearDeviceHttpPassword"
                                    >Forget password</v-btn
                                  >
                                </v-col>
                              </v-row>
                              <!-- Changes the frame itself, unlike the field above,
                                   which only tells this server what the frame has. -->
                              <v-row>
                                <v-col cols="12">
                                  <v-card variant="outlined" class="pa-3">
                                    <div class="text-subtitle-2">
                                      Password on the frame
                                    </div>
                                    <div
                                      class="text-body-2 text-medium-emphasis"
                                    >
                                      {{
                                        editingDevice.http_password_set
                                          ? 'Changes the password the frame itself requires.'
                                          : 'Makes the frame itself require a password.'
                                      }}
                                      The server signs in to the frame with the
                                      password it has stored, sets the new one
                                      on the frame, then stores that too. If the
                                      frame does not accept it, nothing changes.
                                    </div>
                                    <div
                                      v-if="framePasswordHostUnsaved"
                                      class="text-caption text-warning mt-2"
                                    >
                                      Save the new Host / IP first: this acts on
                                      the frame at the saved address.
                                    </div>
                                    <div
                                      v-if="framePasswordTypedUnsaved"
                                      class="text-caption text-warning mt-2"
                                    >
                                      Save the password typed above first: the
                                      server signs in to the frame with the
                                      saved one.
                                    </div>
                                    <div
                                      v-if="!framePasswordFormOpen"
                                      class="d-flex flex-wrap ga-2 mt-2"
                                    >
                                      <v-btn
                                        size="small"
                                        variant="tonal"
                                        :disabled="
                                          framePasswordBusy ||
                                          !framePasswordReady
                                        "
                                        @click="openFramePasswordForm"
                                        >{{
                                          editingDevice.http_password_set
                                            ? 'Change password on the frame'
                                            : 'Set a password on the frame'
                                        }}</v-btn
                                      >
                                      <v-btn
                                        v-if="editingDevice.http_password_set"
                                        size="small"
                                        variant="tonal"
                                        color="error"
                                        :loading="framePasswordBusy"
                                        :disabled="!framePasswordReady"
                                        @click="turnOffFramePassword"
                                        >Turn off password on the frame</v-btn
                                      >
                                    </div>
                                    <div v-else class="mt-2">
                                      <v-row dense>
                                        <v-col cols="12" md="6">
                                          <v-text-field
                                            v-model="newFramePassword"
                                            label="New frame password"
                                            type="password"
                                            autocomplete="new-password"
                                            variant="outlined"
                                            density="compact"
                                            hide-details="auto"
                                          ></v-text-field>
                                        </v-col>
                                        <v-col cols="12" md="6">
                                          <v-text-field
                                            v-model="newFramePasswordRepeat"
                                            label="Repeat new frame password"
                                            type="password"
                                            autocomplete="new-password"
                                            variant="outlined"
                                            density="compact"
                                            hide-details="auto"
                                            :error-messages="
                                              framePasswordTouched
                                                ? framePasswordProblem || []
                                                : []
                                            "
                                          ></v-text-field>
                                        </v-col>
                                      </v-row>
                                      <div class="d-flex flex-wrap ga-2 mt-2">
                                        <v-btn
                                          size="small"
                                          color="primary"
                                          :loading="framePasswordBusy"
                                          :disabled="!framePasswordReady"
                                          @click="applyFramePassword"
                                          >Change on the frame</v-btn
                                        >
                                        <v-btn
                                          size="small"
                                          variant="text"
                                          :disabled="framePasswordBusy"
                                          @click="closeFramePasswordForm"
                                          >Cancel</v-btn
                                        >
                                      </div>
                                    </div>
                                    <v-alert
                                      v-if="framePasswordNote"
                                      :type="framePasswordNote.type"
                                      variant="tonal"
                                      density="compact"
                                      class="mt-3"
                                      >{{ framePasswordNote.text }}</v-alert
                                    >
                                  </v-card>
                                </v-col>
                              </v-row>
                            </v-expansion-panel-text>
                          </v-expansion-panel>
                        </v-expansion-panels>
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
                              v-model="deviceSourceKey"
                              :items="deviceSourceItems"
                              item-title="title"
                              item-value="value"
                              label="Image Source"
                              variant="outlined"
                              density="compact"
                              hide-details
                              class="mb-2 ml-8"
                              :menu-props="{ maxHeight: 420 }"
                              :disabled="!deviceConfig.auto_rotate"
                            >
                              <template #selection="{ item }">
                                <span class="text-truncate">{{
                                  item.raw.title
                                }}</span>
                                <span
                                  v-if="
                                    item.raw.group &&
                                    item.raw.group !== item.raw.title
                                  "
                                  class="text-caption text-grey ml-2"
                                  >{{ item.raw.group }}</span
                                >
                              </template>
                              <template #item="{ props: itemProps, item }">
                                <v-list-item
                                  v-bind="itemProps"
                                  :title="item.raw.title"
                                  :subtitle="item.raw.subtitle"
                                  :prepend-icon="item.raw.icon"
                                >
                                  <template
                                    v-if="item.raw.notConnected"
                                    #append
                                  >
                                    <v-chip size="x-small" variant="tonal"
                                      >Not connected</v-chip
                                    >
                                  </template>
                                </v-list-item>
                              </template>
                            </v-select>

                            <!-- Per-frame album selection for the chosen source -->
                            <div
                              v-if="
                                useThisServer && selectedSource === 'gallery'
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Albums for this frame:
                              </div>
                              <AlbumPicker
                                v-model="deviceGalleryAlbumIds"
                                all-label="All gallery photos"
                                :albums="deviceGalleryAlbumOptions"
                                label-field="name"
                                :max-height="220"
                                :disabled="!deviceConfig.auto_rotate"
                                empty-text="No gallery albums yet. Create them under Settings → Gallery → Albums."
                              ></AlbumPicker>
                            </div>

                            <div
                              v-if="
                                useThisServer &&
                                selectedSource === 'immich' &&
                                hasConnections('immich')
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Albums for this frame:
                              </div>
                              <AlbumPicker
                                v-model="deviceImmichAlbumIds"
                                all-label="All synced Immich photos"
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
                                hasConnections('synology_photos')
                              "
                              class="mb-2 ml-8"
                            >
                              <div class="text-caption text-grey mb-1">
                                Albums for this frame:
                              </div>
                              <AlbumPicker
                                v-model="deviceSynologyAlbumIds"
                                all-label="All synced Synology photos"
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
                                Topics for this frame:
                              </div>
                              <AlbumPicker
                                v-model="deviceUnsplashAlbumIds"
                                all-label="All synced Unsplash photos"
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
                                Topics for this frame:
                              </div>
                              <AlbumPicker
                                v-model="devicePexelsAlbumIds"
                                all-label="All synced Pexels photos"
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
                          <!-- One format for today's date and the photo's date. -->
                          <v-select
                            v-if="
                              editingDevice.show_date ||
                              editingDevice.show_photo_date
                            "
                            v-model="editingDevice.date_format"
                            :items="dateFormatOptions"
                            item-title="label"
                            item-value="value"
                            label="Date Format"
                            variant="outlined"
                            density="compact"
                            :hint="
                              editingDevice.show_photo_date &&
                              !editingDevice.date_format
                                ? 'With the default, the photo date is shown as Jan 02, 2006.'
                                : ''
                            "
                            :persistent-hint="
                              editingDevice.show_photo_date &&
                              !editingDevice.date_format
                            "
                            :hide-details="
                              !(
                                editingDevice.show_photo_date &&
                                !editingDevice.date_format
                              )
                            "
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
                            :disabled="calendarAccounts.length > 0"
                            location="top"
                            text="Add a Google account under Settings → Calendar first"
                          >
                            <template #activator="{ props }">
                              <div v-bind="props" class="d-inline-flex">
                                <v-checkbox
                                  v-model="editingDevice.show_calendar"
                                  label="Show Google Calendar Events"
                                  color="primary"
                                  hide-details
                                  class="mt-2 mb-1"
                                  :disabled="calendarAccounts.length === 0"
                                ></v-checkbox>
                              </div>
                            </template>
                          </v-tooltip>
                          <template
                            v-if="
                              editingDevice.show_calendar &&
                              calendarAccounts.length > 0
                            "
                          >
                            <v-select
                              v-model="editingDevice.calendar_connection_id"
                              :items="calendarAccountOptions"
                              item-title="title"
                              item-value="value"
                              label="Google account"
                              variant="outlined"
                              density="compact"
                              class="mt-2"
                              @update:model-value="
                                editingDevice.calendar_id = ''
                              "
                            ></v-select>
                            <v-select
                              v-model="editingDevice.calendar_id"
                              :items="deviceCalendars"
                              item-title="summary"
                              item-value="id"
                              label="Select Calendar"
                              variant="outlined"
                              density="compact"
                              class="mt-2"
                              :loading="deviceCalendarsLoading"
                              :hint="deviceCalendarsHint"
                              :persistent-hint="!!deviceCalendarsHint"
                            ></v-select>
                          </template>
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

                        <v-divider class="my-4" />

                        <v-switch
                          v-model="deviceConfig.low_battery_indicator"
                          label="Show a low battery badge on the picture"
                          color="primary"
                          hint="A small empty-battery icon in the top-right corner of the picture, drawn when the battery is at or below the threshold. Never shown on USB power or while charging."
                          persistent-hint
                        />
                        <v-slider
                          v-model="deviceConfig.low_battery_threshold"
                          :disabled="!deviceConfig.low_battery_indicator"
                          label="Low battery threshold"
                          :min="5"
                          :max="50"
                          :step="5"
                          thumb-label="always"
                          color="primary"
                          class="mt-6"
                        >
                          <template #thumb-label="{ modelValue }">
                            {{ modelValue }}%
                          </template>
                        </v-slider>
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
                        <!-- Layout first, matching the device web app -->
                        <v-row>
                          <v-col cols="12" md="4">
                            <v-select
                              v-model="deviceProcessing.scaleMode"
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
                              label="Scale Mode"
                              variant="outlined"
                              density="compact"
                            ></v-select>
                          </v-col>
                          <v-col
                            v-if="deviceProcessing.scaleMode === 'fit'"
                            cols="12"
                            md="4"
                          >
                            <v-select
                              v-model="deviceProcessing.backgroundColor"
                              :items="[
                                { title: 'White', value: 'white' },
                                { title: 'Black', value: 'black' },
                              ]"
                              label="Background Color"
                              variant="outlined"
                              density="compact"
                            ></v-select>
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
                      :disabled="framePasswordBusy"
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
import { inUseMessage } from '../utils/inUse';
import {
  CALENDAR_SOURCE,
  CONNECTION_SOURCES,
  SOURCE_ICONS,
  SOURCE_LABELS,
  connectionSubtitle,
  useConnectionsStore,
  type Connection,
} from '../stores/connections';
import { useGalleryStore } from '../stores/gallery';
import { useAuthStore } from '../stores/auth';
import { useSnackbar } from '../composables/useSnackbar';
import { useFramePasswordPrompt } from '../composables/useFramePasswordPrompt';
import { isFrameAuthRefusal } from '../utils/frameAuth';
import {
  api,
  listDevices,
  addDevice,
  deleteDevice,
  updateDevice,
  setDeviceHttpPassword,
  changeFramePassword,
  refreshDevice,
  type Device,
  createURLSource,
  updateURLSource,
  listURLSources,
  deleteURLSource,
  getDeviceConfig,
  updateDeviceConfig,
} from '../api';
import Gallery from './Gallery.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import SourcePicker from './SourcePicker.vue';
import AddConnectionDialog from './AddConnectionDialog.vue';
import CalendarConnectionCard from './CalendarConnectionCard.vue';
import { cardComponentFor } from './connectionCards';
import { newFramePasswordProblem } from '../utils/framePassword';
import { getApiError } from '../utils/errors';
import AlbumPicker from './AlbumPicker.vue';
import SecurityTab from './SecurityTab.vue';
import RotationSchedule from './RotationSchedule.vue';
import { intervalToCron, cronToInterval, isValidCron } from '../utils/cron';
import {
  DEFAULT_TZ_RULE,
  TIMEZONE_NAMES,
  browserTimeZone,
  choiceForRule,
  ruleForZone,
  tzRuleProblem,
  zoneCaveat,
} from '../utils/timezone';

const { smAndDown } = useDisplay(); // true on phones / small tablets
const store = useSettingsStore();
const connectionsStore = useConnectionsStore();
// Whether a connection-backed source has at least one connection; drives the
// per-frame album pickers.
const hasConnections = (source: string) =>
  connectionsStore.bySource(source).length > 0;

// The card of the connection being viewed. Its album or topic selection
// lives in the card; Settings only asks it to offer a sync when the user
// moves to another source.
const activeCard = ref<any>(null);
const showAddConnection = ref(false);
// The Google connection the OAuth callback just connected (?connection=),
// so its card is scrolled into view.
const focusConnectionId = ref<number | undefined>(undefined);
// Per-device gallery album bindings (the gallery's own folders).
const deviceGalleryAlbumIds = ref<number[]>([]);
const deviceGallerySyncedAlbums = ref<any[]>([]);
// Per-device Immich album bindings (Part B)
const deviceImmichAlbumIds = ref<number[]>([]);
const deviceImmichSyncedAlbums = ref<any[]>([]);
// Per-device Synology album bindings (Part C)
const deviceSynologyAlbumIds = ref<number[]>([]);
const deviceSynologySyncedAlbums = ref<any[]>([]);
// Per-device topic bindings (Unsplash / Pexels topics are albums too).
const deviceUnsplashAlbumIds = ref<number[]>([]);
const deviceUnsplashSyncedAlbums = ref<any[]>([]);
const devicePexelsAlbumIds = ref<number[]>([]);
const devicePexelsSyncedAlbums = ref<any[]>([]);
const galleryStore = useGalleryStore();
// Only to know whether the session is still on when a load finishes: a
// list requested before a logout must not open the password prompt after.
const authStore = useAuthStore();
const activeMainTab = ref('devices');
// The source picked at the top of the page: a built-in one ('gallery',
// 'url', 'ai_generation') or one connection ('conn:<id>').
const sourceKey = ref('gallery');
const activeConnectionId = computed(() => {
  const m = /^conn:(\d+)$/.exec(sourceKey.value);
  return m ? Number(m[1]) : null;
});
const activeConnection = computed(() => {
  if (activeConnectionId.value == null) return undefined;
  const c = connectionsStore.byId(activeConnectionId.value);
  // Only photo sources are viewed here; a calendar account is not one.
  return c && CONNECTION_SOURCES.includes(c.source) ? c : undefined;
});
// Props only the Google card takes (its OAuth round trip).
const activeCardProps = computed(() =>
  activeConnection.value?.source === 'google_photos'
    ? {
        beforeConnect: saveSettingsInternal,
        focus: activeConnection.value.id === focusConnectionId.value,
      }
    : {}
);
const sourceHasGallery = computed(() => sourceKey.value === 'gallery');
const confirmDialog = ref();
// Passed to the connection cards so they ask through the same dialog.
const confirmPrompt = (message: string): Promise<boolean> =>
  confirmDialog.value.open(message);
// A notice with only OK: what could not be done and why.
const informPrompt = (message: string, title?: string): Promise<void> =>
  confirmDialog.value.inform(message, title);

// Image Source Binding State
const useThisServer = ref(true);
// Write-only input for a frame whose own HTTP API needs a password. Never
// populated from the server, which only reports http_password_set.
const deviceHttpPassword = ref('');
// Reveals the password input when adding a frame. Most frames are open, so
// the field stays out of the way until it is actually needed.
const deviceNeedsPassword = ref(false);
watch(deviceNeedsPassword, (needs) => {
  if (!needs) deviceHttpPassword.value = '';
});

// The prompt for the frame's password, shared with the device list and the
// gallery. promptIfNeeded asks on its own when the list loads with a
// refusing frame; promptFramePassword asks for one device on request.
const { promptFramePassword, promptIfNeeded } = useFramePasswordPrompt();

// mirrorFramePasswordState copies what the server reports after a password
// was stored or forgotten onto both copies of the device: the list item
// (the badge) and the dialog's (its notice and field labels).
function mirrorFramePasswordState(
  id: number,
  res: {
    http_password_set: boolean;
    auth_required: boolean;
    auth_failed_at: string | null;
  }
) {
  for (const target of [
    availableDevices.value.find((d: Device) => d.id === id),
    editingDevice.id === id ? editingDevice : undefined,
  ]) {
    if (!target) continue;
    target.http_password_set = res.http_password_set;
    target.auth_required = res.auth_required;
    target.auth_failed_at = res.auth_failed_at;
  }
}

// promptForEditingDevice asks for the password of the device in the dialog.
// The list item is what the prompt updates, so the dialog's copy is brought
// in step afterwards.
async function promptForEditingDevice() {
  const id = editingDevice.id;
  if (!id) return { saved: false as const };
  const listed = availableDevices.value.find((d: Device) => d.id === id);
  const result = await promptFramePassword(listed ?? (editingDevice as Device));
  if (listed && editingDevice.id === id) {
    editingDevice.http_password_set = listed.http_password_set;
    editingDevice.auth_required = listed.auth_required;
    editingDevice.auth_failed_at = listed.auth_failed_at;
  }
  return result;
}

// reflectFrameRefusal shows a refusal the server has just recorded -- after
// a sync or push it refused -- in the list and in the dialog's copy of the
// device, before the user is asked for the password: the chip and the
// notice then stay up if the answer is "Not now".
async function reflectFrameRefusal(id: number) {
  await loadDevices();
  const listed = availableDevices.value.find((d: Device) => d.id === id);
  if (listed && editingDevice.id === id) {
    editingDevice.http_password_set = listed.http_password_set;
    editingDevice.auth_required = listed.auth_required;
    editingDevice.auth_failed_at = listed.auth_failed_at;
  }
}

async function clearDeviceHttpPassword() {
  if (!editingDevice.id) return;
  try {
    const res = await setDeviceHttpPassword(editingDevice.id, '');
    // Keep the list in sync too, or reopening the device without saving
    // would still show a stored password.
    mirrorFramePasswordState(editingDevice.id, res);
    deviceHttpPassword.value = '';
    showMessage(
      res.verified
        ? 'Forgot the stored frame password. The frame does not ask for one.'
        : `Forgot the stored frame password. ${res.warning ?? ''}`.trim()
    );
  } catch (e: unknown) {
    showMessage(getApiError(e, 'Could not forget the password.'), true);
  }
}

// "Password on the frame": changes the frame itself, through the server.
const framePasswordFormOpen = ref(false);
const newFramePassword = ref('');
const newFramePasswordRepeat = ref('');
// Mismatch errors only once the user has tried to apply, not while typing.
const framePasswordTouched = ref(false);
const framePasswordBusy = ref(false);
const framePasswordNote = ref<{
  type: 'success' | 'warning' | 'error';
  text: string;
} | null>(null);
// The server acts on the device's SAVED host. With an unsaved edit of the
// Host field, the action would hit a different frame than the one shown.
const framePasswordHostUnsaved = computed(() => {
  const saved = availableDevices.value.find(
    (d: Device) => d.id === editingDevice.id
  );
  return !!saved && saved.host !== editingDevice.host;
});
// A password typed into "Frame password" but not saved yet: the change
// would sign in with the old stored one instead (a likely wrong guess
// against the frame), and a later Save would overwrite the new one.
const framePasswordTypedUnsaved = computed(
  () => deviceHttpPassword.value !== ''
);
// Not while the dialog is saving either: its Save may store a typed
// password, which must not interleave with a change on the frame. (Save is
// disabled during a change, for the same reason.)
const framePasswordReady = computed(
  () =>
    !!editingDevice.host &&
    !framePasswordHostUnsaved.value &&
    !framePasswordTypedUnsaved.value &&
    !savingDeviceConfig.value
);
const framePasswordProblem = computed(() =>
  newFramePasswordProblem(newFramePassword.value, newFramePasswordRepeat.value)
);

// Bumped each time the dialog opens on a device, so a request still in
// flight from an earlier opening -- of this device or another -- leaves the
// new one alone.
let framePasswordGeneration = 0;

function resetFramePasswordState() {
  framePasswordBusy.value = false;
  framePasswordFormOpen.value = false;
  newFramePassword.value = '';
  newFramePasswordRepeat.value = '';
  framePasswordTouched.value = false;
  framePasswordNote.value = null;
}

function openFramePasswordForm() {
  resetFramePasswordState();
  framePasswordFormOpen.value = true;
}

function closeFramePasswordForm() {
  const note = framePasswordNote.value;
  resetFramePasswordState();
  framePasswordNote.value = note;
}

async function sendFramePassword(password: string) {
  // The dialog can be closed and reopened while this is in flight: the
  // result belongs to the device it was sent for, and to that opening.
  const deviceId = editingDevice.id;
  if (!deviceId) return false;
  const generation = framePasswordGeneration;
  const stillOpen = () => framePasswordGeneration === generation;
  framePasswordBusy.value = true;
  framePasswordNote.value = null;
  try {
    const res = await changeFramePassword(
      deviceId,
      password,
      editingDevice.host ?? ''
    );
    if (!stillOpen()) {
      // A later opening may have changed it again since: ask the server,
      // which saw the changes in order, rather than trust this result.
      await loadDevices();
      // And if that later opening is of this same device, its flag too.
      const fresh = availableDevices.value.find(
        (d: Device) => d.id === deviceId
      );
      if (fresh && editingDevice.id === deviceId) {
        editingDevice.http_password_set = fresh.http_password_set;
      }
      return false;
    }
    // The change also cleared the refusal flag, the old password being
    // gone with whatever it was refused for: the badge goes with it.
    mirrorFramePasswordState(deviceId, res);
    const others =
      password === ''
        ? 'The frame no longer requires a password. The Home Assistant integration and the mobile app keep working; you can remove the password from them.'
        : 'Give the new password to the Home Assistant integration and the mobile app too, or they can no longer reach the frame.';
    framePasswordNote.value = res.verified
      ? { type: 'success', text: `Done. ${others}` }
      : {
          type: 'warning',
          text: `${res.warning || 'The frame accepted the change, but it could not be confirmed.'} ${others}`,
        };
    return true;
  } catch (e: unknown) {
    if (stillOpen()) {
      framePasswordNote.value = {
        type: 'error',
        text: getApiError(e, 'Could not change the password on the frame.'),
      };
    } else {
      showMessage(
        getApiError(e, 'Could not change the password on the frame.'),
        true
      );
    }
    return false;
  } finally {
    if (stillOpen()) framePasswordBusy.value = false;
  }
}

async function applyFramePassword() {
  framePasswordTouched.value = true;
  if (framePasswordProblem.value) return;
  // false as well when the dialog moved on to another device meanwhile.
  const ok = await sendFramePassword(newFramePassword.value);
  if (ok) closeFramePasswordForm();
}

async function turnOffFramePassword() {
  const ok = await confirmDialog.value.open(
    'The frame will no longer ask for a password: anyone on your network can then open its web page and change its settings. The server stops sending the password.',
    'Turn off password on the frame?'
  );
  if (!ok) return;
  await sendFramePassword('');
}

// What the frame draws from, as one dropdown value: a built-in source by
// name, one connection ('conn:<id>'), or every connection of a provider
// ('any:<source>'). The device record stores it as source +
// image_connection_id (null = any).
const deviceSourceKey = ref('gallery');
const BUILT_IN_SOURCES = [
  { value: 'gallery', title: 'Gallery', icon: 'mdi-image-multiple' },
  { value: 'url_proxy', title: 'URL Proxy', icon: 'mdi-link-variant' },
  { value: 'ai_generation', title: 'AI Generation', icon: 'mdi-creation' },
  {
    value: 'fractal',
    title: 'Fractal (Mandelbrot zoom)',
    icon: 'mdi-infinity',
  },
  {
    value: 'dla',
    title: 'DLA (diffusion-limited aggregation)',
    icon: 'mdi-snowflake',
  },
];
type DeviceSourceItem = {
  value: string;
  title: string;
  subtitle?: string;
  icon?: string;
  group?: string;
  notConnected?: boolean;
  type?: 'subheader' | 'divider';
};
const deviceSourceKeyFor = (
  source: string,
  connectionId: number | null | undefined
) => {
  if (!CONNECTION_SOURCES.includes(source as any)) return source;
  // A binding to a connection this dashboard no longer knows (deleted
  // since the device list loaded) counts as unbound.
  if (connectionId && connectionsStore.byId(connectionId)) {
    return `conn:${connectionId}`;
  }
  // Unbound on a provider with one connection: that connection, which is
  // what the frame draws from anyway and what the dropdown can show.
  const conns = connectionsStore.bySource(source);
  if (conns.length === 1) return `conn:${conns[0].id}`;
  return `any:${source}`;
};
// The source type the key stands for ('' until a device is loaded).
const selectedSource = computed(() => {
  const key = deviceSourceKey.value;
  if (key.startsWith('conn:')) {
    return connectionsStore.byId(Number(key.slice(5)))?.source || '';
  }
  if (key.startsWith('any:')) return key.slice(4);
  return key;
});
// The connection the key names, or null for a built-in or 'any'.
const selectedConnectionId = computed(() => {
  const key = deviceSourceKey.value;
  return key.startsWith('conn:') ? Number(key.slice(5)) || null : null;
});
// Built-ins first, then one group per provider that has connections. A
// provider with several also offers 'Any', which is what a frame bound
// before connections were plural still has.
const deviceSourceItems = computed<DeviceSourceItem[]>(() => {
  const out: DeviceSourceItem[] = [
    { type: 'subheader', title: 'Built-in', value: '' },
    ...BUILT_IN_SOURCES,
  ];
  for (const source of CONNECTION_SOURCES) {
    const conns = connectionsStore.bySource(source);
    const current = deviceSourceKey.value === `any:${source}`;
    if (conns.length === 0 && !current) continue;
    const label = SOURCE_LABELS[source];
    out.push({ type: 'divider', title: '', value: '' });
    out.push({ type: 'subheader', title: label, value: '' });
    for (const c of conns) {
      out.push({
        value: `conn:${c.id}`,
        title: c.name,
        subtitle: connectionSubtitle(c),
        icon: SOURCE_ICONS[source],
        group: label,
        notConnected: !c.connected,
      });
    }
    if (conns.length > 1 || current) {
      out.push({
        value: `any:${source}`,
        title: conns.length
          ? `Any ${label} connection`
          : `${label} (no connection yet)`,
        subtitle: conns.length
          ? `Photos of all ${conns.length} connections`
          : 'Add one with + above the photos',
        icon: SOURCE_ICONS[source],
        group: label,
      });
    }
  }
  return out;
});

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

// Calendar accounts (google_calendar connections) and, in the device
// dialog, the calendars of the account the frame uses.
const showAddCalendar = ref(false);
const googleClientReady = computed(
  () => !!(form.google_client_id && form.google_client_secret)
);
const calendarConnections = computed(() =>
  connectionsStore.bySource(CALENDAR_SOURCE)
);
const calendarAccounts = computed(() =>
  calendarConnections.value.filter((c) => c.connected)
);
const calendarAccountOptions = computed(() =>
  calendarAccounts.value.map((c) => ({
    value: c.id,
    title: c.config.email ? `${c.name} (${c.config.email})` : c.name,
  }))
);
const deviceCalendars = ref<any[]>([]);
const deviceCalendarsLoading = ref(false);
const deviceCalendarsHint = ref('');
// Only the latest request may fill the list: switching accounts while the
// previous account's calendars are still loading must not leave those on
// screen under the new account.
let deviceCalendarsRequest = 0;
const loadDeviceCalendars = async (accountId: number | null | undefined) => {
  const request = ++deviceCalendarsRequest;
  deviceCalendars.value = [];
  deviceCalendarsHint.value = '';
  if (!accountId) {
    deviceCalendarsLoading.value = false;
    return;
  }
  deviceCalendarsLoading.value = true;
  try {
    const calendars = await connectionsStore.calendars(accountId);
    if (request !== deviceCalendarsRequest) return;
    deviceCalendars.value = calendars;
  } catch (e: any) {
    if (request !== deviceCalendarsRequest) return;
    deviceCalendarsHint.value =
      e.response?.status === 403
        ? 'This account was signed in without calendar access; sign in again under Settings → Calendar.'
        : 'Could not load the calendars of this account.';
  } finally {
    if (request === deviceCalendarsRequest)
      deviceCalendarsLoading.value = false;
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
  // The POSIX TZ rule the frame stores, kept verbatim (#128). It is never
  // reduced to an offset: a DST rule used to load as 0 and save as "UTC0".
  timezone: DEFAULT_TZ_RULE,
  ntp_server: 'pool.ntp.org',
  // Advanced network settings (firmware #43)
  ip_mode: 'dhcp',
  static_ip: '',
  static_netmask: '255.255.255.0',
  static_gateway: '',
  dns_server: '',
  deep_sleep_enabled: true,
  low_battery_indicator: true,
  low_battery_threshold: 10,
  ha_url: '',
  openai_api_key: '',
  google_api_key: '',
});

// Time zone picker (#128). deviceConfig.timezone is the truth; the picker
// only names it. Picking a zone writes that zone's rule; a rule no zone
// yields shows as a synthesized "Fixed offset" or "Custom rule" entry.
const browserZone = browserTimeZone();
const browserZoneRule = browserZone === null ? null : ruleForZone(browserZone);
// The zone the user picked, kept on show while its rule is still the one
// set: many zones share a rule, and Europe/Berlin must not turn into
// Africa/Ceuta the moment it is chosen. Null after a load, when the
// browser's zone is the best guess for a rule the frame already had.
const tzPicked = ref<string | null>(null);
const tzChoice = computed(() =>
  choiceForRule(deviceConfig.timezone, [tzPicked.value, browserZone])
);
// The picker value for a rule the table cannot name. Not an IANA name.
const TZ_UNNAMED = '(unnamed)';
const tzItems = computed(() => {
  const items = TIMEZONE_NAMES.map((name) => ({ title: name, value: name }));
  const choice = tzChoice.value;
  if (choice.kind === 'fixed') {
    items.unshift({ title: choice.label, value: TZ_UNNAMED });
  } else if (choice.kind === 'custom') {
    items.unshift({ title: 'Custom rule', value: TZ_UNNAMED });
  }
  return items;
});
const tzZone = computed({
  get: () => {
    const choice = tzChoice.value;
    return choice.kind === 'zone' ? choice.name : TZ_UNNAMED;
  },
  set: (name: string | null) => {
    // Re-picking the synthesized entry, or clearing, changes nothing.
    const rule = name === null ? null : ruleForZone(name);
    if (rule === null) return;
    tzPicked.value = name;
    deviceConfig.timezone = rule;
  },
});
const tzProblem = computed(() => tzRuleProblem(deviceConfig.timezone));
const tzHint = computed(() =>
  tzProblem.value === null
    ? `POSIX rule ${deviceConfig.timezone}. The rotation schedule runs in this zone.`
    : 'The rotation schedule runs in this zone.'
);
// Where the frame will not keep the named zone's time exactly, say so.
const tzCaveat = computed(() => {
  const choice = tzChoice.value;
  return choice.kind === 'zone' ? zoneCaveat(choice.name) : null;
});
// The advanced field is opt-in while the picker can name the rule, and
// stays open when it cannot: that is the only place such a rule is editable.
// A load opens it for real when the rule is unnamed, not just by the
// fallback: a rule being typed can pass through a zone's ("GMT0" on the way
// to "GMT0BST,..."), and the fallback alone would unmount the field there.
const tzRuleOpened = ref(false);
const tzRuleShown = computed({
  get: () => tzRuleOpened.value || tzChoice.value.kind !== 'zone',
  set: (shown: boolean) => {
    tzRuleOpened.value = shown;
  },
});
const useBrowserZone = () => {
  if (browserZoneRule === null) return;
  tzPicked.value = browserZone;
  deviceConfig.timezone = browserZoneRule;
};
// On focus Vuetify puts the shown name in the search box, unselected, so
// typing would append to "Africa/Ceuta" and match nothing. Select it, so
// typing replaces it; the name it filled in lands in the box a tick later.
const selectTzSearch = (e: FocusEvent) => {
  const el = e.target;
  if (el instanceof HTMLInputElement) nextTick(() => el.select());
};

// Whether the device firmware understands rotate_cron. Old firmware reports
// only rotate_interval and silently ignores a cron schedule. Best-effort:
// derived from the loaded config, which is the server's stored copy.
const deviceSupportsCron = ref(true);

// Whether the device firmware supports static IP / DNS override (#43).
// Detected by key presence in the stored config; old firmware must render
// (and be sent) exactly what it did before.
const deviceSupportsStaticIp = ref(false);

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
  scaleMode: 'cover',
  backgroundColor: 'white',
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
      deviceSourceKey.value = deviceSourceKeyFor(
        editingDevice.source,
        editingDevice.image_connection_id
      );
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
          deviceSourceKey.value = deviceSourceKeyFor(match[1], null);
        }
      }
    }

    Object.assign(deviceConfig, {
      sleep_schedule_enabled: cfg.sleep_schedule_enabled ?? false,
      display_orientation:
        cfg.display_orientation ?? deviceConfig.display_orientation,
      display_rotation_deg: cfg.display_rotation_deg ?? 180,
      deep_sleep_enabled: cfg.deep_sleep_enabled ?? true,
      low_battery_indicator: cfg.low_battery_indicator ?? true,
      low_battery_threshold: cfg.low_battery_threshold ?? 10,
      ha_url: cfg.ha_url ?? '',
      ntp_server: cfg.ntp_server ?? 'pool.ntp.org',
      ip_mode: cfg.ip_mode ?? 'dhcp',
      static_ip: cfg.static_ip ?? '',
      static_netmask: cfg.static_netmask || '255.255.255.0',
      static_gateway: cfg.static_gateway ?? '',
      dns_server: cfg.dns_server ?? '',
      openai_api_key: cfg.openai_api_key ?? '',
      google_api_key: cfg.google_api_key ?? '',
    });
    deviceSupportsStaticIp.value = 'ip_mode' in cfg;
    const startMin = cfg.sleep_schedule_start ?? 1380;
    deviceConfig.sleep_start_time = `${String(Math.floor(startMin / 60)).padStart(2, '0')}:${String(startMin % 60).padStart(2, '0')}`;
    const endMin = cfg.sleep_schedule_end ?? 420;
    deviceConfig.sleep_end_time = `${String(Math.floor(endMin / 60)).padStart(2, '0')}:${String(endMin % 60).padStart(2, '0')}`;

    // The frame's POSIX TZ rule, verbatim. Whatever it is -- a fixed offset,
    // a DST rule, something typed by hand -- it goes back unchanged on save.
    deviceConfig.timezone =
      typeof cfg.timezone === 'string' && cfg.timezone !== ''
        ? cfg.timezone
        : DEFAULT_TZ_RULE;
    tzPicked.value = null;
    tzRuleOpened.value = tzChoice.value.kind !== 'zone';

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
    // Layout fields are initialized on EVERY load -- even when the device has
    // no stored processing settings -- falling back to the legacy server-side
    // fields; otherwise values linger from the previously edited device.
    deviceProcessing.scaleMode =
      proc.scaleMode ?? (editingDevice.display_mode || 'cover');
    deviceProcessing.backgroundColor =
      proc.backgroundColor ?? (editingDevice.background_color || 'white');
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
    if (isFrameAuthRefusal(e)) {
      // The frame wants its password: show the refusal, ask, and sync again
      // once the password is in.
      syncingFromDevice.value = false;
      await reflectFrameRefusal(editingDevice.id);
      if ((await promptForEditingDevice()).saved) await syncFromDevice();
      return;
    }
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

// The gallery follows the picked source. A connection that is not (or no
// longer) known falls back to the gallery once the list has loaded.
let appliedSource = '';
const applySource = () => {
  const conn = activeConnection.value;
  if (activeConnectionId.value != null && !conn) {
    if (connectionsStore.loaded) sourceKey.value = 'gallery';
    return;
  }
  const key = sourceKey.value;
  if (key === appliedSource) return;
  appliedSource = key;
  if (conn) {
    galleryStore.setSource(
      conn.source as 'immich' | 'synology_photos' | 'google_photos',
      conn.id
    );
  } else if (key === 'gallery') {
    galleryStore.setSource('gallery');
  } else if (key === 'url') {
    loadURLSources();
  }
  // Persist the pick in the URL hash so a refresh restores it.
  window.location.hash = 'src=' + key;
};
watch(
  [sourceKey, activeConnection, () => connectionsStore.loaded],
  applySource
);

// Devices State
const availableDevices = ref<Device[]>([]);
const deviceListLoading = ref(false);

// The device dialog: a frame showing a calendar needs an account; one
// without a stored choice gets the first connected account, which is also
// what the server falls back to. The calendars follow the account, and a
// calendar picked from one account is dropped when the user switches to
// another (the select's update handler), since the new account may not be
// able to read it.
watch(
  () => [
    showEditDeviceDialog.value,
    editingDevice.show_calendar,
    editingDevice.calendar_connection_id,
  ],
  ([open, show, account]) => {
    if (!open || !show) return;
    if (!account && calendarAccounts.value.length > 0) {
      editingDevice.calendar_connection_id = calendarAccounts.value[0].id;
      return; // the watcher runs again with the account set
    }
    loadDeviceCalendars(account as number | null | undefined);
  }
);

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
    calendar_connection_id: null,
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
    timezone: DEFAULT_TZ_RULE,
    deep_sleep_enabled: true,
    low_battery_indicator: true,
    low_battery_threshold: 10,
  });
  tzPicked.value = null;
  tzRuleOpened.value = false;
  // The Display Settings selects bind to deviceProcessing; without a reset
  // they would carry the previously edited device's layout into the dialog
  deviceProcessing.scaleMode = 'cover';
  deviceProcessing.backgroundColor = 'white';
  isAddingDevice.value = true;
  deviceNeedsPassword.value = false;
  deviceHttpPassword.value = '';
  deviceDialogTab.value = 'general';
  showEditDeviceDialog.value = true;
};

const batteryIcon = (level: number) => {
  if (level >= 95) return 'mdi-battery';
  if (level <= 5) return 'mdi-battery-outline';
  return `mdi-battery-${Math.max(1, Math.round(level / 10)) * 10}`;
};

const formatReportedAt = (iso?: string) => {
  if (!iso) return 'unknown';
  const d = new Date(iso);
  return isNaN(d.getTime()) ? 'unknown' : d.toLocaleString();
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
  // Never carry a half-typed password over from another device's dialog.
  deviceHttpPassword.value = '';
  framePasswordGeneration++;
  resetFramePasswordState();
  deviceDialogTab.value = 'general';
  showEditDeviceDialog.value = true;
  deviceGalleryAlbumIds.value = [];
  deviceImmichAlbumIds.value = [];
  deviceSynologyAlbumIds.value = [];
  deviceUnsplashAlbumIds.value = [];
  devicePexelsAlbumIds.value = [];
  // The connections first: the source dropdown names them, so the device's
  // binding can only be shown once they are known (best-effort).
  if (!connectionsStore.loaded) {
    try {
      await connectionsStore.fetch();
    } catch (e) {
      // Non-fatal
    }
  }
  // Load device remote config
  await loadDeviceConfig(device.id);
  // Load the synced albums of every connection plus this device's bindings
  // (best-effort).
  const connectionBindings: [
    string,
    typeof deviceImmichAlbumIds,
    typeof deviceImmichSyncedAlbums,
  ][] = [
    ['gallery', deviceGalleryAlbumIds, deviceGallerySyncedAlbums],
    ['immich', deviceImmichAlbumIds, deviceImmichSyncedAlbums],
    ['synology_photos', deviceSynologyAlbumIds, deviceSynologySyncedAlbums],
    ['unsplash', deviceUnsplashAlbumIds, deviceUnsplashSyncedAlbums],
    ['pexels', devicePexelsAlbumIds, devicePexelsSyncedAlbums],
  ];
  await Promise.all(
    connectionBindings.map(async ([source, ids, synced]) => {
      // The gallery has albums without connections.
      if (source !== 'gallery' && !hasConnections(source)) return;
      try {
        // All albums, not only the synced ones: a frame may still be
        // limited to an album that was later unticked (before that was
        // refused), and the picker has to show it so it can be unticked.
        const albums = await api.get(`/albums?source=${source}`);
        synced.value = albums.data || [];
        const res = await api.get(
          `/devices/${device.id}/albums?source=${source}`
        );
        ids.value = res.data?.album_ids || [];
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
  // The frame would truncate or misapply the rule without a word (#128).
  if (tzProblem.value !== null) {
    showMessage(`Time zone: ${tzProblem.value}`, true);
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
  // Set when the frame refused the direct config push: the prompt for its
  // password waits until the dialog is closed.
  let pushRefused = false;
  const savedId = editingDevice.id;
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
        display_mode: deviceProcessing.scaleMode || 'cover',
        show_calendar: editingDevice.show_calendar || false,
        calendar_id: editingDevice.calendar_id || '',
        calendar_connection_id: editingDevice.calendar_connection_id || null,
        date_format: editingDevice.date_format || '',
        http_password: deviceHttpPassword.value || undefined,
      });
      deviceHttpPassword.value = '';
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

      // Save device remote config (config + processing + palette)
      const [startH, startM] = deviceConfig.sleep_start_time
        .split(':')
        .map(Number);
      const [endH, endM] = deviceConfig.sleep_end_time.split(':').map(Number);

      // Compute image URL: use server URL if "use this server" is checked.
      // getImageUrl() targets the direct add-on port, so the URL works when
      // the ESP32 reaches the server straight (ingress port 8123 cannot serve
      // /image/*).
      let imageUrl = deviceConfig.image_url;
      if (useThisServer.value && deviceConfig.rotation_mode === 'url') {
        imageUrl = getImageUrl();
      }

      // Reconcile the schedule with firmware capability.
      // - New firmware: send rotate_cron only. It ignores rotate_interval
      //   next to rotate_cron and never reports it, so sending one would
      //   make every save differ from the frame's stored settings.
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
        ? { rotate_cron: deviceConfig.rotate_cron }
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

      // Static IP / DNS exist only on firmware reporting ip_mode; don't send
      // the fields to older firmware (#43).
      const networkFields = deviceSupportsStaticIp.value
        ? {
            ip_mode: deviceConfig.ip_mode,
            static_ip: deviceConfig.static_ip,
            static_netmask: deviceConfig.static_netmask,
            static_gateway: deviceConfig.static_gateway,
            dns_server: deviceConfig.dns_server,
          }
        : {};

      // Save server-side device fields, the host among them, before the
      // password: the server checks that against the saved host, so a host
      // typed alongside it is the one it is checked at.
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
        deviceProcessing.scaleMode || 'cover',
        editingDevice.show_calendar || false,
        editingDevice.calendar_id || '',
        editingDevice.date_format || '',
        deviceSource,
        deviceProcessing.backgroundColor || '',
        editingDevice.calendar_connection_id || null,
        deviceSource ? selectedConnectionId.value : null
      );

      // Write-only: only send when the field was filled in. Blank leaves any
      // stored password alone, matching how the frame treats a null. Done
      // after validation, so a rejected save does not change the credential,
      // after the host is saved, so it is checked at that host, and before
      // the config push, which has to authenticate with it. The server checks
      // it with the frame first: one the frame rejects stops the save here,
      // to be corrected, with the fields above saved and said so; one the
      // frame cannot confirm is kept, and noted with the outcome.
      let passwordNote = '';
      if (deviceHttpPassword.value !== '') {
        let stored;
        try {
          stored = await setDeviceHttpPassword(
            editingDevice.id!,
            deviceHttpPassword.value
          );
        } catch (e: unknown) {
          if (!isFrameAuthRefusal(e)) throw e;
          showMessage(
            'Frame password: ' +
              getApiError(e, 'the frame rejected it.') +
              " The device's other settings were saved; the frame's settings were not sent. Fix the password and save again.",
            true
          );
          // The list shows the fields just saved, whatever the dialog does next.
          await loadDevices();
          return;
        }
        deviceHttpPassword.value = '';
        mirrorFramePasswordState(editingDevice.id!, stored);
        if (!stored.verified && stored.warning) {
          passwordNote = ' ' + stored.warning;
        }
      }

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
          timezone: deviceConfig.timezone,
          ntp_server: deviceConfig.ntp_server,
          ...networkFields,
          deep_sleep_enabled: deviceConfig.deep_sleep_enabled,
          low_battery_indicator: deviceConfig.low_battery_indicator,
          low_battery_threshold: deviceConfig.low_battery_threshold,
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

      // Persist per-device gallery album bindings (best-effort).
      try {
        await api.put(`/devices/${editingDevice.id}/albums`, {
          source: 'gallery',
          album_ids: deviceGalleryAlbumIds.value,
        });
      } catch (e: any) {
        showMessage(
          'Device saved, but failed to save gallery album bindings: ' +
            (e.response?.data?.error || e.message),
          true
        );
      }

      // Persist per-device Immich album bindings (best-effort).
      if (hasConnections('immich')) {
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
      if (hasConnections('synology_photos')) {
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

      // Persist per-device topic bindings (best-effort).
      const topicBindings: Record<string, number[]> = {
        unsplash: deviceUnsplashAlbumIds.value,
        pexels: devicePexelsAlbumIds.value,
      };
      for (const [source, album_ids] of Object.entries(topicBindings)) {
        if (!hasConnections(source)) continue;
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
        showMessage('Device saved and config pushed to device.' + passwordNote);
      } else if (result.push_result === 'unchanged') {
        // Only server-side settings changed; the frame was not contacted.
        showMessage('Device saved.' + passwordNote);
      } else if (result.push_result === 'auth_required') {
        // Saved server-side and delivered with the frame's next image fetch
        // (the frame's own channel, not password-gated); only the direct
        // push was refused. Asked for the password once the dialog is away.
        showMessage(
          "Device saved. The frame refused the server's password, so the config will sync on its next image fetch." +
            passwordNote
        );
        pushRefused = true;
      } else {
        showMessage(
          'Device saved. Device is offline — config will sync on next image fetch.' +
            passwordNote
        );
      }
    }
    await loadDevices();
    showEditDeviceDialog.value = false;
    if (pushRefused && savedId) {
      const refused = availableDevices.value.find(
        (d: Device) => d.id === savedId
      );
      if (refused?.auth_required && authStore.isLoggedIn) {
        void promptFramePassword(refused);
      }
    }
  } catch (e: any) {
    showMessage(
      'Failed to save device: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    savingDeviceConfig.value = false;
  }
};

// Closing the device dialog, however it opens next (another device, the
// same one, or Add Device), leaves any frame password request still in
// flight to the device list alone.
watch(showEditDeviceDialog, (open) => {
  if (!open) {
    framePasswordGeneration++;
    resetFramePasswordState();
  }
});

const loadDevices = async () => {
  deviceListLoading.value = true;
  try {
    availableDevices.value = await listDevices();
    // A frame refusing the server gets asked about as soon as the list
    // shows it -- but not over the device dialog, where the field for it
    // is, not again once put off (see useFramePasswordPrompt), and not for
    // a session that ended while the list was loading.
    if (authStore.isLoggedIn && !showEditDeviceDialog.value) {
      promptIfNeeded(availableDevices.value);
    }
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

// Restore the picked source from the URL hash on load: #src=<key>, or the
// older #gtab=<provider>, which now means that provider's first connection
// (resolved once the connections have loaded).
let pendingProvider = '';
const restoreSourceFromHash = () => {
  const h = window.location.hash;
  const src = h.match(/src=([\w:-]+)/);
  if (src) {
    sourceKey.value = src[1];
    return;
  }
  const tab = h.match(/gtab=([\w-]+)/);
  if (!tab) return;
  if (['gallery', 'url', 'ai_generation'].includes(tab[1])) {
    sourceKey.value = tab[1];
  } else {
    pendingProvider = tab[1];
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
  google_client_id: '',
  google_client_secret: '',
  telegram_bot_token: '',
  telegram_push_enabled: false,
  telegram_target_device_id: [] as number[],
  telegram_album_id: '',
  openai_api_key: '',
  google_api_key: '',
  // Device-facing base URL for image requests (e.g. http://homeassistant.local:9608).
  // Empty = derive from the browser's location.
  device_image_base_url: '',
  device_host: '', // Keep for backward compatibility/display? Or remove. Remove from form, keep in store maybe?
});

// Synced (persisted) albums of a connection-backed source available for
// per-device binding. Items use the internal album row id (number) as the
// value. A frame bound to one connection sees that connection's albums (its
// selections on the others are kept, unseen, for when it switches back); one
// on 'Any' sees every connection's, each name prefixed with the connection's
// so two servers' "Family" albums can be told apart.
const connectionAlbumOptions = (
  source: string,
  albums: any[],
  selected: number[]
) => {
  const bound =
    selectedSource.value === source ? selectedConnectionId.value : null;
  const several = !bound && connectionsStore.hasSeveral(source);
  return (albums || [])
    .filter((a: any) => a.sync_enabled || selected.includes(a.id))
    .filter((a: any) => !bound || a.connection_id === bound)
    .map((a: any) => ({
      id: a.id,
      name:
        (several && a.connection_name
          ? `${a.connection_name} · ${a.name}`
          : a.name) + (a.sync_enabled ? '' : ' (not synced)'),
    }));
};
// Every source's selection is saved, shown or not: a frame switched to
// another source or connection and back finds its albums still ticked. The
// server only serves from the selection of the connection the frame is on.
const deviceGalleryAlbumOptions = computed(() =>
  connectionAlbumOptions(
    'gallery',
    deviceGallerySyncedAlbums.value,
    deviceGalleryAlbumIds.value
  )
);
const deviceImmichAlbumOptions = computed(() =>
  connectionAlbumOptions(
    'immich',
    deviceImmichSyncedAlbums.value,
    deviceImmichAlbumIds.value
  )
);

// --- Gallery albums (folders inside the local gallery) ---
const newAlbumName = ref('');
const creatingAlbum = ref(false);
const renamingAlbumId = ref<number | null>(null);
const renamingAlbumName = ref('');
const telegramAlbumOptions = computed(() => [
  { id: '', name: 'No album (unfiled)' },
  ...galleryStore.albums.map((a: any) => ({ id: String(a.id), name: a.name })),
]);
const createGalleryAlbum = async () => {
  const name = newAlbumName.value.trim();
  if (!name) return;
  creatingAlbum.value = true;
  try {
    await galleryStore.createAlbum(name);
    newAlbumName.value = '';
  } catch (e: any) {
    showMessage(
      'Failed to create album: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    creatingAlbum.value = false;
  }
};
const startRenameAlbum = (album: any) => {
  renamingAlbumId.value = album.id;
  renamingAlbumName.value = album.name;
};
const saveAlbumName = async (album: any) => {
  if (renamingAlbumId.value !== album.id) return;
  renamingAlbumId.value = null;
  const name = renamingAlbumName.value.trim();
  if (!name || name === album.name) return;
  try {
    await galleryStore.renameAlbum(album.id, name);
  } catch (e: any) {
    showMessage(
      'Failed to rename album: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};
const deleteGalleryAlbum = async (album: any, withPhotos: boolean) => {
  const n = album.asset_count || 0;
  const what = withPhotos
    ? `Delete the album "${album.name}" and its ${n} photo${n === 1 ? '' : 's'}? The photos are removed from this server.`
    : `Delete the album "${album.name}"? Its ${n} photo${n === 1 ? '' : 's'} stay in the gallery, unfiled.`;
  if (!(await confirmDialog.value.open(what))) return;
  try {
    await galleryStore.deleteAlbum(album.id, withPhotos);
    if (String(album.id) === String(form.telegram_album_id)) {
      form.telegram_album_id = '';
      await saveSettingsInternal();
    }
    showMessage(withPhotos ? 'Album and photos deleted.' : 'Album deleted.');
  } catch (e: any) {
    const inUse = inUseMessage(e, `The album "${album.name}"`);
    if (inUse) {
      await informPrompt(inUse, 'Album in use');
      return;
    }
    showMessage(
      'Failed to delete album: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};
const deviceSynologyAlbumOptions = computed(() =>
  connectionAlbumOptions(
    'synology_photos',
    deviceSynologySyncedAlbums.value,
    deviceSynologyAlbumIds.value
  )
);

const deviceUnsplashAlbumOptions = computed(() =>
  connectionAlbumOptions(
    'unsplash',
    deviceUnsplashSyncedAlbums.value,
    deviceUnsplashAlbumIds.value
  )
);
const devicePexelsAlbumOptions = computed(() =>
  connectionAlbumOptions(
    'pexels',
    devicePexelsSyncedAlbums.value,
    devicePexelsAlbumIds.value
  )
);

// Album search + checked-first sorting now live in <AlbumPicker>.

// The connection card started a sync or cleared photos: refresh the photos
// shown below it.
const onConnectionSynced = () => {
  galleryStore.triggerRefresh();
};
// Leaving a connection with unsynced album or topic changes: its card offers
// to sync now. The card is taken before the switch re-renders, while it is
// still mounted; the prompt itself only needs the store.
watch(sourceKey, () => {
  activeCard.value?.promptIfDirty?.();
});
// The add dialog connected a new source: show it.
const onConnectionCreated = (conn: Connection) => {
  sourceKey.value = `conn:${conn.id}`;
};
const openSystemTab = () => {
  showAddConnection.value = false;
  activeMainTab.value = 'security';
};

// Delete all photos for a given image source. Also disables that source's
// album sync on the backend, so refresh the sync-selection UI afterwards.
const deletingAllPhotos = ref(false);
const deleteAllPhotosForSource = async (source: 'gallery') => {
  if (
    !(await confirmDialog.value.open(
      'Delete all photos for this source? This also turns off its album sync.'
    ))
  )
    return;

  deletingAllPhotos.value = true;
  try {
    await api.delete('/gallery/photos?source=' + source);

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
  restoreSourceFromHash();
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
    telegram_bot_token: store.settings.telegram_bot_token || '',
    telegram_album_id: store.settings.telegram_album_id || '',
    telegram_push_enabled: store.settings.telegram_push_enabled === 'true',
    telegram_target_device_id: store.settings.telegram_target_device_id
      ? store.settings.telegram_target_device_id
          .split(',')
          .filter((id) => id)
          .map((id) => parseInt(id))
      : [],
    weather_lat: store.settings.weather_lat || '',
    weather_lon: store.settings.weather_lon || '',
    device_image_base_url: store.settings.device_image_base_url || '',
    openai_api_key: store.settings.openai_api_key || '',
    google_api_key: store.settings.google_api_key || '',
  });

  // Run independent fetches in parallel
  const parallelFetches: Promise<void>[] = [
    loadDevices(),
    galleryStore.fetchAlbums(),
  ];

  // Connections (the cards load their own albums, topics and counts).
  parallelFetches.push(
    (async () => {
      try {
        await connectionsStore.fetch();
      } catch (e) {
        // Non-fatal: the Data Sources tab retries when opened.
      }
    })()
  );

  await Promise.all(parallelFetches);

  // Parse URL params for deep linking (e.g. from OAuth callback)
  const params = new URLSearchParams(window.location.search);
  const tab = params.get('tab');
  const source = params.get('source');
  const connection = Number(params.get('connection'));

  if (tab && tab !== 'datasources') {
    activeMainTab.value = tab;
  }
  if (connection > 0) {
    focusConnectionId.value = connection;
    // Fresh from the OAuth callback: the connection list must not show the
    // state it cached before the redirect.
    await connectionsStore.fetch();
    if (connectionsStore.byId(connection)?.source === CALENDAR_SOURCE) {
      activeMainTab.value = 'calendar';
    } else {
      sourceKey.value = `conn:${connection}`;
    }
  } else {
    // A provider named by the callback (an aborted sign-in) or by an old
    // #gtab= link: show its first connection.
    const provider =
      source === 'google' ? 'google_photos' : source || pendingProvider;
    const first = provider ? connectionsStore.bySource(provider)[0] : null;
    if (first) sourceKey.value = `conn:${first.id}`;
  }

  // Clean up URL if params were present
  if (tab || source || connection) {
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
    telegram_album_id: String(form.telegram_album_id ?? ''),
    telegram_push_enabled: String(form.telegram_push_enabled),
    telegram_target_device_id: Array.isArray(form.telegram_target_device_id)
      ? form.telegram_target_device_id.join(',')
      : form.telegram_target_device_id,
    weather_lat: form.weather_lat,
    weather_lon: form.weather_lon,
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
