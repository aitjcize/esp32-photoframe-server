<template>
  <!-- Persistent: only Save and Not now close it, so whoever asked always
       gets an answer (see useFramePasswordPrompt). -->
  <v-dialog v-model="state.show" max-width="460" persistent>
    <v-card v-if="state.device">
      <v-card-title class="text-h6 text-wrap">{{ text.title }}</v-card-title>
      <v-card-text>
        <div class="text-body-2">{{ text.body }}</div>
        <div
          v-if="state.device.auth_failed_at"
          class="text-caption text-medium-emphasis mt-1"
        >
          Refusing since {{ formatSince(state.device.auth_failed_at) }}
        </div>
        <v-text-field
          v-model="state.password"
          label="Frame password"
          type="password"
          autocomplete="current-password"
          maxlength="63"
          variant="outlined"
          density="compact"
          class="mt-4"
          autofocus
          hide-details="auto"
          :error-messages="state.error || []"
          :disabled="state.saving"
          @keyup.enter="save"
        ></v-text-field>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn variant="text" :disabled="state.saving" @click="dismiss"
          >Not now</v-btn
        >
        <v-btn
          color="primary"
          :loading="state.saving"
          :disabled="!state.password"
          @click="save"
          >Save</v-btn
        >
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useFramePasswordPrompt } from '../composables/useFramePasswordPrompt';
import { framePasswordPromptText } from '../utils/frameAuth';

const { state, dismiss, save } = useFramePasswordPrompt();

const text = computed(() =>
  state.device ? framePasswordPromptText(state.device) : { title: '', body: '' }
);

const formatSince = (iso: string) => {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? 'unknown' : d.toLocaleString();
};
</script>
