<template>
  <v-dialog v-model="dialog" max-width="400" persistent>
    <v-card>
      <v-card-title class="text-h6">{{ title }}</v-card-title>
      <v-card-text>{{ message }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          v-if="!informOnly"
          color="grey-darken-1"
          variant="text"
          @click="cancel"
          >Cancel</v-btn
        >
        <v-btn color="primary" @click="confirm">{{
          informOnly ? 'OK' : 'Confirm'
        }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const dialog = ref(false);
const title = ref('Confirm');
const message = ref('');
// inform(): a notice with one OK button, no choice to make.
const informOnly = ref(false);
let resolvePromise: (value: boolean) => void;

const open = (msg: string, titleText = 'Confirm Action') => {
  message.value = msg;
  title.value = titleText;
  informOnly.value = false;
  dialog.value = true;
  return new Promise<boolean>((resolve) => {
    resolvePromise = resolve;
  });
};

const inform = async (msg: string, titleText = 'Notice') => {
  message.value = msg;
  title.value = titleText;
  informOnly.value = true;
  dialog.value = true;
  await new Promise<boolean>((resolve) => {
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

defineExpose({ open, inform });
</script>
