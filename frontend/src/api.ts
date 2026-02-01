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
  width: number;
  height: number;
  orientation: string;
  use_device_parameter: boolean;
  enable_collage: boolean;
  show_date?: boolean;
  show_weather?: boolean;
  weather_lat?: number;
  weather_lon?: number;
  created_at: string;
  model?: any;
}

export const listDevices = async () => {
  const response = await api.get('/devices');
  return response.data;
};

export const addDevice = async (
  host: string,
  useDeviceParameter: boolean,
  enableCollage: boolean,
  showDate: boolean,
  showWeather: boolean,
  weatherLat: number,
  weatherLon: number
) => {
  const response = await api.post('/devices', {
    host,
    use_device_parameter: useDeviceParameter,
    enable_collage: enableCollage,
    show_date: showDate,
    show_weather: showWeather,
    weather_lat: weatherLat,
    weather_lon: weatherLon,
  });
  return response.data;
};

export const updateDevice = async (
  id: number,
  name: string,
  host: string,
  width: number,
  height: number,
  orientation: string,
  useDeviceParameter: boolean,
  enableCollage: boolean,
  showDate: boolean,
  showWeather: boolean,
  weatherLat: number,
  weatherLon: number
) => {
  const response = await api.put(`/devices/${id}`, {
    name,
    host,
    width,
    height,
    orientation,
    use_device_parameter: useDeviceParameter,
    enable_collage: enableCollage,
    show_date: showDate,
    show_weather: showWeather,
    weather_lat: weatherLat,
    weather_lon: weatherLon,
  });
  return response.data;
};

export const deleteDevice = async (id: number) => {
  const response = await api.delete(`/devices/${id}`);
  return response.data;
};

export const pushToDevice = async (deviceID: number, imageID: number) => {
  const response = await api.post(`/devices/${deviceID}/push`, {
    image_id: imageID,
  });
  return response.data;
};

export const configureDeviceSource = async (id: number, source: string) => {
  const response = await api.post(`/devices/${id}/configure-source`, {
    source,
  });
  return response.data;
};

export const createURLSource = async (url: string, deviceIDs: number[]) => {
  const response = await api.post('/gallery/urls', {
    url,
    device_ids: deviceIDs,
  });
  return response.data;
};

export const updateURLSource = async (
  id: number,
  url: string,
  deviceIDs: number[]
) => {
  const response = await api.put(`/gallery/urls/${id}`, {
    url,
    device_ids: deviceIDs,
  });
  return response.data;
};

export const listURLSources = async () => {
  const response = await api.get('/gallery/urls');
  return response.data;
};

export const deleteURLSource = async (id: number) => {
  const response = await api.delete(`/gallery/urls/${id}`);
  return response.data;
};

export const listPhotos = async (
  source?: string,
  limit?: number,
  offset?: number
) => {
  const params: any = {};
  if (source) params.source = source;
  if (limit) params.limit = limit;
  if (offset) params.offset = offset;
  const response = await api.get('/gallery/photos', { params });
  return response.data;
};

export const deletePhoto = async (id: number) => {
  const response = await api.delete(`/gallery/photos/${id}`);
  return response.data;
};
