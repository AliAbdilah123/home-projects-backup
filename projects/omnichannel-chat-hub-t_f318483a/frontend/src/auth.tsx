import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { ApiError, fetchMe, login as loginRequest, logout as logoutRequest, tokenStore } from './api';
import type { AuthUser, LoginRequest } from './types';

type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated';

type AuthContextValue = {
  status: AuthStatus;
  user: AuthUser | null;
  error: string | null;
  login: (payload: LoginRequest) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const existingToken = tokenStore.read();
    if (!existingToken) {
      setStatus('unauthenticated');
      return;
    }

    setToken(existingToken);
    fetchMe(existingToken)
      .then((me) => {
        setUser(me);
        setStatus('authenticated');
      })
      .catch(() => {
        tokenStore.clear();
        setToken(null);
        setStatus('unauthenticated');
      });
  }, []);

  const login = useCallback(async (payload: LoginRequest) => {
    setError(null);
    const session = await loginRequest(payload);
    tokenStore.write(session.token);
    setToken(session.token);
    setUser(session.user);
    setStatus('authenticated');
  }, []);

  const logout = useCallback(async () => {
    if (token) {
      try {
        await logoutRequest(token);
      } catch {
        // allow local cleanup even if network logout fails
      }
    }
    tokenStore.clear();
    setToken(null);
    setUser(null);
    setStatus('unauthenticated');
  }, [token]);

  const value = useMemo<AuthContextValue>(() => ({
    status,
    user,
    error,
    login: async (payload: LoginRequest) => {
      try {
        await login(payload);
      } catch (err: unknown) {
        if (err instanceof ApiError && err.status === 401) {
          setError('Invalid email or password.');
          throw err;
        }
        setError(err instanceof Error ? err.message : 'Unexpected authentication error.');
        throw err;
      }
    },
    logout
  }), [error, login, logout, status, user]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
}
