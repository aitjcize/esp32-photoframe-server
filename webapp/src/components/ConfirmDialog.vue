<template>
  <v-dialog v-model="dialog" max-width="400" persistent>
    <v-card>
      <v-card-title class="text-h6">{{ title }}</v-card-title>
      <v-card-text>{{ message }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="grey-darken-1" variant="text" @click="cancel"
          >Cancel</v-btn
        >
        <v-btn color="primary" @click="confirm">Confirm</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const dialog = ref(false);
const title = ref('Confirm');
const message = ref('');
let resolvePromise: (value: boolean) => void;

const open = (msg: string, titleText = 'Confirm Action') => {
  message.value = msg;
  title.value = titleText;
  dialog.value = true;
  return new Promise<boolean>((resolve) => {
    resolvePromise = resolve;
  });
};

const confirm = () => {
  dialog.value = false;
  resolvePromise(true);
};

const cancel = () => {
  dialog.value = false;
  resolvePromise(false);
};

defineExpose({ open });
</script>
