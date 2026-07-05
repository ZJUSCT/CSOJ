import axios from 'axios';

const api = axios.create({
  baseURL: '/api/v1',
});

api.interceptors.request.use(
  (config) => {
    if (typeof window !== 'undefined') {
      const token = localStorage.getItem('csoj_jwt');
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response) {
      const isBannedResponse =
        error.response.status === 403 &&
        error.response.data?.data?.ban_reason &&
        error.response.data?.data?.banned_until;

      if (isBannedResponse) {
        const banDetails = {
          reason: error.response.data.data.ban_reason,
          until: error.response.data.data.banned_until,
        };
        window.dispatchEvent(new CustomEvent('userBanned', { detail: banDetails }));
      }
    }
    return Promise.reject(error);
  }
);


export default api;