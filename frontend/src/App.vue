<template>
  <div class="min-h-screen p-5">
    <div class="max-w-6xl mx-auto">
      <!-- Header -->
      <header class="flex justify-between items-center text-white mb-8">
        <h1 class="text-4xl font-bold drop-shadow-lg">PhotoFrame Server</h1>
        <button
          v-if="authStore.isLoggedIn"
          @click="authStore.logout"
          class="px-4 py-2 bg-white/10 hover:bg-white/20 rounded-lg text-sm backdrop-blur-sm transition-colors"
        >
          Logout
        </button>
      </header>

      <!-- Main Content -->
      <div v-if="authStore.loading && !authStore.isInitialized">
        <div class="text-center text-white p-10">Loading...</div>
      </div>

      <div v-else>
        <Setup v-if="!authStore.isInitialized" />
        <Login v-else-if="!authStore.isLoggedIn" />
        <Settings v-else />
      </div>

      <!-- Footer -->
      <footer class="text-center text-white mt-8 py-5 text-sm opacity-80">
        <p>ESP32-S3 PhotoFrame Server</p>
      </footer>
    </div>
  </div>
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
