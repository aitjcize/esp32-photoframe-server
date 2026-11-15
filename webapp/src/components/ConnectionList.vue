<template>
  <v-card-text>
    <div v-if="!store.loaded && store.loading" class="text-center py-6">
      <v-progress-circular indeterminate color="primary"></v-progress-circular>
    </div>

    <template v-else>
      <v-alert
        v-if="connections.length === 0"
        type="info"
        variant="tonal"
        density="compact"
        class="mb-4"
      >
        <span v-if="source === 'immich'">
          Connect an Immich server to sync its albums and collections. You can
          add more than one.
        </span>
        <span v-else-if="source === 'google_photos'">
          Connect a Google account to pick photos from its Google Photos
          library. You can add more than one account.
        </span>
        <span v-else>
          Connect a Synology Photos account to sync its albums. You can add more
          than one NAS or account.
        </span>
      </v-alert>

      <component
        :is="source === 'google_photos' ? GoogleConnectionCard : ConnectionCard"
        v-for="c in connections"
        :key="c.id"
        :ref="(el: any) => setCardRef(c.id, el)"
        :connection="c"
        :confirm="confirm"
        :show-message="showMessage"
        :before-connect="beforeConnect"
        :focus="c.id === focusId"
        @synced="emit('synced', $event)"
        @deleted="cardRefs.delete($event)"
      ></component>

      <v-btn
        color="primary"
        variant="tonal"
        prepend-icon="mdi-plus"
        :loading="adding"
        @click="add"
      >
        {{ connections.length ? `Add another ${label}` : `Add ${label}` }}
      </v-btn>
    </template>
  </v-card-text>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import ConnectionCard from './ConnectionCard.vue';
import GoogleConnectionCard from './GoogleConnectionCard.vue';
import { SOURCE_LABELS, useConnectionsStore } from '../stores/connections';

const props = defineProps<{
  source: 'immich' | 'synology_photos' | 'google_photos';
  confirm: (message: string) => Promise<boolean>;
  showMessage: (message: string, isError?: boolean) => void;
  // Google cards: runs before leaving for Google's consent page.
  beforeConnect?: () => Promise<void>;
  // The connection to scroll to (the OAuth callback names the one it
  // connected).
  focusId?: number;
}>();
const emit = defineEmits<{ synced: [source: string] }>();

const store = useConnectionsStore();
const connections = computed(() => store.bySource(props.source));
const label = computed(() => SOURCE_LABELS[props.source] || props.source);

const adding = ref(false);
const add = async () => {
  adding.value = true;
  try {
    const n = connections.value.length;
    await store.create({
      source: props.source,
      name: n === 0 ? label.value : `${label.value} ${n + 1}`,
    });
  } catch (e: any) {
    props.showMessage(
      'Failed to add connection: ' + (e.response?.data?.error || e.message),
      true
    );
  } finally {
    adding.value = false;
  }
};

// Card instances by connection id, so Settings can ask each one to offer a
// sync when the user leaves with an unsynced album selection.
const cardRefs = new Map<number, any>();
const setCardRef = (id: number, el: any) => {
  if (el) cardRefs.set(id, el);
  else cardRefs.delete(id);
};
const promptIfDirty = async () => {
  for (const card of cardRefs.values()) {
    await card.promptIfDirty?.();
  }
};
defineExpose({ promptIfDirty });

onMounted(() => {
  if (!store.loaded) store.fetch();
});
</script>
