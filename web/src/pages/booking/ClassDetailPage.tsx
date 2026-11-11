import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { useConfirm } from '../../components/ConfirmDialog';
import { EmptyState, Notice, NoticeState, PageHeading, SectionHeading } from '../../components/PageParts';
import { BOOKING_STATES, Clase, CLASS_STATES, day, errorText, newIdempotencyKey, occupancy, range, ReservaDeClase, time } from './types';

// Detalle de una clase con sus reservas, asistentes y ausentes (booking.v1,
// HU-08, HU-21, HU-35). El administrador cambia la capacidad, cancela la clase
// o cancela una reserva en nombre del alumno (D-15); el profesor responsable
// solo consulta.
export function ClassDetailPage({ id }: { id: string }) {
  const { authRequest, user } = useSession();
  const [dialog, confirm] = useConfirm();
  const [clase, setClase] = useState<Clase | null>(null);
  const [reservas, setReservas] = useState<ReservaDeClase[]>([]);
  const [capacity, setCapacity] = useState('');
  const [reason, setReason] = useState('');
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [busy, setBusy] = useState(false);
  const admin = user?.role === 'ADMINISTRADOR';

  const load = useCallback(async () => {
    try {
      const res = await authRequest<{ clase: Clase; reservas?: ReservaDeClase[] }>(`/api/v1/classes/${id}`);
      setClase(res.data.clase);
      setReservas(res.data.reservas ?? []);
      setCapacity(String(res.data.clase.capacidad));
    } catch (error) {
      setNotice({ tone: 'error', text: errorText(error, 'No se pudo cargar la clase.') });
    }
  }, [authRequest, id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    setNotice(null);
    try {
      await action();
      setNotice({ tone: 'success', text: done });
      await load();
    } catch (error) {
      setNotice({ tone: 'error', text: errorText(error, 'No se pudo completar la operación.') });
    } finally {
      setBusy(false);
    }
  }

  function changeCapacity(event: FormEvent) {
    event.preventDefault();
    void run(() => authRequest(`/api/v1/classes/${id}/capacity`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ capacidad: Number(capacity) }),
    }), 'Capacidad actualizada.');
  }

  async function cancelClass(event: FormEvent) {
    event.preventDefault();
    const ok = await confirm({ title: 'Cancelar clase', message: 'Se cancelan la clase y todas sus reservas activas. Esta acción no se puede deshacer.', confirmLabel: 'Cancelar clase', danger: true });
    if (ok === null) return;
    void run(() => authRequest(`/api/v1/classes/${id}/cancel`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ motivo: reason }),
    }), 'Clase cancelada.');
  }

  async function cancelBooking(r: ReservaDeClase) {
    const ok = await confirm({ title: 'Cancelar reserva', message: `Se cancela la reserva de ${r.alumno_nombre ?? r.alumno_id} y se libera su lugar.`, confirmLabel: 'Cancelar reserva', danger: true });
    if (ok === null) return;
    void run(() => authRequest(`/api/v1/bookings/${r.id}/cancel`, { method: 'POST', headers: { 'Idempotency-Key': newIdempotencyKey() } }), 'Reserva cancelada.');
  }

  const back = admin ? '#/admin/clases' : '#/prof/clases';
  if (!clase) {
    return (
      <main className="page admin-page">
        <a className="admin-back-link" href={back}>← Volver a clases</a>
        {notice ? <Notice notice={notice} /> : <div className="dashboard-skeleton" aria-label="Cargando clase" />}
      </main>
    );
  }

  const editable = admin && clase.estado === 'PROGRAMADA';
  return (
    <main className="page admin-page">
      {dialog}
      <a className="admin-back-link" href={back}>← Volver a clases</a>
      <PageHeading
        eyebrow={`${CLASS_STATES[clase.estado]} · ${day(clase.inicio)} ${range(clase)}`}
        title={clase.actividad.nombre}
        description={`Profesor: ${clase.profesor.nombre} · ${occupancy(clase)}`}
        index={String(clase.ocupacion).padStart(2, '0')}
      />
      <Notice key={notice?.text} notice={notice} />

      {editable && (
        <section className="admin-section">
          <SectionHeading index="01" kicker="Operación" title="Capacidad y cancelación" />
          <form className="inline-form" onSubmit={changeCapacity}>
            <label>Capacidad <input type="number" min={Math.max(1, clase.ocupacion)} max={50} value={capacity} onChange={(e) => setCapacity(e.target.value)} required /></label>
            <button type="submit" disabled={busy}>Guardar capacidad</button>
          </form>
          <form className="inline-form" onSubmit={(e) => void cancelClass(e)}>
            <label>Motivo de cancelación <input value={reason} onChange={(e) => setReason(e.target.value)} minLength={3} maxLength={200} required /></label>
            <button type="submit" className="button-danger" disabled={busy}>Cancelar clase</button>
          </form>
        </section>
      )}

      <section className="admin-section">
        <SectionHeading index={editable ? '02' : '01'} kicker={`${reservas.length} en total`} title="Reservas y asistencia" />
        {reservas.length === 0 ? (
          <EmptyState title="Sin reservas" />
        ) : (
          <table className="data-table responsive-table">
            <thead>
              <tr><th>Alumno</th><th>Estado</th><th>Reservó</th><th>Ingreso</th>{editable && <th><span className="sr-only">Acciones</span></th>}</tr>
            </thead>
            <tbody>
              {reservas.map((r) => (
                <tr key={r.id}>
                  <td data-label="Alumno">{r.alumno_nombre ?? <span className="muted">{r.alumno_id}</span>}</td>
                  <td data-label="Estado">{BOOKING_STATES[r.estado]}</td>
                  <td data-label="Reservó">{day(r.creada_en)} {time(r.creada_en)}</td>
                  <td data-label="Ingreso">{r.ingreso_en ? time(r.ingreso_en) : '—'}</td>
                  {editable && (
                    <td>{r.estado === 'CONFIRMADA' && <button type="button" disabled={busy} onClick={() => void cancelBooking(r)}>Cancelar</button>}</td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
