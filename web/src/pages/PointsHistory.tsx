import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError } from '../api/client';
import { useSession } from '../auth/session';

interface Movement {
  id: string; fecha: string; aplicado_en: string; motivo: string; puntos: number;
  origen: 'ASISTENCIA' | 'INASISTENCIA' | 'PARTNER'; secuencia: number;
}
interface History { items: Movement[]; total: number; pagina: number; tamano: number }

// HU-19, RN-22/38/42. La secuencia refleja el orden de aplicación; una ausencia
// de cero puntos sigue visible y no genera una deuda para créditos futuros.
export function PointsHistory() {
  const { authRequest } = useSession();
  const [history, setHistory] = useState<History | null>(null);
  const [balance, setBalance] = useState<number | null>(null);
  const [page, setPage] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  const generation = useRef(0);
  const load = useCallback(async () => {
    const current = ++generation.current;
    setBusy(true); setError('');
    try {
      const [account, movements] = await Promise.all([
        authRequest<{ balance: number }>('/api/v1/benefits/me'),
        authRequest<History>(`/api/v1/benefits/me/movements?pagina=${page}&tamano=20`),
      ]);
      if (current !== generation.current) return;
      setBalance(account.data.balance); setHistory(movements.data); setPending(false);
    } catch (cause) {
      if (current !== generation.current) return;
      setHistory(null); setBalance(null);
      if (cause instanceof ApiError && cause.status === 404) { setPending(true); }
      else { setPending(false); setError(cause instanceof Error ? cause.message : 'No se pudieron cargar tus movimientos.'); }
    } finally { if (current === generation.current) setBusy(false); }
  }, [authRequest, page]);
  useEffect(() => {
    void load();
    return () => { generation.current++; };
  }, [load]);
  return <section className="club-card" aria-label="Saldo y movimientos" aria-busy={busy}>
    <h2>Tus puntos</h2>
    {error && <p role="alert">{error}</p>}
    {pending ? <p>Tu cuenta de puntos todavía no está disponible. Volvé a consultar en unos minutos.</p>
      : <p>Puntos disponibles: <strong>{balance ?? '…'}</strong></p>}
    <p className="muted">Los puntos por asistencia y ausencia pueden demorar unos instantes. Actualizá para consultar los últimos movimientos.</p>
    <button type="button" disabled={busy} onClick={() => void load()}>{busy ? 'Actualizando…' : 'Actualizar puntos'}</button>
    {history && <>
      <h3>Movimientos</h3>
      {history.items.length === 0 ? <p>Todavía no tenés movimientos.</p> : <ol>
        {history.items.map(m => <li key={m.id}>
          <strong>{m.puntos > 0 ? '+' : ''}{m.puntos} puntos</strong> · {m.motivo}
          {m.origen === 'INASISTENCIA' && m.puntos === 0 && ' (sin descuento por saldo cero)'}
          <br /><span className="muted">{new Date(m.fecha).toLocaleString('es-AR')} · Movimiento {m.secuencia}</span>
        </li>)}
      </ol>}
      <p>Página {page} de {Math.max(1, Math.ceil(history.total / history.tamano))} · {history.total} movimientos</p>
      <button type="button" disabled={busy || page === 1} onClick={() => setPage(p => p - 1)}>Anterior</button>{' '}
      <button type="button" disabled={busy || page * history.tamano >= history.total} onClick={() => setPage(p => p + 1)}>Siguiente</button>
    </>}
  </section>;
}
