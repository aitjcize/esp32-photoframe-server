<template>
  <v-text-field
    v-model="search"
    placeholder="Search albums…"
    prepend-inner-icon="mdi-magnify"
    variant="outlined"
    density="compact"
    hide-details
    clearable
    class="mb-2"
  ></v-text-field>

  <v-card variant="outlined" class="mb-3 album-list-card">
    <v-list
      density="compact"
      class="py-0 overflow-y-auto"
      :style="{ maxHeight: maxHeight + 'px' }"
    >
      <slot name="prepend"></slot>
      <v-divider v-if="hasPrepend && albums.length"></v-divider>
      <v-list-item v-for="album in filtered" :key="album.id">
        <v-checkbox
          :model-value="modelValue.includes(valueOf(album))"
          :label="album[labelField]"
          :disabled="disabled"
          color="primary"
          density="compact"
          hide-details
          @update:model-value="toggle(album, $event)"
        ></v-checkbox>
      </v-list-item>
      <v-list-item v-if="!albums.length" class="text-caption text-grey">
        {{ emptyText }}
      </v-list-item>
    </v-list>
  </v-card>
</template>

<script setup lang="ts">
import { computed, ref, useSlots } from 'vue';
import { matchesAlbum, sortCheckedFirst } from '../utils/albums';

const props = withDefaults(
  defineProps<{
    albums: any[];
    // Source albums use string ids (Immich UUID / Synology String(id)); per-device
    // pickers use numeric album ids — so accept both.
    modelValue: (string | number)[];
    labelField: string;
    // Synology source albums key on a numeric id; selections are stored as
    // strings, so stringify the value to match.
    stringify?: boolean;
    disabled?: boolean;
    emptyText?: string;
    maxHeight?: number;
  }>(),
  {
    stringify: false,
    disabled: false,
    emptyText: 'No albums found.',
    maxHeight: 320,
  }
);
const emit = defineEmits<{ 'update:modelValue': [(string | number)[]] }>();

const slots = useSlots();
const hasPrepend = computed(() => !!slots.prepend);

const search = ref('');
const valueOf = (a: any) => (props.stringify ? String(a.id) : a.id);

const filtered = computed(() =>
  (props.albums || [])
    .filter((a: any) => matchesAlbum(a[props.labelField], search.value))
    .sort(
      sortCheckedFirst(props.labelField, (a: any) =>
        props.modelValue.includes(valueOf(a))
      )
    )
);

function toggle(album: any, checked: boolean | null) {
  const v = valueOf(album);
  const next = props.modelValue.filter((x) => x !== v);
  if (checked) next.push(v);
  emit('update:modelValue', next);
}
</script>

<style scoped>
/* Vuetify's .v-card--variant-outlined sets `border: thin solid` with no color,
   so the border falls back to currentColor (near-black) here. Match the
   outlined search field's border instead: the field draws currentColor at
   --v-field-border-opacity (0.38), so use the theme border color at 0.38
   (dark-mode aware via --v-border-color) rather than the card's lighter 0.12
   divider opacity. */
.album-list-card {
  border-color: rgba(var(--v-border-color), 0.38) !important;
}
</style>
