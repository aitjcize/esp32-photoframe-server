<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-1">
      <h3 class="text-subtitle-1 font-weight-bold">{{ title }}</h3>
      <span class="text-caption text-grey">{{ count }} photos</span>
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

    <!-- Randomize results -->
    <v-checkbox
      :model-value="randomizeResults"
      label="Randomize results"
      color="primary"
      density="compact"
      hide-details="auto"
      hint="Sample random pages of the search results on each sync, so the albums rotate over time instead of always keeping the top results."
      persistent-hint
      @update:model-value="emit('update:randomizeResults', !!$event)"
    ></v-checkbox>

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

    <div class="d-flex flex-wrap align-center ga-2 mt-4">
      <v-btn
        color="primary"
        variant="tonal"
        :loading="loading"
        @click="emit('sync')"
        >Sync Now</v-btn
      >
      <v-btn color="warning" @click="emit('clear')">Clear All Photos</v-btn>
      <v-spacer />
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';

// The topic editor of one Unsplash / Pexels connection. The card owns the
// data: it passes the stored topics in and saves the edited set.
const props = defineProps<{
  title: string;
  hint?: string;
  topics: string[];
  count: number;
  loading: boolean;
  randomizeResults: boolean;
  autoSyncEnabled: boolean;
  autoSyncInterval: number;
  autoSyncOptions: { title: string; value: number }[];
}>();

const emit = defineEmits<{
  save: [topics: string[]];
  sync: [];
  clear: [];
  'update:randomizeResults': [boolean];
  'update:autoSyncEnabled': [boolean];
  'update:autoSyncInterval': [number];
}>();

const topics = ref<string[]>([...props.topics]);
const newTopic = ref('');

// Follow the stored set when the card reloads it (after a save, a clear).
watch(
  () => props.topics,
  (stored) => {
    topics.value = [...stored];
  }
);

const addTopic = () => {
  const t = newTopic.value.trim();
  if (!t) return;
  if (!topics.value.some((x) => x.toLowerCase() === t.toLowerCase())) {
    topics.value.push(t);
  }
  newTopic.value = '';
  emit('save', [...topics.value]);
};

const removeTopic = (t: string) => {
  topics.value = topics.value.filter((x) => x !== t);
  emit('save', [...topics.value]);
};
</script>
