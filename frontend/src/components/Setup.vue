<template>
  <div
    class="max-w-md mx-auto bg-white rounded-xl p-8 shadow-xl"
  >
    <h2 class="text-3xl font-bold text-gray-800 mb-6 text-center">Welcome!</h2>
    <p class="text-gray-600 mb-8 text-center">
      Create an admin account to get started.
    </p>

    <form @submit.prevent="handleRegister" class="space-y-6">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-2"
          >Username</label
        >
        <input
          v-model="username"
          type="text"
          required
          class="w-full px-4 py-3 bg-white border-2 border-gray-300 rounded-lg text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent transition-all"
          placeholder="admin"
        />
      </div>

      <div>
        <label class="block text-sm font-medium text-gray-700 mb-2"
          >Password</label
        >
        <input
          v-model="password"
          type="password"
          required
          class="w-full px-4 py-3 bg-white border-2 border-gray-300 rounded-lg text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent transition-all"
          placeholder="••••••••"
        />
      </div>

      <div
        v-if="error"
        class="text-red-400 text-sm text-center bg-red-900/20 p-2 rounded"
      >
        {{ error }}
      </div>

      <button
        type="submit"
        :disabled="loading"
        class="w-full py-3 px-4 bg-primary-600 text-white font-semibold rounded-lg hover:bg-primary-700 hover:shadow-lg hover:-translate-y-0.5 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <span v-if="loading">Creating Account...</span>
        <span v-else>Get Started</span>
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import axios from 'axios';
import { useAuthStore } from '../stores/auth';

const authStore = useAuthStore();
const username = ref('');
const password = ref('');
const error = ref('');
const loading = ref(false);

const handleRegister = async () => {
  loading.value = true;
  error.value = '';

  try {
    const res = await axios.post('/api/auth/register', {
      username: username.value,
      password: password.value,
    });

    // Auto login with the returned token
    if (res.data.token) {
      authStore.setToken(res.data.token);
      // Trigger status check to update global state if needed, or just let App.vue handle it
      await authStore.checkStatus();
    }
  } catch (err: any) {
    error.value = err.response?.data?.error || 'Registration failed';
  } finally {
    loading.value = false;
  }
};
</script>
