import { create } from 'zustand';
import type { Tokens, TokenStore } from '../api/client';
import type { User } from '../api/types';

export interface SecureKV {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  deleteItem(key: string): void;
}

interface SessionState {
  user: User | null;
  tokens: Tokens | null;
  hydrated: boolean;
  signIn(tokens: Tokens, user: User): void;
  setUser(user: User): void;
  signOut(): void;
}

const KEY = 'fixr.session.v1';

export function createSessionStore(secure: SecureKV) {
  const store = create<SessionState>((set) => ({
    user: null,
    tokens: null,
    hydrated: false,
    signIn(tokens, user) {
      secure.setItem(KEY, JSON.stringify({ tokens, user }));
      set({ tokens, user });
    },
    setUser(user) {
      set((s) => {
        if (s.tokens) secure.setItem(KEY, JSON.stringify({ tokens: s.tokens, user }));
        return { user };
      });
    },
    signOut() {
      secure.deleteItem(KEY);
      set({ tokens: null, user: null });
    },
  }));

  try {
    const saved = secure.getItem(KEY);
    if (saved) {
      const { tokens, user } = JSON.parse(saved) as { tokens: Tokens; user: User };
      store.setState({ tokens, user });
    }
  } catch {
    secure.deleteItem(KEY);
  }
  store.setState({ hydrated: true });

  const tokenStore: TokenStore = {
    get: () => store.getState().tokens,
    set: (tokens) => {
      const { user } = store.getState();
      if (!tokens || !user) {
        store.getState().signOut();
        return;
      }
      store.getState().signIn(tokens, user);
    },
  };

  return { useSession: store, tokenStore };
}
