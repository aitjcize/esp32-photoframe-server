<template>
  <div class="d-flex align-center ga-2">
    <v-select
      :model-value="modelValue"
      :items="items"
      item-title="title"
      item-value="value"
      variant="outlined"
      density="comfortable"
      hide-details
      class="source-picker flex-grow-1"
      :menu-props="{ maxHeight: 480 }"
      @update:model-value="emit('update:modelValue', $event)"
    >
      <template #selection="{ item }">
        <v-icon :icon="item.raw.icon" size="small" class="mr-2"></v-icon>
        <span class="text-truncate">{{ item.raw.title }}</span>
        <span
          v-if="item.raw.group && item.raw.group !== item.raw.title"
          class="text-caption text-grey ml-2 d-none d-sm-inline"
          >{{ item.raw.group }}</span
        >
      </template>
      <template #item="{ props: itemProps, item }">
        <v-list-item
          v-bind="itemProps"
          :title="item.raw.title"
          :subtitle="item.raw.subtitle"
          :prepend-icon="item.raw.icon"
          :disabled="item.raw.disabled"
        >
          <template v-if="item.raw.notConnected" #append>
            <v-chip size="x-small" variant="tonal">Not connected</v-chip>
          </template>
        </v-list-item>
      </template>
    </v-select>
    <v-btn
      color="primary"
      variant="tonal"
      icon="mdi-plus"
      title="Add a photo source"
      aria-label="Add a photo source"
      @click="emit('add')"
    ></v-btn>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  CONNECTION_SOURCES,
  SOURCE_ICONS,
  SOURCE_LABELS,
  connectionSubtitle,
  useConnectionsStore,
} from '../stores/connections';

// Which photo source the top card shows: a built-in one, or one connection.
// Values: 'gallery' | 'url' | 'ai_generation' | 'conn:<id>'.
defineProps<{ modelValue: string }>();
const emit = defineEmits<{
  'update:modelValue': [value: string];
  add: [];
}>();

type SourceItem = {
  value: string;
  title: string;
  subtitle?: string;
  icon?: string;
  group?: string;
  notConnected?: boolean;
  disabled?: boolean;
  type?: 'subheader' | 'divider';
};

const store = useConnectionsStore();

const BUILT_IN: SourceItem[] = [
  {
    value: 'gallery',
    title: 'Gallery',
    subtitle: 'Uploads and Telegram',
    icon: 'mdi-image-multiple',
  },
  {
    value: 'url',
    title: 'URL Proxy',
    subtitle: 'External image URLs',
    icon: 'mdi-link-variant',
  },
  {
    value: 'ai_generation',
    title: 'AI Generation',
    subtitle: 'OpenAI and Gemini images',
    icon: 'mdi-creation',
  },
];

// Built-ins first, then one group per provider that has connections.
const items = computed<SourceItem[]>(() => {
  const out: SourceItem[] = [
    { type: 'subheader', title: 'Built-in', value: '' },
    ...BUILT_IN,
  ];
  let any = false;
  for (const source of CONNECTION_SOURCES) {
    const conns = store.bySource(source);
    if (conns.length === 0) continue;
    any = true;
    out.push({ type: 'divider', title: '', value: '' });
    out.push({ type: 'subheader', title: SOURCE_LABELS[source], value: '' });
    for (const c of conns) {
      out.push({
        value: `conn:${c.id}`,
        title: c.name,
        subtitle: connectionSubtitle(c),
        icon: SOURCE_ICONS[source],
        group: SOURCE_LABELS[source],
        notConnected: !c.connected,
      });
    }
  }
  if (!any && store.loaded) {
    out.push({ type: 'divider', title: '', value: '' });
    out.push({ type: 'subheader', title: 'Photo sources', value: '' });
    out.push({
      value: 'none',
      title: 'No connections yet',
      subtitle:
        'Use + to add Immich, Nextcloud, Synology, Google, Unsplash or Pexels',
      icon: 'mdi-plus-circle-outline',
      disabled: true,
    });
  }
  return out;
});
</script>

<style scoped>
.source-picker {
  min-width: 0;
}
</style>
