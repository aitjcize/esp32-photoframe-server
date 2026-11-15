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
      <!-- Connected: topics, options, actions -->
      <div v-if="connection.connected && !editingKey">
        <v-alert
          type="success"
          variant="tonal"
          class="mb-4"
          density="compact"
          icon="mdi-check-circle"
        >
          Connected to {{ sourceLabel }}
          <span class="text-grey"> · {{ count }} photos synced</span>
          <template #append>
            <v-btn size="small" variant="text" @click="editingKey = true"
              >Change key</v-btn
            >
          </template>
        </v-alert>

        <TopicManager
          title="Search topics"
          hint="Each topic becomes an album of matching photos."
          :topics="topics"
          :count="count"
          :loading="loading"
          :randomize-results="randomizeResults"
          :auto-sync-enabled="autoSyncEnabled"
          :auto-sync-interval="autoSyncInterval"
          :auto-sync-options="intervalOptions"
          @update:randomize-results="
            randomizeResults = $event;
            saveOptions();
          "
          @update:auto-sync-enabled="
            autoSyncEnabled = $event;
            saveOptions();
          "
          @update:auto-sync-interval="
            autoSyncInterval = $event;
            saveOptions();
          "
          @save="saveTopics"
          @sync="syncNow"
          @clear="clearPhotos"
        ></TopicManager>
      </div>

      <!-- Not connected (or changing the key) -->
      <div v-else>
        <v-text-field
          v-model="apiKey"
          :label="keyLabel"
          :hint="keyHint"
          :persistent-hint="!!keyHint"
          type="password"
          variant="outlined"
          density="compact"
          class="mb-4"
          :placeholder="
            connection.config.has_api_key ? '(unchanged)' : undefined
          "
          :persistent-placeholder="!!connection.config.has_api_key"
        ></v-text-field>

        <div class="d-flex flex-wrap ga-2">
          <v-btn
            color="primary"
            :disabled="!apiKey && !connection.config.has_api_key"
            :loading="loading"
            @click="connect"
          >
            Connect
          </v-btn>
          <v-btn v-if="editingKey" variant="text" @click="cancelEditKey"
            >Cancel</v-btn
          >
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import TopicManager from './TopicManager.vue';
import {
  SOURCE_LABELS,
  useConnectionsStore,
  type Connection,
} from '../stores/connections';

// One Unsplash or Pexels connection: its API key, search topics (each a
// synced album), sync options and photos.
const props = defineProps<{
  connection: Connection;
  // Settings' confirm dialog, so every card asks the same way.
  confirm: (message: string) => Promise<boolean>;
  showMessage: (message: string, isError?: boolean) => void;
}>();
const emit = defineEmits<{
  // A sync was started or photos were cleared: the gallery should refresh.
  synced: [source: string];
  deleted: [id: number];
}>();

const store = useConnectionsStore();
const sourceLabel = computed(
  () => SOURCE_LABELS[props.connection.source] || props.connection.source
);
const isUnsplash = computed(() => props.connection.source === 'unsplash');
const keyLabel = computed(() =>
  isUnsplash.value ? 'Unsplash Access Key' : 'Pexels API Key'
);
const keyHint = computed(() =>
  isUnsplash.value ? "Your app's Access Key — the Secret Key is not needed" : ''
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

// --- API key ---
const editingKey = ref(false);
const apiKey = ref('');
const cancelEditKey = () => {
  editingKey.value = false;
  apiKey.value = '';
};

// Store the key (an empty field keeps the stored one), then verify it with a
// one-photo search.
const connect = async () => {
  loading.value = true;
  try {
    if (apiKey.value) {
      // A connection the migration created without a key is disabled; a
      // working key switches it back on.
      await store.update(props.connection.id, {
        config: { api_key: apiKey.value },
        enabled: true,
      });
    }
    await store.test(props.connection.id);
    apiKey.value = '';
    editingKey.value = false;
    props.showMessage('Connection Successful!');
    await loadAll();
  } catch (e: any) {
    props.showMessage(
      'Connection Failed: ' +
        (e.response?.data?.error || e.message || 'Unknown error'),
      true
    );
  } finally {
    loading.value = false;
  }
};

const remove = async () => {
  if (
    !(await props.confirm(
      `Delete the connection "${props.connection.name}"? Its topics and synced photos are removed; frames using them fall back to the remaining ones.`
    ))
  )
    return;
  try {
    await store.remove(props.connection.id);
    emit('deleted', props.connection.id);
    emit('synced', props.connection.source);
  } catch (e: any) {
    props.showMessage(
      'Failed to delete: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- topics ---
const topics = ref<string[]>([]);
// Topics save on every add/remove but do not import until the user syncs or
// is prompted on leaving the tab.
const dirty = ref(false);

const loadTopics = async () => {
  try {
    topics.value = await store.topics(props.connection.id);
  } catch (e) {
    // Non-fatal: the list starts empty.
  }
};

const saveTopics = async (next: string[]) => {
  try {
    topics.value = await store.setTopics(props.connection.id, next);
    dirty.value = true;
    await store.fetchCount(props.connection.id);
  } catch (e: any) {
    props.showMessage(
      'Failed to save topics: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- options (randomize, auto-sync) ---
const randomizeResults = ref(false);
const autoSyncEnabled = ref(false);
const autoSyncInterval = ref(60);
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
  randomizeResults.value = !!cfg.randomize_results;
  autoSyncEnabled.value = !!cfg.auto_sync_enabled;
  autoSyncInterval.value = cfg.auto_sync_interval_minutes || 60;
};
const saveOptions = async () => {
  try {
    await store.update(props.connection.id, {
      config: {
        randomize_results: randomizeResults.value,
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
    props.showMessage(
      'Sync Failed: ' + (e.response?.data?.error || 'Unknown error'),
      true
    );
  } finally {
    loading.value = false;
  }
};

// Clear = delete the photos; the topics stay and the next sync refills them.
const clearPhotos = async () => {
  if (
    !(await props.confirm(
      `Delete all photos synced from ${props.connection.name}? Your topics stay, but their photos are removed.`
    ))
  )
    return;
  try {
    await store.clearPhotosKeepSync(props.connection.id);
    props.showMessage('All photos deleted.');
    emit('synced', props.connection.source);
  } catch (e: any) {
    props.showMessage(
      'Failed to delete photos: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Settings calls this when the user leaves the tab with unsynced topics.
const promptIfDirty = async () => {
  if (!dirty.value) return;
  dirty.value = false; // ask once
  if (
    await props.confirm(
      `You changed the topics of ${props.connection.name}. Sync now to import photos for them?`
    )
  ) {
    await syncNow();
  }
};
defineExpose({ promptIfDirty });

const loadAll = async () => {
  await store.fetchCount(props.connection.id);
  await loadTopics();
};

onMounted(async () => {
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
