import { useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { BOOKING_STATES, day, errorText, newIdempotencyKey, Pagina, range, Reserva, time } from './types';

// Mis reservas: futuras e historial, con cancelación hasta 1 hora antes
// (booking.v1, HU-15, HU-16, RN-17).
const PAGE_SIZE = 20;

export function MyBookingsPage() {
  const { authRequest } = useSession();
  const [state, setState] = useState('');
  const [page, setPage] = useState(1);
  const [data, setData] = useState<Pagina<Reserva> | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState('');

  const load = useCallback(async () => {
    const params = new URLSearchParams({ pagina: String(page), tamano: String(PAGE_SIZE) });
    if (state) params.set('estado', state);
    try {
      const res = await authRequest<Pagina<Reserva>>(`/api/v1/bookings/me?${params.toString()}`);
      setData(res.data);
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron cargar tus reservas.'));
    }
  }, [authRequest, state, page]);

  useEffect(() => {
    void load();
  }, [load]);

  async function cancel(r: Reserva) {
    if (!window.confirm(`¿Cancelar ${r.clase.actividad} del ${day(r.clase.inicio)} a las ${time(r.clase.inicio)}?`)) return;
    setBusy(r.id);
    setMessage('');
    try {
      await authRequest(`/api/v1/bookings/${r.id}/cancel`, { method: 'POST', headers: { 'Idempotency-Key': newIdempotencyKey() } });
      setMessage('Reserva cancelada; el lugar quedó libre.');
      await load();
    } catch (error) {
      setMessage(errorText(error, 'No se pudo cancelar la reserva.'));
    } finally {
      setBusy('');
    }
  }

  const now = Date.now();
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;

  return (
    <main className="page">
      <h1>Mis reservas</h1>
      <div className="filters">
        <label>Estado
          <select value={state} onChange={(e) => { setPage(1); setState(e.target.value); }}>
            <option value="">Todas</option>
            {Object.entries(BOOKING_STATES).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <a href="#/app/clases">Buscar clases</a>
      </div>
      {message && <p role="status">{message}</p>}
      {data === null ? (
        !message && <p className="muted">Cargando…</p>
      ) : data.items.length === 0 ? (
        <p>No tenés reservas{state ? ' en ese estado' : ''}.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Día</th><th>Horario</th><th>Actividad</th><th>Estado</th><th>Ingreso</th><th /></tr>
          </thead>
          <tbody>
            {data.items.map((r) => {
              // RN-17: se puede cancelar mientras falte más de 1 hora; el backend decide.
              const cancelable = r.estado === 'CONFIRMADA' && new Date(r.clase.inicio).getTime() - now > 60 * 60 * 1000;
              return (
                <tr key={r.id}>
                  <td>{day(r.clase.inicio)}</td>
                  <td>{range(r.clase)}</td>
                  <td>{r.clase.actividad}</td>
                  <td>{BOOKING_STATES[r.estado]}{r.motivo_cancelacion ? ` (${r.motivo_cancelacion})` : ''}</td>
                  <td>{r.ingreso_en ? time(r.ingreso_en) : '—'}</td>
                  <td>
                    {cancelable && (
                      <button type="button" disabled={busy !== ''} onClick={() => void cancel(r)}>{busy === r.id ? 'Cancelando…' : 'Cancelar'}</button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {data && pages > 1 && (
        <div className="pagination">
          <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>Anterior</button>
          <span>Página {page} de {pages}</span>
          <button type="button" disabled={page >= pages} onClick={() => setPage(page + 1)}>Siguiente</button>
        </div>
      )}
    </main>
  );
}
