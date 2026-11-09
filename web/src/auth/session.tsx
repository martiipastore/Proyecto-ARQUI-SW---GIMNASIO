// Sesión del usuario (ADR-006, auth.v1): el JWT vive solo en memoria, vence a
// los 15 minutos y cualquier 401 cierra la sesión local. Sin refresh tokens: al
// vencer, la persona vuelve a ingresar. El rol que ve el frontend solo decide la
// navegación; el gateway y cada servicio vuelven a autorizar (RN-30).
import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { ApiError, ApiResponse, request } from '../api/client';

export type Role = 'ADMINISTRADOR' | 'PROFESOR' | 'NUTRICIONISTA' | 'ALUMNO';

const ROLES: readonly Role[] = ['ADMINISTRADOR', 'PROFESOR', 'NUTRICIONISTA', 'ALUMNO'];

export interface SessionUser {
  id: string;
  name: string;
  email: string;
  role: Role;
}

interface Session {
  token: string;
  expiresAt: number;
  user: SessionUser;
}

export interface AuthInit extends Omit<RequestInit, 'headers'> {
  headers?: Record<string, string>;
}

interface SessionApi {
  user: SessionUser | null;
  /** Aviso para la próxima pantalla, por ejemplo "Tu sesión venció". */
  notice: string;
  clearNotice(): void;
  login(email: string, password: string): Promise<SessionUser>;
  logout(): Promise<void>;
  /** Solicitud autenticada; un 401 cierra la sesión y vuelve a lanzar el error. */
  authRequest<T>(path: string, init?: AuthInit): Promise<ApiResponse<T>>;
}

const SessionContext = createContext<SessionApi | null>(null);

interface LoginResponse {
  access_token: string;
  expires_in: number;
}

interface MeResponse {
  id: string;
  name: string;
  email: string;
  role: string;
}

function isRole(value: string): value is Role {
  return (ROLES as readonly string[]).includes(value);
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [notice, setNotice] = useState('');

  const expire = useCallback((message: string) => {
    setSession(null);
    setNotice(message);
  }, []);

  useEffect(() => {
    if (!session) return;
    const timer = window.setTimeout(() => expire('Tu sesión venció. Ingresá nuevamente.'), Math.max(0, session.expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [session, expire]);

  const login = useCallback(async (email: string, password: string) => {
    const tokens = await request<LoginResponse>('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    const token = tokens.data.access_token;
    const me = await request<MeResponse>('/api/v1/users/me', { headers: { Authorization: `Bearer ${token}` } });
    if (!isRole(me.data.role)) {
      throw new ApiError('Tu cuenta no tiene un rol habilitado para esta aplicación.', 403, me.correlationId);
    }
    const user: SessionUser = { id: me.data.id, name: me.data.name, email: me.data.email, role: me.data.role };
    setSession({ token, expiresAt: Date.now() + tokens.data.expires_in * 1000, user });
    setNotice('');
    return user;
  }, []);

  const logout = useCallback(async () => {
    const token = session?.token;
    setSession(null);
    if (!token) return;
    try {
      // Logout v1 descarta el token en el cliente; el servidor no lo revoca (auth.v1).
      await request('/api/v1/auth/logout', { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
    } catch {
      // La sesión local ya se cerró; no hay nada más que hacer.
    }
  }, [session]);

  const authRequest = useCallback(async <T,>(path: string, init: AuthInit = {}) => {
    if (!session) {
      throw new ApiError('Ingresá para continuar.', 401, null);
    }
    try {
      return await request<T>(path, { ...init, headers: { ...init.headers, Authorization: `Bearer ${session.token}` } });
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        expire('Tu sesión ya no es válida. Ingresá nuevamente.');
      }
      throw error;
    }
  }, [session, expire]);

  const value = useMemo<SessionApi>(() => ({
    user: session?.user ?? null,
    notice,
    clearNotice: () => setNotice(''),
    login,
    logout,
    authRequest,
  }), [session, notice, login, logout, authRequest]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionApi {
  const value = useContext(SessionContext);
  if (!value) {
    throw new Error('useSession debe usarse dentro de SessionProvider');
  }
  return value;
}

/** Pantalla inicial de cada rol (etapa 1, §2). */
export function homeFor(role: Role): string {
  switch (role) {
    case 'ADMINISTRADOR':
      return '/admin';
    case 'PROFESOR':
      return '/prof';
    case 'NUTRICIONISTA':
      return '/nutri';
    default:
      return '/app';
  }
}
