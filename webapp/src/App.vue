<template>
  <v-app>
    <v-app-bar color="primary" density="compact">
      <v-app-bar-title>ESP32 PhotoFrame Server</v-app-bar-title>
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

    <v-footer
      app
      class="text-center d-flex justify-center text-caption text-grey"
    >
      ESP32-S3 PhotoFrame Server
    </v-footer>
  </v-app>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import Settings from './components/Settings.vue';
import Login from './components/Login.vue';
import Setup from './components/Setup.vue';
import { useAuthStore } from './stores/auth';

const authStore = useAuthStore();

onMounted(async () => {
  await authStore.checkStatus();
});
</script>
