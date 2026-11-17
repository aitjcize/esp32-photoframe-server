<template>
  <v-card
    ref="card"
    variant="outlined"
    class="mb-4 connection-card"
    :class="{ 'connection-card--focus': focus }"
  >
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
      <div v-if="connection.connected">
        <v-alert
          type="success"
          variant="tonal"
          class="mb-4"
          density="compact"
          icon="mdi-check-circle"
        >
          Connected to Google Photos<span v-if="connection.config.email">
            as {{ connection.config.email }}</span
          >
          <span class="text-grey"> · {{ count }} photos imported</span>
        </v-alert>

        <div class="text-caption text-grey mb-3">
          Google only hands out the photos you pick: use the picker to add
          photos from this account. Imported photos stay on this server until
          you clear them.
        </div>

        <div class="d-flex flex-wrap ga-2">
          <v-btn
            color="primary"
            prepend-icon="mdi-google"
            :loading="galleryStore.loading"
            :disabled="galleryStore.loading"
            @click="pickPhotos"
            >Add Photos via Google</v-btn
          >
          <v-btn color="warning" @click="clearPhotos">Clear All Photos</v-btn>
          <v-btn color="error" variant="text" @click="disconnect"
            >Disconnect</v-btn
          >
        </div>
      </div>

      <div v-else>
        <div class="text-body-2 mb-3">
          <span v-if="count > 0">
            {{ count }} photos imported earlier are still available. Connect the
            account again to add more.
          </span>
          <span v-else>
            Sign in with the Google account whose photos this frame should show.
            You will be sent to Google and back.
          </span>
        </div>
        <div class="d-flex flex-wrap ga-2">
          <v-btn
            color="primary"
            prepend-icon="mdi-google"
            :loading="connecting"
            @click="connect"
            >Connect Google account</v-btn
          >
          <v-btn v-if="count > 0" color="warning" @click="clearPhotos"
            >Clear All Photos</v-btn
          >
        </div>
      </div>
    </v-card-text>
  </v-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useConnectionsStore, type Connection } from '../stores/connections';
import { inUseMessage } from '../utils/inUse';
import { useGalleryStore } from '../stores/gallery';

const props = defineProps<{
  connection: Connection;
  confirm: (message: string) => Promise<boolean>;
  showMessage: (message: string, isError?: boolean) => void;
  // A notice with only OK, for a removal the server refused because a frame
  // is bound to it.
  inform?: (message: string, title?: string) => Promise<void>;
  // Runs before the browser leaves for Google, so unsaved OAuth app
  // credentials typed above the cards are stored first.
  beforeConnect?: () => Promise<void>;
  // Scroll this card into view: the OAuth callback lands on it.
  focus?: boolean;
}>();
const emit = defineEmits<{
  // Photos were added or cleared: the gallery should refresh on this source.
  synced: [source: string];
  deleted: [id: number];
}>();

const store = useConnectionsStore();
const galleryStore = useGalleryStore();
const count = computed(() => store.counts[props.connection.id] ?? 0);
const card = ref<any>(null);

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

// --- OAuth ---
const connecting = ref(false);
const connect = async () => {
  connecting.value = true;
  try {
    await props.beforeConnect?.();
    window.location.href = await store.googleLoginUrl(props.connection.id);
  } catch (e: any) {
    props.showMessage(
      'Failed to connect: ' + (e.response?.data?.error || e.message),
      true
    );
    connecting.value = false;
  }
};

const disconnect = async () => {
  if (
    !(await props.confirm(
      `Disconnect ${props.connection.name}? The photos already imported stay; connect again to add more.`
    ))
  )
    return;
  try {
    await store.logout(props.connection.id);
    props.showMessage('Disconnected.');
  } catch (e: any) {
    props.showMessage(
      'Error disconnecting: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const remove = async () => {
  if (
    !(await props.confirm(
      `Delete the connection "${props.connection.name}"? Its imported photos are removed from this server.`
    ))
  )
    return;
  try {
    await store.remove(props.connection.id);
    emit('deleted', props.connection.id);
    emit('synced', props.connection.source);
  } catch (e: any) {
    const inUse = inUseMessage(e, `The connection "${props.connection.name}"`);
    if (inUse) {
      if (props.inform) await props.inform(inUse, 'Connection in use');
      else props.showMessage(inUse, true);
      return;
    }
    props.showMessage(
      'Failed to delete: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- photos ---
const pickPhotos = async () => {
  emit('synced', props.connection.source);
  await galleryStore.startPicker(props.connection.id);
};

// The picker import runs in the gallery store; when it finishes the count
// here is stale.
watch(
  () => galleryStore.loading,
  (loading, was) => {
    if (was && !loading) store.fetchCount(props.connection.id);
  }
);

const clearPhotos = async () => {
  if (
    !(await props.confirm(
      `Delete all photos imported from ${props.connection.name}?`
    ))
  )
    return;
  try {
    await store.clearPhotos(props.connection.id);
    props.showMessage('All photos deleted.');
    emit('synced', props.connection.source);
  } catch (e: any) {
    props.showMessage(
      'Failed to delete photos: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// Nothing to sync on leaving the tab; ConnectionList calls this on every card.
const promptIfDirty = async () => {};
defineExpose({ promptIfDirty });

onMounted(async () => {
  await store.fetchCount(props.connection.id);
  if (props.focus) {
    card.value?.$el?.scrollIntoView?.({ behavior: 'smooth', block: 'center' });
  }
});
</script>

<style scoped>
.connection-name-field {
  max-width: 320px;
}
.connection-card--focus {
  border-color: rgb(var(--v-theme-primary));
}
</style>
