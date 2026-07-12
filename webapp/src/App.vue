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

    <!-- Same style as the device webapp's version footer -->
    <v-footer
      v-if="serverVersion"
      app
      class="text-center d-flex justify-center"
    >
      <span class="text-body-2 text-grey">
        ESP32 PhotoFrame Server {{ serverVersion }}
      </span>
    </v-footer>

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
import { onMounted, ref } from 'vue';
import Settings from './components/Settings.vue';
import Login from './components/Login.vue';
import Setup from './components/Setup.vue';
import { useAuthStore } from './stores/auth';
import { useSnackbar } from './composables/useSnackbar';
import { getStatus } from './api';

const authStore = useAuthStore();
const { snackbar } = useSnackbar();
const serverVersion = ref('');

onMounted(async () => {
  try {
    const status = await getStatus();
    serverVersion.value = status.version || '';
  } catch {
    // Non-fatal: just omit the footer.
  }
  await authStore.checkStatus();
});
</script>
