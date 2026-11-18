import { authApi, createApiClient, createSessionStore } from '@fixr/core';
import * as SecureStore from 'expo-secure-store';
import { API_URL } from './config';

export const { useSession, tokenStore } = createSessionStore({
  getItem: (k) => SecureStore.getItem(k),
  setItem: (k, v) => SecureStore.setItem(k, v),
  deleteItem: (k) => void SecureStore.deleteItemAsync(k),
});

export const api = createApiClient({
  baseUrl: API_URL,
  tokens: tokenStore,
  onSessionExpired: () => useSession.getState().signOut(),
});

export const auth = authApi(api);
