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
        title="Remove this account"
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
          Signed in<span v-if="connection.config.email">
            as {{ connection.config.email }}</span
          >
          <span v-if="calendars.length" class="text-grey">
            · {{ calendars.length }} calendar{{
              calendars.length === 1 ? '' : 's'
            }}</span
          >
        </v-alert>

        <v-alert
          v-if="notAuthorized"
          type="warning"
          variant="tonal"
          density="compact"
          class="mb-4"
        >
          This account was signed in without calendar access. Sign in again to
          grant it.
          <template #append>
            <v-btn size="small" variant="text" @click="connect"
              >Sign in again</v-btn
            >
          </template>
        </v-alert>

        <div v-else-if="calendars.length" class="mb-4">
          <div class="text-caption text-grey mb-1">
            Calendars frames can show (choose one per frame in the device
            dialog):
          </div>
          <v-chip
            v-for="cal in calendars"
            :key="cal.id"
            size="small"
            variant="outlined"
            class="mr-1 mb-1"
            >{{ cal.summary }}</v-chip
          >
        </div>

        <div class="d-flex flex-wrap ga-2">
          <v-btn color="error" variant="text" @click="disconnect"
            >Disconnect</v-btn
          >
        </div>
      </div>

      <div v-else>
        <div class="text-body-2 mb-3">
          Sign in with the Google account whose calendar your frames should
          show. You will be sent to Google and back.
        </div>
        <v-btn
          color="primary"
          prepend-icon="mdi-google"
          :loading="connecting"
          @click="connect"
          >Sign in with Google</v-btn
        >
      </div>
    </v-card-text>
  </v-card>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { useConnectionsStore, type Connection } from '../stores/connections';
import { inUseMessage } from '../utils/inUse';

// One Google account whose calendar frames can show: sign in, see which
// calendars it offers, disconnect or remove it.
const props = defineProps<{
  connection: Connection;
  confirm: (message: string) => Promise<boolean>;
  showMessage: (message: string, isError?: boolean) => void;
  // A notice with only OK, for a removal the server refused because a frame
  // is bound to it.
  inform?: (message: string, title?: string) => Promise<void>;
  // Runs before the browser leaves for Google, so unsaved OAuth app
  // credentials are stored first.
  beforeConnect?: () => Promise<void>;
  // Scroll this card into view: the OAuth callback lands on it.
  focus?: boolean;
}>();
const emit = defineEmits<{ deleted: [id: number] }>();

const store = useConnectionsStore();
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
      `Disconnect ${props.connection.name}? Frames showing its calendar fall back to another connected account until you sign in again.`
    ))
  )
    return;
  try {
    await store.logout(props.connection.id);
    calendars.value = [];
    props.showMessage('Disconnected.');
  } catch (e: any) {
    props.showMessage(
      'Error disconnecting: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

const remove = async () => {
  if (!(await props.confirm(`Remove the account "${props.connection.name}"?`)))
    return;
  try {
    await store.remove(props.connection.id);
    emit('deleted', props.connection.id);
  } catch (e: any) {
    const inUse = inUseMessage(
      e,
      `The account "${props.connection.name}"`,
      'Pick another account for their calendar in the device dialog first, then try again.'
    );
    if (inUse) {
      if (props.inform) await props.inform(inUse, 'Account in use');
      else props.showMessage(inUse, true);
      return;
    }
    props.showMessage(
      'Failed to remove: ' + (e.response?.data?.error || e.message),
      true
    );
  }
};

// --- calendars ---
const calendars = ref<any[]>([]);
const notAuthorized = ref(false);
const loadCalendars = async () => {
  if (!props.connection.connected) {
    calendars.value = [];
    return;
  }
  try {
    calendars.value = await store.calendars(props.connection.id);
    notAuthorized.value = false;
  } catch (e: any) {
    calendars.value = [];
    notAuthorized.value = e.response?.status === 403;
  }
};

onMounted(async () => {
  await loadCalendars();
  if (props.focus) {
    card.value?.$el?.scrollIntoView?.({ behavior: 'smooth', block: 'center' });
  }
});

watch(
  () => props.connection.connected,
  () => loadCalendars()
);
</script>

<style scoped>
.connection-name-field {
  max-width: 320px;
}
.connection-card--focus {
  border-color: rgb(var(--v-theme-primary));
}
</style>
