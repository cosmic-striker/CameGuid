import axios from 'axios';

const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
});

// Request interceptor: attach JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('camguid_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Response interceptor: handle 401 + auto-refresh
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config;
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true;
      const refreshToken = localStorage.getItem('camguid_refresh');
      if (refreshToken) {
        try {
          const { data } = await axios.post('/api/v1/auth/refresh', {
            refresh_token: refreshToken,
          });
          localStorage.setItem('camguid_token', data.access_token);
          localStorage.setItem('camguid_refresh', data.refresh_token);
          original.headers.Authorization = `Bearer ${data.access_token}`;
          return api(original);
        } catch {
          localStorage.removeItem('camguid_token');
          localStorage.removeItem('camguid_refresh');
          window.location.href = '/login';
        }
      } else {
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

export default api;

// --- API Functions ---

export const authAPI = {
  login: (username, password) => api.post('/auth/login', { username, password }),
  refresh: (refreshToken) => api.post('/auth/refresh', { refresh_token: refreshToken }),
  me: () => api.get('/auth/me'),
  register: (data) => api.post('/auth/register', data),
  changePassword: (data) => api.put('/auth/password', data),
};

export const devicesAPI = {
  list: (params) => api.get('/devices', { params }),
  get: (id) => api.get(`/devices/${id}`),
  create: (data) => api.post('/devices', data),
  update: (id, data) => api.put(`/devices/${id}`, data),
  delete: (id) => api.delete(`/devices/${id}`),
  toggle: (id) => api.patch(`/devices/${id}/toggle`),
  generateChannels: (id) => api.post(`/devices/${id}/channels`),
};

export const dashboardAPI = {
  stats: () => api.get('/dashboard/stats'),
};

export const scansAPI = {
  start: (data) => api.post('/scan', data, { baseURL: '/' }),
  list: () => api.get('/scan', { baseURL: '/' }),
  get: (id) => api.get(`/scan/${id}/status`, { baseURL: '/' }),
  cancel: (id) => api.post(`/scan/${id}/cancel`, {}, { baseURL: '/' }),
};

export const streamsAPI = {
  start: (data) => api.post('/start', data, { baseURL: '/stream/api' }),
  stop: (id) => api.delete(`/stop/${id}`, { baseURL: '/stream/api' }),
  list: () => api.get('/sessions', { baseURL: '/stream/api' }),
};
