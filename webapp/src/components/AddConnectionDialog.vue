<template>
  <v-dialog
    :model-value="modelValue"
    max-width="560px"
    :persistent="loading"
    @update:model-value="onToggle"
  >
    <v-card>
      <v-card-title class="d-flex align-center ga-2">
        <v-btn
          v-if="source"
          icon="mdi-arrow-left"
          size="small"
          variant="text"
          title="Back"
          :disabled="loading"
          @click="back"
        ></v-btn>
        <span>{{ source ? `Add ${label}` : 'Add a photo source' }}</span>
      </v-card-title>

      <!-- Step 1: which provider -->
      <v-card-text v-if="!source" class="pt-0">
        <v-list lines="two" density="comfortable">
          <v-list-item
            v-for="s in sources || CONNECTION_SOURCES"
            :key="s"
            :title="SOURCE_LABELS[s]"
            :subtitle="SOURCE_DESCRIPTIONS[s]"
            :prepend-icon="SOURCE_ICONS[s]"
            rounded
            @click="choose(s)"
          >
            <template #append>
              <v-icon icon="mdi-chevron-right"></v-icon>
            </template>
          </v-list-item>
        </v-list>
      </v-card-text>

      <!-- Step 2: name + credentials -->
      <v-card-text v-else>
        <v-form @submit.prevent="connect">
          <v-text-field
            v-model="name"
            label="Name"
            variant="outlined"
            class="mb-2"
            hint="How this source is listed, e.g. “Home NAS” or “Mom's photos”"
            persistent-hint
            autofocus
          ></v-text-field>

          <template v-if="source === 'immich'">
            <v-text-field
              v-model="form.url"
              label="Immich Server URL"
              placeholder="http://192.168.1.10:2283"
              variant="outlined"
              class="mb-2 mt-4"
            ></v-text-field>
            <v-text-field
              v-model="form.api_key"
              label="API Key"
              type="password"
              variant="outlined"
              class="mb-2"
            ></v-text-field>
          </template>

          <template v-else-if="source === 'synology_photos'">
            <v-text-field
              v-model="form.url"
              label="NAS URL"
              placeholder="https://192.168.1.10:5001"
              variant="outlined"
              class="mb-2 mt-4"
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
            ></v-text-field>
            <v-checkbox
              v-model="form.skip_cert"
              label="Skip Certificate Verification (Insecure)"
              color="primary"
              density="compact"
              hide-details
            ></v-checkbox>
            <v-text-field
              v-if="needsOtp"
              v-model="form.otp_code"
              label="OTP Code (2FA)"
              placeholder="6-digit code"
              variant="outlined"
              class="mb-2 mt-2"
              autofocus
            ></v-text-field>
          </template>

          <template v-else-if="isGoogle">
            <v-alert
              v-if="!googleReady"
              type="warning"
              variant="tonal"
              density="compact"
              class="mt-4"
            >
              Enter the Google API client ID and secret under
              <b>Settings → System → Google</b> first.
              <template #append>
                <v-btn size="small" variant="text" @click="emit('openSystem')"
                  >Open</v-btn
                >
              </template>
            </v-alert>
            <div v-else class="text-body-2 text-grey mt-4">
              You will be sent to Google to sign in with the account whose
              {{ source === 'google_calendar' ? 'calendar' : 'photos' }} this
              {{ source === 'google_calendar' ? 'connection' : 'source' }}
              should offer, and back here afterwards.
            </div>
          </template>

          <template v-else>
            <v-text-field
              v-model="form.api_key"
              :label="
                source === 'unsplash' ? 'Unsplash Access Key' : 'Pexels API Key'
              "
              :hint="
                source === 'unsplash'
                  ? 'Your app\'s Access Key — the Secret Key is not needed'
                  : ''
              "
              :persistent-hint="source === 'unsplash'"
              type="password"
              variant="outlined"
              class="mb-2 mt-4"
            ></v-text-field>
          </template>
        </v-form>

        <v-alert
          v-if="error"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-4"
        >
          {{ error }}
        </v-alert>
      </v-card-text>

      <v-card-actions v-if="source">
        <v-spacer></v-spacer>
        <v-btn variant="text" :disabled="loading" @click="cancel">Cancel</v-btn>
        <v-btn
          color="primary"
          :disabled="!canConnect"
          :loading="loading"
          @click="connect"
        >
          {{ isGoogle ? 'Sign in with Google' : 'Connect' }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import {
  CONNECTION_SOURCES,
  GOOGLE_SOURCES,
  SOURCE_DESCRIPTIONS,
  SOURCE_ICONS,
  SOURCE_LABELS,
  useConnectionsStore,
  type Connection,
} from '../stores/connections';

// Adds one connection: pick the provider, name it, enter its credentials,
// connect. The row is created on the first Connect attempt and deleted again
// if the user cancels before it ever connected, so a typo leaves nothing
// behind.
const props = defineProps<{
  modelValue: boolean;
  // The providers to offer; the photo sources by default. With exactly one
  // the provider step is skipped.
  sources?: string[];
  // Whether the Google OAuth client is configured (Google sign-ins need it).
  googleReady: boolean;
  // Runs before the browser leaves for Google's consent page.
  beforeGoogleConnect?: () => Promise<void>;
}>();
const emit = defineEmits<{
  'update:modelValue': [open: boolean];
  // The connection is created and verified (Google: created; it is verified
  // when the browser comes back from Google).
  created: [connection: Connection];
  // The user wants the System tab (to enter the Google client).
  openSystem: [];
}>();

const store = useConnectionsStore();

const source = ref<string>('');
const label = computed(() => SOURCE_LABELS[source.value] || source.value);
const isGoogle = computed(() => GOOGLE_SOURCES.includes(source.value));
const name = ref('');
const form = reactive({
  url: '',
  api_key: '',
  account: '',
  password: '',
  skip_cert: false,
  otp_code: '',
});
const loading = ref(false);
const error = ref('');
const needsOtp = ref(false);
// The row created by a Connect attempt that did not verify yet.
const createdId = ref<number | null>(null);

const defaultName = (s: string) => {
  const n = store.bySource(s).length;
  return n === 0 ? SOURCE_LABELS[s] : `${SOURCE_LABELS[s]} ${n + 1}`;
};

const choose = (s: string) => {
  source.value = s;
  name.value = defaultName(s);
  error.value = '';
};

const reset = () => {
  source.value = '';
  name.value = '';
  form.url = '';
  form.api_key = '';
  form.account = '';
  form.password = '';
  form.skip_cert = false;
  form.otp_code = '';
  error.value = '';
  needsOtp.value = false;
  createdId.value = null;
};

const canConnect = computed(() => {
  if (loading.value) return false;
  switch (source.value) {
    case 'immich':
      return !!form.url && !!form.api_key;
    case 'synology_photos':
      return !!form.url && !!form.account && !!form.password;
    case 'google_photos':
    case 'google_calendar':
      return props.googleReady;
    case 'unsplash':
    case 'pexels':
      return !!form.api_key;
    default:
      return false;
  }
});

const configFor = (): Record<string, any> => {
  switch (source.value) {
    case 'immich':
      return { url: form.url, api_key: form.api_key };
    case 'synology_photos':
      return {
        url: form.url,
        account: form.account,
        password: form.password,
        skip_cert: form.skip_cert,
      };
    case 'unsplash':
    case 'pexels':
      return { api_key: form.api_key };
    default:
      return {};
  }
};

const connect = async () => {
  if (!canConnect.value) return;
  loading.value = true;
  error.value = '';
  const finalName = name.value.trim() || defaultName(source.value);
  try {
    let id = createdId.value;
    if (id == null) {
      const c = await store.create({ source: source.value, name: finalName });
      id = c.id;
      createdId.value = id;
    } else if (finalName !== store.byId(id)?.name) {
      await store.update(id, { name: finalName });
    }

    if (isGoogle.value) {
      await props.beforeGoogleConnect?.();
      const url = await store.googleLoginUrl(id);
      const created = store.byId(id);
      if (created) emit('created', created);
      window.location.href = url;
      return; // the page is leaving; keep the button spinning
    }

    await store.update(id, { config: configFor() });
    const conn = await store.test(id, form.otp_code);
    createdId.value = null;
    emit('created', conn);
    emit('update:modelValue', false);
    reset();
  } catch (e: any) {
    const msg = e.response?.data?.error || e.message || 'Unknown error';
    if (
      source.value === 'synology_photos' &&
      String(msg).includes('code: 403')
    ) {
      needsOtp.value = true;
      error.value = '2FA required: enter the OTP code and connect again.';
    } else {
      error.value = 'Connection failed: ' + msg;
    }
  } finally {
    loading.value = false;
  }
};

// Leaving before the connection ever worked removes the row again.
const discardCreated = async () => {
  const id = createdId.value;
  createdId.value = null;
  if (id == null) return;
  try {
    await store.remove(id);
  } catch {
    // A row that stays behind shows as "Not connected" and can be deleted.
  }
};

// Back to the provider list; the fields start over.
const back = async () => {
  await discardCreated();
  reset();
};

const cancel = async () => {
  await discardCreated();
  emit('update:modelValue', false);
  reset();
};

const onToggle = (open: boolean) => {
  if (!open) cancel();
  else emit('update:modelValue', true);
};

// One provider only (the Calendar tab): straight to its form.
watch(
  () => props.modelValue,
  (open) => {
    if (open && !source.value && props.sources?.length === 1) {
      choose(props.sources[0]);
    }
  },
  { immediate: true }
);
</script>
