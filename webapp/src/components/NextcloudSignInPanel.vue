<template>
  <v-alert type="info" variant="tonal" density="compact" class="mt-4">
    <div>
      Waiting for you to sign in to Nextcloud and grant access. A new tab should
      have opened; you can also scan the code with your phone, or copy the link
      into any browser.
    </div>
    <div v-if="loginUrl" class="nc-signin mt-3">
      <img
        v-if="qrDataUrl"
        :src="qrDataUrl"
        alt="QR code of the Nextcloud sign-in link"
        class="nc-qr"
        width="168"
        height="168"
      />
      <div class="nc-link">
        <v-text-field
          ref="linkField"
          :model-value="loginUrl"
          label="Sign-in link"
          variant="outlined"
          density="compact"
          readonly
          hide-details
          @focus="selectLink"
        >
          <template #append-inner>
            <v-btn
              :icon="copied ? 'mdi-check' : 'mdi-content-copy'"
              size="small"
              variant="text"
              :title="copied ? 'Copied' : 'Copy link'"
              @click="copyLink"
            ></v-btn>
          </template>
        </v-text-field>
        <div class="text-caption mt-1">
          <span v-if="copyFailed"
            >Could not copy automatically: the link is selected, press Ctrl+C
            (⌘C).
          </span>
          <a v-else :href="loginUrl" target="_blank" rel="noopener"
            >Open in a new tab</a
          >
        </div>
      </div>
    </div>
  </v-alert>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import QRCode from 'qrcode';

// The Nextcloud Login Flow v2 page while a sign-in is waiting: as a QR code
// for signing in on a phone, and as a link to copy into another browser.
// The flow completes on the server whichever device grants access.
const props = defineProps<{ loginUrl: string }>();

const qrDataUrl = ref('');
const copied = ref(false);
const copyFailed = ref(false);
const linkField = ref<any>(null);

watch(
  () => props.loginUrl,
  async (url) => {
    copied.value = false;
    copyFailed.value = false;
    qrDataUrl.value = '';
    if (!url) return;
    try {
      // Rendered in the browser: the link never leaves this page.
      const data = await QRCode.toDataURL(url, { margin: 1, width: 336 });
      if (url === props.loginUrl) qrDataUrl.value = data;
    } catch {
      // No code; the link still works.
    }
  },
  { immediate: true }
);

const linkInput = (): HTMLInputElement | null =>
  linkField.value?.$el?.querySelector('input') ?? null;

const selectLink = () => {
  linkInput()?.select();
};

// The Clipboard API needs a secure context (HTTPS or localhost); a dashboard
// on plain HTTP in the LAN falls back to copying the selected field.
const copyLink = async () => {
  copyFailed.value = false;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(props.loginUrl);
    } else {
      selectLink();
      if (!document.execCommand('copy')) throw new Error('copy refused');
    }
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    selectLink();
    copyFailed.value = true;
  }
};
</script>

<style scoped>
.nc-signin {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: flex-start;
}
.nc-qr {
  background: #fff;
  border-radius: 4px;
  flex: none;
}
.nc-link {
  flex: 1 1 220px;
  min-width: 0;
}
</style>
