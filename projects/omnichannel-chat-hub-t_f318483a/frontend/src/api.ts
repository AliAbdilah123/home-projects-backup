import type { AuthUser, LoginRequest, LoginResponse } from './types';

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';
const AUTH_TOKEN_KEY = 'och.auth.token';

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body) {
    headers.set('Content-Type', 'application/json');
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(text || `Request failed with ${response.status}`, response.status);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export const tokenStore = {
  read(): string | null {
    return window.localStorage.getItem(AUTH_TOKEN_KEY);
  },
  write(token: string): void {
    window.localStorage.setItem(AUTH_TOKEN_KEY, token);
  },
  clear(): void {
    window.localStorage.removeItem(AUTH_TOKEN_KEY);
  }
};

export async function login(payload: LoginRequest): Promise<LoginResponse> {
  return request<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(payload)
  });
}

export async function fetchMe(token: string): Promise<AuthUser> {
  return request<AuthUser>('/me', { method: 'GET' }, token);
}

export async function logout(token: string): Promise<void> {
  await request<void>('/auth/logout', { method: 'POST' }, token);
}
