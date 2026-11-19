<template>
  <v-card variant="outlined" class="mb-4 connection-card">
    <v-card-title class="d-flex align-center ga-2 flex-wrap">
      <template v-if="renaming">
        <v-text-field
          v-model="nameDraft"
          density="compact"
          variant="outlined"
          hide-details
          autofocus
          class="connection-name-field"
          @keyup.enter="saveName"
          @keyup.esc="renaming = false"
          @blur="saveName"
        ></v-text-field>
      </template>
      <template v-else>
        <span class="text-subtitle-1 font-weight-bold">{{
          connection.name
        }}</span>
        <v-btn
          icon="mdi-pencil"
          size="x-small"
          variant="text"
          title="Rename"
          @click="startRename"
        ></v-btn>
      </template>
      <v-spacer></v-spacer>
      <v-chip
        v-if="connection.connected"
        size="small"
        color="success"
        variant="tonal"
        prepend-icon="mdi-check-circle"
        >Connected</v-chip
      >
      <v-chip v-else size="small" variant="tonal">Not connected</v-chip>
      <v-btn
        icon="mdi-delete-outline"
        size="small"
        variant="text"
        color="error"
        title="Delete this connection and its photos"
        @click="remove"
      ></v-btn>
    </v-card-title>

    <v-card-text>
      <!-- Connected: albums, auto-sync, actions -->
      <div v-if="connection.connected && !editingCredentials">
        <v-alert
          type="success"
          variant="tonal"
          class="mb-4"
          density="compact"
          icon="mdi-check-circle"
        >
          <span v-if="isImmich">
            Connected to Immich ({{ connection.config.url }})
          </span>
          <span v-else-if="isNextcloud">
            Connected to Nextcloud Memories ({{ connection.config.username }} @
            {{ connection.config.url }})
          </span>
          <span v-else>
            Connected to Synology Photos ({{ connection.config.account }} @
            {{ connection.config.url }})
          </span>
          <span class="text-grey"> · {{ count }} photos synced</span>
          <template #append>
            <v-btn
              size="small"
              variant="text"
              @click="editingCredentials = true"
              >Edit connection</v-btn
            >
          </template>
        </v-alert>

        <v-row class="mt-2">
          <v-col cols="12">
            <div class="d-flex align-center justify-space-between mb-1">
              <h3 class="text-subtitle-1 font-weight-bold">Albums to sync</h3>
              <v-btn
                size="small"
                variant="text"
                prepend-icon="mdi-refresh"
                :loading="loading"
                @click="loadRemoteAlbums(true)"
                >Refresh albums</v-btn
              >
            </div>
            <div class="text-caption text-grey mb-2">
              <span v-if="isImmich">
                Choose which Immich albums and collections to pull photos from.
                Saving replaces the entire sync set.
              </span>
              <span v-else-if="isNextcloud">
                Choose which Memories albums and collections to pull photos
                from. Albums other Nextcloud users shared with you are listed
                too. Saving replaces the entire sync set.
              </span>
              <span v-else>
                Choose which Synology Photos albums to pull photos from. Albums
                other DSM users shared with this account are listed too. Saving
                replaces the entire sync set.
              </span>
            </div>

            <AlbumPicker
              v-model="syncAlbumIds"
              :albums="remoteAlbums"
              :label-field="isImmich ? 'albumName' : 'name'"
              :stringify="!isImmich"
              :badge-field="isImmich ? '' : 'shared_with_me'"
              badge-text="Shared with me"
              :empty-text="`No albums found. Click &quot;Refresh albums&quot; to load them from ${sourceLabel}.`"
            >
              <template v-if="hasVirtualCollections" #prepend>
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
                    :label="isImmich ? 'Memories (on this day)' : 'On This Day'"
                    color="primary"
                    density="compact"
                    hide-details
                  ></v-checkbox>
                </v-list-item>
              </template>
            </AlbumPicker>
          </v-col>
        </v-row>

        <v-row v-if="hasVirtualCollections && syncMemories" class="mt-0">
          <v-col cols="12">
            <v-select
              v-model="memoryMode"
              :items="memoryModeOptions"
              item-title="title"
              item-value="value"
              label="Memory Years"
              variant="outlined"
              density="compact"
              hint="Shuffle across all past years, or only the most recent year's photos"
              persistent-hint
              @update:model-value="saveOptions"
            ></v-select>
          </v-col>
        </v-row>

        <v-row class="mt-1">
          <v-col cols="12" md="6">
            <v-checkbox
              v-model="autoSyncEnabled"
              label="Auto Sync Album"
              color="primary"
              density="compact"
              hide-details
              @update:model-value="saveOptions"
            ></v-checkbox>
          </v-col>
          <v-col cols="12" md="6">
            <v-select
              v-model="autoSyncInterval"
              :items="intervalOptions"
              item-title="title"
              item-value="value"
              label="Auto Sync Interval"
              variant="outlined"
              density="compact"
              :disabled="!autoSyncEnabled"
              hint="How often to refresh photos from the selected albums"
              persistent-hint
              @update:model-value="saveOptions"
            ></v-select>
          </v-col>
        </v-row>

        <div class="d-flex flex-wrap ga-2 mt-4">
          <v-btn color="primary" :loading="loading" @click="syncNow"
            >Sync Now</v-btn
          >
          <v-btn color="warning" @click="clearPhotos">Clear All Photos</v-btn>
          <v-btn
            v-if="isSynology || isNextcloud"
            color="error"
            variant="text"
            @click="disconnect"
            >{{ isNextcloud ? 'Sign out' : 'Disconnect' }}</v-btn
          >
        </div>
      </div>

      <!-- Not connected (or editing): credentials -->
      <div v-else>
        <template v-if="isImmich">
          <v-text-field
            v-model="form.url"
            label="Immich Server URL"
            placeholder="http://192.168.1.10:2283"
            variant="outlined"
            class="mb-2"
          ></v-text-field>
          <v-text-field
            v-model="form.api_key"
            label="API Key"
            type="password"
            variant="outlined"
            class="mb-4"
            :placeholder="
              connection.config.has_api_key ? '(unchanged)' : undefined
            "
            :persistent-placeholder="!!connection.config.has_api_key"
          ></v-text-field>
        </template>
        <template v-else-if="isNextcloud">
          <v-text-field
            v-model="form.url"
            label="Nextcloud URL"
            placeholder="https://cloud.example.com"
            variant="outlined"
            class="mb-2"
            hint="Changing the URL signs this connection out of the old server"
            persistent-hint
            :disabled="nc.waiting.value"
          ></v-text-field>
          <div v-if="!nc.waiting.value" class="text-body-2 text-grey mb-4">
            <template v-if="connection.config.username">
              Signed in as <b>{{ connection.config.username }}</b
              >. Sign in again to switch accounts or renew access.
            </template>
            <template v-else>
              Sign in on your Nextcloud and grant access; this server receives
              an app password of its own.
            </template>
          </div>
          <NextcloudSignInPanel
            v-else
            class="mb-4"
            :login-url="nc.loginUrl.value"
          ></NextcloudSignInPanel>
        </template>
        <template v-else>
          <v-text-field
            v-model="form.url"
            label="NAS URL"
            placeholder="https://192.168.1.10:5001"
            variant="outlined"
            class="mb-2"
          ></v-text-field>
          <v-text-field
            v-model="form.account"
            label="Account"
            variant="outlined"
            class="mb-2"
          ></v-text-field>
          <v-text-field
            v-model="form.password"
            label="Password"
            type="password"
            variant="outlined"
            class="mb-2"
            :placeholder="
              connection.config.has_password ? '(unchanged)' : undefined
            "
            :persistent-placeholder="!!connection.config.has_password"
          ></v-text-field>
          <v-checkbox
            v-model="form.skip_cert"
            label="Skip Certificate Verification (Insecure)"
            color="primary"
            density="compact"
          ></v-checkbox>
          <v-text-field
            v-model="form.otp_code"
            label="OTP Code (If 2FA enabled)"
            placeholder="6-digit code"
            variant="outlined"
            class="mb-4"
          ></v-text-field>
        </template>

        <div class="d-flex flex-wrap ga-2">
          <v-btn
            color="primary"
            :disabled="!canConnect"
            :loading="loading || nc.waiting.value"
            @click="connect"
          >
            {{ isNextcloud ? 'Sign in with Nextcloud' : 'Connect' }}
          </v-btn>
          <v-btn
            v-if="editingCredentials || nc.waiting.value"
            variant="text"
            @click="cancelEditCredentials"
            >Cancel</v-btn
          >
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import AlbumPicker from './AlbumPicker.vue';
import { inUseMessage } from '../utils/inUse';
import {
  SOURCE_LABELS,
  VIRTUAL_COLLECTION_SOURCES,
  useConnectionsStore,
  type Connection,
} from '../stores/connections';
import { useNextcloudLogin } from '../composables/useNextcloudLogin';
import NextcloudSignInPanel from './NextcloudSignInPanel.vue';

const props = defineProps<{
  connection: Connection;
  // Settings' confirm dialog, so every card asks the same way.
  confirm: (message: string) => Promise<boolean>;
  showMessage: (message: string, isError?: boolean) => void;
  // A notice with only OK, for a removal the server refused because a frame
  // is bound to it.
  inform?: (message: string, title?: string) => Promise<void>;
}>();
const emit = defineEmits<{
  // A sync was started or photos were cleared: the gallery should refresh.
  synced: [source: string];
  deleted: [id: number];
}>();

const store = useConnectionsStore();
const isImmich = computed(() => props.connection.source === 'immich');
const isNextcloud = computed(
  () => props.connection.source === 'nextcloud_memories'
);
const isSynology = computed(
  () => props.connection.source === 'synology_photos'
);
// All Photos / Favorites / On This Day offered next to the albums.
const hasVirtualCollections = computed(() =>
  VIRTUAL_COLLECTION_SOURCES.includes(props.connection.source)
);
const sourceLabel = computed(
  () => SOURCE_LABELS[props.connection.source] || props.connection.source
);
const loading = ref(false);
const count = computed(() => store.counts[props.connection.id] ?? 0);

// --- name ---
const renaming = ref(false);
const nameDraft = ref('');
const startRename = () => {
  nameDraft.value = props.connection.name;
  renaming.value = true;
};
const saveName = async () => {
  if (!renaming.value) return;
  renaming.value = false;
  const name = nameDraft.value.trim();
  if (!name || name === props.connection.name) return;
  try {
    await store.update(props.connection.id, { name });
  } catch (e: any) {
    props.showMessage(
      'Failed to rename: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- credentials ---
const editingCredentials = ref(false);
const form = reactive({
  url: '',
  api_key: '',
  account: '',
  password: '',
  skip_cert: false,
  otp_code: '',
});
const nc = useNextcloudLogin();
const resetForm = () => {
  const cfg = props.connection.config || {};
  form.url = cfg.url || '';
  form.api_key = '';
  form.account = cfg.account || '';
  form.password = '';
  form.skip_cert = !!cfg.skip_cert;
  form.otp_code = '';
};
const canConnect = computed(() => {
  const cfg = props.connection.config || {};
  if (isImmich.value) {
    return !!form.url && (!!form.api_key || !!cfg.has_api_key);
  }
  if (isNextcloud.value) {
    return !!form.url && !nc.waiting.value;
  }
  return (
    !!form.url && !!form.account && (!!form.password || !!cfg.has_password)
  );
});
const cancelEditCredentials = () => {
  nc.cancel();
  editingCredentials.value = false;
  resetForm();
};

// Nextcloud: save the URL, then sign in through Nextcloud's login flow.
const signInNextcloud = async () => {
  // Opened before anything is awaited, or the browser blocks it.
  const tab = nc.openTab();
  try {
    await store.update(props.connection.id, {
      config: { url: form.url },
      enabled: true,
    });
    const conn = await nc.signIn(props.connection.id, tab);
    if (!conn) return; // cancelled
    editingCredentials.value = false;
    resetForm();
    props.showMessage(`Signed in as ${conn.config.username}.`);
    await loadAll();
  } catch (e: any) {
    tab?.close();
    props.showMessage(
      'Sign-in failed: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Save the credentials, then verify them. An empty secret keeps the stored one.
const connect = async () => {
  if (isNextcloud.value) return signInNextcloud();
  loading.value = true;
  try {
    const config: Record<string, any> = isImmich.value
      ? { url: form.url, api_key: form.api_key }
      : {
          url: form.url,
          account: form.account,
          password: form.password,
          skip_cert: form.skip_cert,
        };
    // A connection the migration created without complete credentials is
    // disabled; working credentials switch it back on.
    await store.update(props.connection.id, { config, enabled: true });
    await store.test(props.connection.id, form.otp_code);
    form.otp_code = '';
    editingCredentials.value = false;
    props.showMessage('Connection Successful!');
    await loadAll();
  } catch (e: any) {
    const err = e.response?.data?.error || e.message || 'Unknown error';
    if (isSynology.value && String(err).includes('code: 403')) {
      props.showMessage(
        '2FA Required! Please enter OTP code and Connect again.',
        true
      );
    } else {
      props.showMessage('Connection Failed: ' + err, true);
    }
  } finally {
    loading.value = false;
  }
};

// Synology: drop the DSM session, keep the credentials. Nextcloud: revoke
// the app password and forget it, keep the URL.
const disconnect = async () => {
  const question = isNextcloud.value
    ? `Sign ${props.connection.name} out of Nextcloud? Its app password is revoked. The albums stay selected; sign in again to resume syncing.`
    : `Disconnect ${props.connection.name}? The albums stay selected; connect again to resume syncing.`;
  if (!(await props.confirm(question))) return;
  try {
    await store.logout(props.connection.id);
    remoteAlbums.value = [];
    resetForm();
    props.showMessage(isNextcloud.value ? 'Signed out.' : 'Disconnected.');
  } catch (e: any) {
    props.showMessage(
      'Error disconnecting: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// The server refuses to delete a connection a frame is bound to; say which.
const reportInUse = async (e: any) => {
  const msg = inUseMessage(e, `The connection "${props.connection.name}"`);
  if (!msg) return false;
  if (props.inform) await props.inform(msg, 'Connection in use');
  else props.showMessage(msg, true);
  return true;
};

const remove = async () => {
  if (
    !(await props.confirm(
      `Delete the connection "${props.connection.name}"? Its synced photos and album selections are removed from this server.`
    ))
  )
    return;
  try {
    await store.remove(props.connection.id);
    emit('deleted', props.connection.id);
    emit('synced', props.connection.source);
  } catch (e: any) {
    if (await reportInUse(e)) return;
    props.showMessage(
      'Failed to delete: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- albums to sync ---
const remoteAlbums = ref<any[]>([]);
const syncAlbumIds = ref<string[]>([]);
const syncFavorites = ref(false);
const syncAll = ref(false);
const syncMemories = ref(false);
const IMMICH_FAVORITES_ID = '__favorites__';
const IMMICH_ALL_ID = '__all__';
const IMMICH_MEMORIES_ID = '__memories__';

// Album selection auto-saves (debounced) but does NOT import — the user syncs
// manually or is prompted on leaving. Suppressed while applying persisted state.
let suppressAutoSave = false;
let saveTimer: number | null = null;
const dirty = ref(false);

const applySyncedState = (synced: any[]) => {
  suppressAutoSave = true;
  const enabled = synced.filter((a: any) => a.sync_enabled);
  if (hasVirtualCollections.value) {
    syncFavorites.value = enabled.some(
      (a: any) => a.external_id === IMMICH_FAVORITES_ID
    );
    syncAll.value = enabled.some((a: any) => a.external_id === IMMICH_ALL_ID);
    syncMemories.value = enabled.some(
      (a: any) => a.external_id === IMMICH_MEMORIES_ID
    );
    syncAlbumIds.value = enabled
      .filter((a: any) => a.kind === 'album')
      .map((a: any) => a.external_id);
  } else {
    syncAlbumIds.value = enabled.map((a: any) => a.external_id);
  }
  nextTick(() => {
    suppressAutoSave = false;
  });
};

const loadSyncedAlbums = async () => {
  try {
    applySyncedState(await store.syncedAlbums(props.connection.id));
  } catch (e) {
    // Non-fatal: checkboxes start unchecked until the user refreshes.
  }
};

const loadRemoteAlbums = async (announce = false) => {
  loading.value = true;
  try {
    remoteAlbums.value = await store.remoteAlbums(props.connection.id);
    await loadSyncedAlbums();
    if (announce) props.showMessage('Albums loaded!');
  } catch (e: any) {
    if (e.response?.status === 401) {
      props.showMessage(
        'Session expired or Unauthorized. Please connect again.',
        true
      );
      await store.refreshOne(props.connection.id);
    } else if (announce) {
      props.showMessage(
        'Failed to load albums: ' + (e.response?.data?.error || e.message),
        true
      );
    }
  } finally {
    loading.value = false;
  }
};

const saveSelection = async () => {
  try {
    await store.saveSyncAlbums(props.connection.id, {
      album_ids: syncAlbumIds.value,
      favorites: syncFavorites.value,
      all: syncAll.value,
      memories: syncMemories.value,
    });
    await loadSyncedAlbums();
    await store.fetchCount(props.connection.id);
  } catch (e: any) {
    const inUse = inUseMessage(
      e,
      'An album you unticked',
      "Change those frames' album selection under Settings → Devices first, then untick it again."
    );
    if (inUse) {
      if (props.inform) await props.inform(inUse, 'Album in use');
      else props.showMessage(inUse, true);
      // Put the tick back: the stored selection did not change.
      suppressAutoSave = true;
      try {
        await loadSyncedAlbums();
      } finally {
        await nextTick();
        suppressAutoSave = false;
      }
      return;
    }
    props.showMessage(
      'Failed to save sync selection: ' +
        (e.response?.data?.error || e.message),
      true
    );
  }
};

watch(
  [syncAlbumIds, syncFavorites, syncAll, syncMemories],
  () => {
    if (suppressAutoSave) return;
    dirty.value = true;
    if (saveTimer != null) clearTimeout(saveTimer);
    saveTimer = window.setTimeout(saveSelection, 600);
  },
  { deep: true }
);

// --- options (memory mode, auto-sync) ---
const memoryMode = ref('all');
const autoSyncEnabled = ref(false);
const autoSyncInterval = ref(60);
const memoryModeOptions = [
  { title: 'All years', value: 'all' },
  { title: 'Most recent year only', value: 'latest' },
];
const intervalOptions = [
  { title: 'Every 15 minutes', value: 15 },
  { title: 'Every 30 minutes', value: 30 },
  { title: 'Every 1 hour', value: 60 },
  { title: 'Every 3 hours', value: 180 },
  { title: 'Every 6 hours', value: 360 },
  { title: 'Every 12 hours', value: 720 },
  { title: 'Every 24 hours', value: 1440 },
];
const applyOptions = () => {
  const cfg = props.connection.config || {};
  memoryMode.value = cfg.memory_mode || 'all';
  autoSyncEnabled.value = !!cfg.auto_sync_enabled;
  autoSyncInterval.value = cfg.auto_sync_interval_minutes || 60;
};
const saveOptions = async () => {
  try {
    await store.update(props.connection.id, {
      config: {
        memory_mode: memoryMode.value,
        auto_sync_enabled: autoSyncEnabled.value,
        auto_sync_interval_minutes: autoSyncInterval.value,
      },
    });
  } catch (e: any) {
    props.showMessage(
      'Failed to save: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- actions ---
const syncNow = async () => {
  loading.value = true;
  try {
    await store.sync(props.connection.id);
    dirty.value = false;
    props.showMessage('Sync started — importing in the background.');
    emit('synced', props.connection.source);
  } catch (e: any) {
    if (e.response?.status === 401) {
      props.showMessage('Session expired. Please connect again.', true);
    } else {
      props.showMessage(
        'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
        true
      );
    }
  } finally {
    loading.value = false;
  }
};

// Clear = delete the photos AND turn album sync off, so they don't re-import.
const clearPhotos = async () => {
  if (
    !(await props.confirm(
      `Delete all photos synced from ${props.connection.name}? This also turns off its album sync.`
    ))
  )
    return;
  try {
    await store.clearPhotos(props.connection.id);
    await loadSyncedAlbums();
    props.showMessage('All photos deleted.');
    emit('synced', props.connection.source);
  } catch (e: any) {
    props.showMessage(
      'Failed to delete photos: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Settings calls this when the user leaves the tab with an unsynced selection.
const promptIfDirty = async () => {
  if (!dirty.value) return;
  dirty.value = false; // ask once
  if (
    await props.confirm(
      `You changed which albums ${props.connection.name} syncs. Sync now to apply the changes?`
    )
  ) {
    await syncNow();
  }
};
defineExpose({ promptIfDirty });

const loadAll = async () => {
  await store.fetchCount(props.connection.id);
  await loadRemoteAlbums(false);
};

onMounted(async () => {
  resetForm();
  applyOptions();
  if (props.connection.connected) await loadAll();
});

watch(
  () => props.connection.connected,
  async (connected, was) => {
    if (connected && !was) await loadAll();
  }
);
</script>

<style scoped>
.connection-name-field {
  max-width: 320px;
}
</style>
