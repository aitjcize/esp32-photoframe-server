<template>
  <div>
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
          <v-alert type="info" variant="tonal" class="mb-4" density="compact">
            Leave new password fields blank if you only want to change the
            username. Current password is required for any change.
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

    <div class="d-flex align-center justify-space-between mb-6">
      <div class="mr-4">
        <div class="text-body-2 font-weight-medium">JWT signing secret</div>
        <div class="text-caption text-grey">
          Regenerate to fully rotate the secret. This signs you out and
          invalidates every device token — each frame will need a new token
          generated and applied.
        </div>
      </div>
      <v-btn
        color="error"
        variant="outlined"
        size="small"
        :loading="rotatingSecret"
        @click="regenerateJWTSecret"
        >Regenerate</v-btn
      >
    </div>

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
      <v-card-title class="text-subtitle-1">Generate New Token</v-card-title>
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
              ...devices.map((d: any) => ({ title: d.name, value: d.id })),
            ]"
            label="Bind to Device"
            variant="outlined"
            density="compact"
            hide-details
            :style="smAndDown ? '' : 'max-width: 220px'"
          ></v-select>
          <v-btn color="primary" :block="smAndDown" @click="generateToken"
            >Generate</v-btn
          >
        </div>
      </v-card-text>
    </v-card>

    <h4 class="text-subtitle-2 mb-2">Active Tokens</h4>
    <!-- Table on tablet/desktop -->
    <v-table v-if="!smAndDown" density="comfortable" class="border rounded">
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
                ...devices.map((d: any) => ({ title: d.name, value: d.id })),
              ]"
              variant="plain"
              density="compact"
              hide-details
              style="max-width: 180px; font-size: inherit"
              @update:model-value="(val: any) => updateTokenDevice(token.id, val)"
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
            No active tokens found. Create one above to connect a device.
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
              ...devices.map((d: any) => ({ title: d.name, value: d.id })),
            ]"
            label="Bound device"
            variant="outlined"
            density="compact"
            hide-details
            class="mt-2"
            @update:model-value="(val: any) => updateTokenDevice(token.id, val)"
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

    <ConfirmDialog ref="confirmDialog" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useDisplay } from 'vuetify';
import { useAuthStore } from '../stores/auth';
import { useSnackbar } from '../composables/useSnackbar';
import { getApiError } from '../utils/errors';
import {
  updateAccount,
  listSessions,
  revokeSession,
  rotateJWTSecret,
} from '../api';
import type { Device } from '../api';
import ConfirmDialog from './ConfirmDialog.vue';

defineProps<{ devices: Device[] }>();

const { smAndDown } = useDisplay();
const authStore = useAuthStore();
const { showMessage } = useSnackbar();
const confirmDialog = ref();

const generatedToken = ref('');
const newTokenName = ref('');
const newTokenDeviceId = ref<number | null>(null);

const copyToken = async () => {
  try {
    await navigator.clipboard.writeText(generatedToken.value);
    showMessage('Token copied to clipboard!');
  } catch (e) {
    showMessage(
      'Failed to copy token automatically. Please copy manually.',
      true
    );
  }
};

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
  } catch (e) {
    showMessage('Failed to generate token: ' + getApiError(e), true);
  }
};

const updateTokenDevice = async (tokenId: number, deviceId: number | null) => {
  try {
    await authStore.updateTokenDevice(tokenId, deviceId);
    showMessage('Token device binding updated');
  } catch (e) {
    showMessage('Failed to update token: ' + getApiError(e), true);
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
  } catch (e) {
    showMessage('Failed: ' + getApiError(e), true);
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
  } catch (e) {
    showMessage('Failed: ' + getApiError(e), true);
  }
};

const rotatingSecret = ref(false);

const regenerateJWTSecret = async () => {
  if (
    !(await confirmDialog.value.open(
      'Regenerate the JWT signing secret? This signs you out and invalidates ALL device tokens — every frame will need a new token generated and applied. Continue?'
    ))
  )
    return;
  rotatingSecret.value = true;
  try {
    await rotateJWTSecret();
    showMessage('JWT secret regenerated. Signing out…');
    // The current session token is now invalid; sign out so the user re-logs in.
    setTimeout(() => authStore.logout(), 800);
  } catch (e) {
    showMessage('Failed to regenerate secret: ' + getApiError(e), true);
  } finally {
    rotatingSecret.value = false;
  }
};

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
  } catch (e) {
    showMessage('Failed: ' + getApiError(e), true);
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

onMounted(() => {
  loadSessions();
  authStore.fetchTokens();
});
</script>
