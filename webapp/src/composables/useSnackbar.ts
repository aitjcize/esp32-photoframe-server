import { reactive } from 'vue';

// Module-level singleton so every caller (and the single global <v-snackbar>
// mounted in App.vue) shares one snackbar state. This lets any component —
// including ones extracted out of Settings.vue — surface a message without
// each owning its own snackbar.
const snackbar = reactive({
  show: false,
  message: '',
  color: 'success' as 'success' | 'error',
});

export function useSnackbar() {
  const showMessage = (msg: string, isError = false) => {
    snackbar.message = msg;
    snackbar.color = isError ? 'error' : 'success';
    snackbar.show = true;
  };
  return { snackbar, showMessage };
}
