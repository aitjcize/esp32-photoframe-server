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

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // Ignore Synology endpoints as they use 401 for 2FA challenges
      if (error.config.url && error.config.url.includes('/synology/')) {
        return Promise.reject(error);
      }

      // Clear token and redirect to login if 401 received
      // Avoid redirect loop if already on login page
      if (!window.location.pathname.startsWith('/login')) {
        localStorage.removeItem('token');
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

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
// Devices
export interface Device {
  id: number;
  name: string;
  host: string;
  last_seen: string;
}

export const listDevices = async () => {
  const response = await api.get('/devices');
  return response.data as Device[];
};

export const addDevice = async (name: string, host: string) => {
  const response = await api.post('/devices', { name, host });
  return response.data as Device;
};

export const deleteDevice = async (id: number) => {
  const response = await api.delete(`/devices/${id}`);
  return response.data;
};

export const pushToDevice = async (deviceID: number, imageID: number) => {
  const response = await api.post(`/devices/${deviceID}/push`, { image_id: imageID });
  return response.data;
};
