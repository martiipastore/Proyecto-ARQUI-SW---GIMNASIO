// Ruteo mínimo por hash (#/app, #/admin…), sin dependencias: funciona igual en
// Nginx, en el servidor de Vite y en Vercel sin configurar respaldos de rutas
// (etapa 2 de la entrega final). El enlace de activación existente usa
// "#activate=<token>" (sin barra) y se distingue de las rutas.
import { useEffect, useState } from 'react';

/** Ruta actual: "#/app/clases" → "/app/clases"; sin hash, "/". */
export function currentPath(): string {
  const hash = window.location.hash;
  return hash.startsWith('#/') ? hash.slice(1) : '/';
}

export function navigate(path: string): void {
  if (currentPath() !== path) {
    window.location.hash = path;
  }
}

/** Devuelve la ruta actual y se actualiza cuando cambia el hash. */
export function useRoute(): string {
  const [path, setPath] = useState(currentPath);
  useEffect(() => {
    const onChange = () => setPath(currentPath());
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return path;
}

/**
 * Lee el token de "#activate=<token>" y lo saca de la barra de direcciones para
 * que no quede en el historial. Se llama una sola vez al iniciar la aplicación.
 */
export function takeActivationToken(): string {
  const hash = window.location.hash;
  if (!hash.startsWith('#activate=')) {
    return '';
  }
  const token = new URLSearchParams(hash.slice(1)).get('activate') ?? '';
  window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}#/activar`);
  return token;
}
