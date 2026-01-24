<template>
  <div class="min-h-screen p-5">
    <div class="max-w-6xl mx-auto">
      <!-- Header -->
      <header class="flex justify-center items-center text-white mb-8">
        <h1 class="text-4xl font-bold drop-shadow-lg">PhotoFrame Server</h1>
      </header>

      <!-- Main Content -->
      <Settings />

      <!-- Footer -->
      <footer class="text-center text-white mt-8 py-5 text-sm opacity-80">
        <p>ESP32-S3 PhotoFrame Server</p>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import Settings from './components/Settings.vue';
</script>
