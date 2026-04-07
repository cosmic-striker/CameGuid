import { create } from 'zustand';

export const useAuthStore = create((set) => ({
  user: null,
  token: localStorage.getItem('camguid_token'),
  isAuthenticated: !!localStorage.getItem('camguid_token'),

  login: (user, tokens) => {
    localStorage.setItem('camguid_token', tokens.access_token);
    localStorage.setItem('camguid_refresh', tokens.refresh_token);
    set({ user, token: tokens.access_token, isAuthenticated: true });
  },

  logout: () => {
    localStorage.removeItem('camguid_token');
    localStorage.removeItem('camguid_refresh');
    set({ user: null, token: null, isAuthenticated: false });
  },

  setUser: (user) => set({ user }),
}));

export const useToastStore = create((set) => ({
  toasts: [],

  addToast: (message, type = 'info') => {
    const id = Date.now();
    set((state) => ({
      toasts: [...state.toasts, { id, message, type }],
    }));
    setTimeout(() => {
      set((state) => ({
        toasts: state.toasts.filter((t) => t.id !== id),
      }));
    }, 4000);
  },

  removeToast: (id) => {
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    }));
  },
}));
