<template>
  <v-app>
    <v-app-bar color="primary" density="compact">
      <template #prepend>
        <v-img
          src="/favicon.svg"
          alt="PhotoFrame Server"
          width="32"
          height="32"
          class="ml-2"
        />
      </template>
      <v-app-bar-title class="ml-4">ESP32 PhotoFrame Server</v-app-bar-title>
      <template v-if="authStore.isLoggedIn" v-slot:append>
        <v-btn
          variant="text"
          @click="authStore.logout"
          prepend-icon="mdi-logout"
        >
          Logout
        </v-btn>
      </template>
    </v-app-bar>

    <v-main class="bg-grey-lighten-4">
      <v-container class="py-6" style="max-width: 1200px">
        <div
          v-if="authStore.loading && !authStore.isInitialized"
          class="d-flex justify-center align-center fill-height"
        >
          <v-progress-circular
            indeterminate
            color="primary"
            size="64"
          ></v-progress-circular>
        </div>

        <div v-else>
          <Setup v-if="!authStore.isInitialized" />
          <Login v-else-if="!authStore.isLoggedIn" />
          <Settings v-else />
        </div>
      </v-container>
    </v-main>

    <!-- Global snackbar shared via useSnackbar() -->
    <v-snackbar
      v-model="snackbar.show"
      :color="snackbar.color"
      :timeout="3000"
      location="bottom right"
    >
      {{ snackbar.message }}
      <template v-slot:actions>
        <v-btn variant="text" @click="snackbar.show = false">Close</v-btn>
      </template>
    </v-snackbar>
  </v-app>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import Settings from './components/Settings.vue';
import Login from './components/Login.vue';
import Setup from './components/Setup.vue';
import { useAuthStore } from './stores/auth';
import { useSnackbar } from './composables/useSnackbar';

const authStore = useAuthStore();
const { snackbar } = useSnackbar();

onMounted(async () => {
  await authStore.checkStatus();
});
</script>
