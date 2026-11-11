import { useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { useConfirm } from '../../components/ConfirmDialog';
import { EmptyState, Notice, NoticeState, PageHeading } from '../../components/PageParts';
import { BOOKING_STATES, day, errorText, newIdempotencyKey, Pagina, range, Reserva, time } from './types';

// Mis reservas: futuras e historial, con cancelación hasta 1 hora antes
// (booking.v1, HU-15, HU-16, RN-17).
const PAGE_SIZE = 20;

const STATE_ICON: Record<Reserva['estado'], string> = { CONFIRMADA: '●', ASISTIDA: '✓', AUSENTE: '✕', CANCELADA: '—' };

export function MyBookingsPage() {
  const { authRequest } = useSession();
  const [dialog, confirm] = useConfirm();
  const [state, setState] = useState('');
  const [page, setPage] = useState(1);
  const [data, setData] = useState<Pagina<Reserva> | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [busy, setBusy] = useState('');

  const load = useCallback(async () => {
    const params = new URLSearchParams({ pagina: String(page), tamano: String(PAGE_SIZE) });
    if (state) params.set('estado', state);
    try {
      const res = await authRequest<Pagina<Reserva>>(`/api/v1/bookings/me?${params.toString()}`);
      setData(res.data);
    } catch (error) {
      setNotice({ tone: 'error', text: errorText(error, 'No se pudieron cargar tus reservas.') });
    }
  }, [authRequest, state, page]);

  useEffect(() => {
    void load();
  }, [load]);

  async function cancel(r: Reserva) {
    const ok = await confirm({
      title: 'Cancelar reserva',
      message: `${r.clase.actividad} del ${day(r.clase.inicio)} a las ${time(r.clase.inicio)}. El lugar queda libre para otra persona.`,
      confirmLabel: 'Cancelar reserva',
      danger: true,
    });
    if (ok === null) return;
    setBusy(r.id);
    setNotice(null);
    try {
      await authRequest(`/api/v1/bookings/${r.id}/cancel`, { method: 'POST', headers: { 'Idempotency-Key': newIdempotencyKey() } });
      setNotice({ tone: 'success', text: 'Reserva cancelada; el lugar quedó libre.' });
      await load();
    } catch (error) {
      setNotice({ tone: 'error', text: errorText(error, 'No se pudo cancelar la reserva.') });
    } finally {
      setBusy('');
    }
  }

  const now = Date.now();
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;

  return (
    <main className="page admin-page">
      {dialog}
      <PageHeading eyebrow="Tu agenda" title="Mis reservas" description="Tus próximas clases y el historial de asistencias. Podés cancelar hasta una hora antes del inicio." index="03" />
      <div className="filters admin-toolbar-panel">
        <label>Estado
          <select value={state} onChange={(e) => { setPage(1); setState(e.target.value); }}>
            <option value="">Todas</option>
            {Object.entries(BOOKING_STATES).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <a className="button button-primary" href="#/app/clases">Buscar clases <span aria-hidden="true">→</span></a>
      </div>
      <Notice key={notice?.text} notice={notice} />
      {data === null ? (
        notice?.tone !== 'error' && <div className="dashboard-skeleton" aria-label="Cargando reservas" />
      ) : data.items.length === 0 ? (
        <EmptyState title={state ? 'Nada en ese estado' : 'Tu agenda está libre'}>
          <a href="#/app/clases">Explorar clases →</a>
        </EmptyState>
      ) : (
        <ol className="booking-list">
          {data.items.map((r) => {
            // RN-17: se puede cancelar mientras falte más de 1 hora; el backend decide.
            const cancelable = r.estado === 'CONFIRMADA' && new Date(r.clase.inicio).getTime() - now > 60 * 60 * 1000;
            return (
              <li key={r.id} className={`booking-item booking-${r.estado.toLowerCase()}`}>
                <time dateTime={r.clase.inicio}><strong>{day(r.clase.inicio)}</strong><span>{range(r.clase)}</span></time>
                <div>
                  <strong>{r.clase.actividad}</strong>
                  <span className="booking-state"><i aria-hidden="true">{STATE_ICON[r.estado]}</i> {r.estado === 'CANCELADA' && r.motivo_cancelacion ? r.motivo_cancelacion : BOOKING_STATES[r.estado]}</span>
                  {r.ingreso_en && <small>Ingreso registrado a las {time(r.ingreso_en)}</small>}
                </div>
                {cancelable ? (
                  <button type="button" className="button-secondary" disabled={busy !== ''} onClick={() => void cancel(r)}>{busy === r.id ? 'Cancelando…' : 'Cancelar'}</button>
                ) : <span aria-hidden="true" />}
              </li>
            );
          })}
        </ol>
      )}
      {data && pages > 1 && (
        <nav className="pagination" aria-label="Páginas de reservas">
          <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>Anterior</button>
          <span>Página {page} de {pages}</span>
          <button type="button" disabled={page >= pages} onClick={() => setPage(page + 1)}>Siguiente</button>
        </nav>
      )}
    </main>
  );
}
