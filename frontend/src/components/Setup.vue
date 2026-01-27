<template>
  <v-row justify="center">
    <v-col cols="12" sm="8" md="6" lg="4">
      <v-card class="mt-8 elevation-4">
        <v-card-title class="text-center text-h5 font-weight-bold pt-6">
          Welcome!
        </v-card-title>
        <v-card-subtitle class="text-center pb-4">
          Create an admin account to get started
        </v-card-subtitle>

        <v-card-text>
          <v-form @submit.prevent="handleRegister">
            <v-text-field
              v-model="username"
              label="Username"
              placeholder="admin"
              prepend-inner-icon="mdi-account-plus"
              variant="outlined"
              class="mb-2"
              required
            ></v-text-field>

            <v-text-field
              v-model="password"
              label="Password"
              type="password"
              placeholder="••••••••"
              prepend-inner-icon="mdi-lock"
              variant="outlined"
              class="mb-4"
              required
            ></v-text-field>

            <v-alert
              v-if="error"
              type="error"
              variant="tonal"
              class="mb-4"
              density="compact"
            >
              {{ error }}
            </v-alert>

            <v-btn
              type="submit"
              color="primary"
              block
              size="large"
              :loading="loading"
              class="mt-2"
            >
              Get Started
            </v-btn>
          </v-form>
        </v-card-text>
      </v-card>
    </v-col>
  </v-row>
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
