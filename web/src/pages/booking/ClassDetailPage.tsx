import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { BOOKING_STATES, Clase, CLASS_STATES, day, errorText, newIdempotencyKey, occupancy, range, ReservaDeClase, time } from './types';

// Detalle de una clase con sus reservas, asistentes y ausentes (booking.v1,
// HU-08, HU-21, HU-35). El administrador cambia la capacidad, cancela la clase
// o cancela una reserva en nombre del alumno (D-15); el profesor responsable
// solo consulta.
export function ClassDetailPage({ id }: { id: string }) {
  const { authRequest, user } = useSession();
  const [clase, setClase] = useState<Clase | null>(null);
  const [reservas, setReservas] = useState<ReservaDeClase[]>([]);
  const [capacity, setCapacity] = useState('');
  const [reason, setReason] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const admin = user?.role === 'ADMINISTRADOR';

  const load = useCallback(async () => {
    try {
      const res = await authRequest<{ clase: Clase; reservas?: ReservaDeClase[] }>(`/api/v1/classes/${id}`);
      setClase(res.data.clase);
      setReservas(res.data.reservas ?? []);
      setCapacity(String(res.data.clase.capacidad));
    } catch (error) {
      setMessage(errorText(error, 'No se pudo cargar la clase.'));
    }
  }, [authRequest, id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    setMessage('');
    try {
      await action();
      setMessage(done);
      await load();
    } catch (error) {
      setMessage(errorText(error, 'No se pudo completar la operación.'));
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

  function cancelClass(event: FormEvent) {
    event.preventDefault();
    if (!window.confirm('Se cancelan la clase y todas sus reservas activas. ¿Continuar?')) return;
    void run(() => authRequest(`/api/v1/classes/${id}/cancel`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ motivo: reason }),
    }), 'Clase cancelada.');
  }

  function cancelBooking(r: ReservaDeClase) {
    if (!window.confirm(`¿Cancelar la reserva de ${r.alumno_nombre ?? r.alumno_id}?`)) return;
    void run(() => authRequest(`/api/v1/bookings/${r.id}/cancel`, { method: 'POST', headers: { 'Idempotency-Key': newIdempotencyKey() } }), 'Reserva cancelada.');
  }

  if (!clase) {
    return <main className="page">{message ? <p role="alert">{message}</p> : <p className="muted">Cargando…</p>}</main>;
  }

  const editable = admin && clase.estado === 'PROGRAMADA';
  return (
    <main className="page">
      <p><a href={admin ? '#/admin/clases' : '#/prof/clases'}>Volver</a></p>
      <h1>{clase.actividad.nombre} — {day(clase.inicio)} {range(clase)}</h1>
      <p>Profesor: {clase.profesor.nombre} · {occupancy(clase)} · {CLASS_STATES[clase.estado]}</p>
      {message && <p role="status">{message}</p>}

      {editable && (
        <section className="card">
          <form className="inline-form" onSubmit={changeCapacity}>
            <label>Capacidad <input type="number" min={Math.max(1, clase.ocupacion)} max={50} value={capacity} onChange={(e) => setCapacity(e.target.value)} required /></label>
            <button type="submit" disabled={busy}>Guardar capacidad</button>
          </form>
          <form className="inline-form" onSubmit={cancelClass}>
            <label>Motivo <input value={reason} onChange={(e) => setReason(e.target.value)} minLength={3} maxLength={200} required /></label>
            <button type="submit" disabled={busy}>Cancelar clase</button>
          </form>
        </section>
      )}

      <h2>Reservas</h2>
      {reservas.length === 0 ? (
        <p>No hay reservas.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Alumno</th><th>Estado</th><th>Reservó</th><th>Ingreso</th>{editable && <th />}</tr>
          </thead>
          <tbody>
            {reservas.map((r) => (
              <tr key={r.id}>
                <td>{r.alumno_nombre ?? <span className="muted">{r.alumno_id}</span>}</td>
                <td>{BOOKING_STATES[r.estado]}</td>
                <td>{day(r.creada_en)} {time(r.creada_en)}</td>
                <td>{r.ingreso_en ? time(r.ingreso_en) : '—'}</td>
                {editable && (
                  <td>{r.estado === 'CONFIRMADA' && <button type="button" disabled={busy} onClick={() => cancelBooking(r)}>Cancelar</button>}</td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
