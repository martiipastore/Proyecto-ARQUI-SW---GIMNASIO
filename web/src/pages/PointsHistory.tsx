import { DATE_TIME } from '../format';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError } from '../api/client';
import { useSession } from '../auth/session';
import { AnimatedCounter } from '../components/AnimatedCounter';

interface Movement {
  id: string; fecha: string; aplicado_en: string; motivo: string; puntos: number;
  origen: 'ASISTENCIA' | 'INASISTENCIA' | 'PARTNER' | 'REVERSION'; secuencia: number;
  estado: 'CONFIRMADO' | 'REVERTIDO';
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
  return <section className="points-dashboard" aria-label="Saldo y movimientos" aria-busy={busy}>
    <header className="points-heading">
      <div><span>03</span><p>Club de beneficios</p></div>
      <button type="button" disabled={busy} onClick={() => void load()}>{busy ? 'Actualizando…' : 'Actualizar puntos'}</button>
    </header>

    <div className="points-layout">
      <div className="points-balance">
        <span>Saldo disponible</span>
        {pending ? <strong className="points-pending">Pendiente</strong>
          : balance === null ? <div className="points-loader" aria-label="Cargando puntos" />
            : <strong><AnimatedCounter value={balance} /><small>pts</small></strong>}
        <p>Tus puntos permanecen en tu cuenta aunque no tengas una membresía activa.</p>
      </div>

      <div className="points-movements">
        <div className="points-movements-title"><h2>Últimos movimientos</h2>{history && <span>{history.total} en total</span>}</div>
        {error && <div className="dashboard-error" role="alert"><strong>No pudimos cargar tus puntos.</strong><span>{error}</span></div>}
        {pending ? <p className="dashboard-empty">Tu cuenta de puntos todavía no está disponible. Volvé a consultar en unos minutos.</p>
          : history && history.items.length === 0 ? <p className="dashboard-empty">Todavía no tenés movimientos.</p>
            : history ? <ol className="movement-list">
              {history.items.map((movement) => <li key={movement.id}>
                <span className={`movement-value${movement.puntos > 0 ? ' movement-positive' : movement.puntos < 0 ? ' movement-negative' : ''}`}>
                  {movement.puntos > 0 ? '+' : ''}{movement.puntos}
                </span>
                <div>
                  <strong>{movement.motivo}</strong>
                  <span>{new Date(movement.fecha).toLocaleString('es-AR', DATE_TIME)} · Movimiento {movement.secuencia}</span>
                  {movement.origen === 'INASISTENCIA' && movement.puntos === 0 && <small>Sin descuento por saldo cero</small>}
                  {movement.origen === 'REVERSION' && <small>Corrección del administrador</small>}
                  {movement.estado === 'REVERTIDO' && <small>Movimiento revertido</small>}
                </div>
                <span className="movement-origin">{movement.origen.toLocaleLowerCase('es-AR')}</span>
              </li>)}
            </ol> : <div className="dashboard-skeleton" aria-label="Cargando movimientos" />}

        {history && history.total > history.tamano && <div className="points-pagination">
          <button type="button" disabled={busy || page === 1} onClick={() => setPage((current) => current - 1)}>← Anterior</button>
          <span>Página {page} de {Math.max(1, Math.ceil(history.total / history.tamano))}</span>
          <button type="button" disabled={busy || page * history.tamano >= history.total} onClick={() => setPage((current) => current + 1)}>Siguiente →</button>
        </div>}
      </div>
    </div>
  </section>;
}
