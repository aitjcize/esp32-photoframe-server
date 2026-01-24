import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export const getSettings = async () => {
  const response = await api.get('/settings');
  return response.data;
};

export const updateSettings = async (settings: Record<string, string>) => {
  const response = await api.post('/settings', { settings });
  return response.data;
};

export const getStatus = async () => {
  const response = await api.get('/status');
  return response.data;
};

export const getGoogleAlbums = async () => {
  const response = await api.get('/google/albums');
  return response.data;
};
