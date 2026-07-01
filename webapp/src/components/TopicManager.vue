<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-1">
      <h3 class="text-subtitle-1 font-weight-bold">{{ title }}</h3>
      <span class="text-caption text-grey">{{ store.count }} photos</span>
    </div>
    <div v-if="hint" class="text-caption text-grey mb-2">{{ hint }}</div>

    <!-- Add topic -->
    <div class="d-flex ga-2 mb-3">
      <v-text-field
        v-model="newTopic"
        placeholder="Add a topic (e.g. mountains)"
        variant="outlined"
        density="compact"
        hide-details
        @keyup.enter="addTopic"
      ></v-text-field>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="addTopic"
        >Add</v-btn
      >
    </div>

    <!-- Current topics -->
    <v-card variant="outlined" class="mb-3 pa-3">
      <div v-if="topics.length" class="d-flex flex-wrap ga-2">
        <v-chip
          v-for="t in topics"
          :key="t"
          closable
          color="primary"
          variant="tonal"
          @click:close="removeTopic(t)"
        >
          {{ t }}
        </v-chip>
      </div>
      <div v-else class="text-caption text-grey">
        No topics yet. Add one above to create an album.
      </div>
    </v-card>

    <!-- Auto sync -->
    <v-row class="mt-0">
      <v-col cols="12" md="6">
        <v-checkbox
          :model-value="autoSyncEnabled"
          label="Auto Sync Topics"
          color="primary"
          density="compact"
          hide-details
          @update:model-value="emit('update:autoSyncEnabled', !!$event)"
        ></v-checkbox>
      </v-col>
      <v-col cols="12" md="6">
        <v-select
          :model-value="autoSyncInterval"
          :items="autoSyncOptions"
          item-title="title"
          item-value="value"
          label="Auto Sync Interval"
          variant="outlined"
          density="compact"
          :disabled="!autoSyncEnabled"
          hint="How often to refresh photos for the selected topics"
          persistent-hint
          @update:model-value="emit('update:autoSyncInterval', $event)"
        ></v-select>
      </v-col>
    </v-row>

    <div class="d-flex flex-wrap ga-2 mt-4">
      <v-btn color="primary" :loading="store.loading" @click="onSave"
        >Save Topics</v-btn
      >
      <v-btn
        color="primary"
        variant="tonal"
        :loading="store.loading"
        @click="emit('sync')"
        >Sync Now</v-btn
      >
      <v-btn color="warning" @click="emit('clear')">Clear All Photos</v-btn>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue';

const props = defineProps<{
  title: string;
  hint?: string;
  // A topic-source store instance (createSourceStore). Read count/loading and
  // seed topics from its syncedAlbums.
  store: any;
  autoSyncEnabled: boolean;
  autoSyncInterval: number;
  autoSyncOptions: { title: string; value: number }[];
}>();

const emit = defineEmits<{
  save: [topics: string[]];
  sync: [];
  clear: [];
  'update:autoSyncEnabled': [boolean];
  'update:autoSyncInterval': [number];
}>();

const topics = ref<string[]>([]);
const newTopic = ref('');

// Seed the topic list from the store's synced albums (external_id = topic text).
const seedFromStore = () => {
  topics.value = (props.store.syncedAlbums || [])
    .map((a: any) => a.external_id)
    .filter((t: string) => !!t);
};

onMounted(async () => {
  try {
    await props.store.fetchSyncedAlbums();
    await props.store.fetchCount();
  } catch (e) {
    // Non-fatal: start with an empty topic list.
  }
  seedFromStore();
});

// Re-seed if the synced albums change out from under us (e.g. after clear).
watch(
  () => props.store.syncedAlbums,
  () => seedFromStore()
);

const addTopic = () => {
  const t = newTopic.value.trim();
  if (!t) return;
  if (!topics.value.some((x) => x.toLowerCase() === t.toLowerCase())) {
    topics.value.push(t);
  }
  newTopic.value = '';
};

const removeTopic = (t: string) => {
  topics.value = topics.value.filter((x) => x !== t);
};

const onSave = () => {
  // Fold a partially-typed topic in so it isn't silently lost.
  addTopic();
  emit('save', [...topics.value]);
};
</script>
