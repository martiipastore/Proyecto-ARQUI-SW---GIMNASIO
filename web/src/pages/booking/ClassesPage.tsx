import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { Actividad, Clase, CLASS_STATES, day, errorText, newIdempotencyKey, occupancy, Pagina, range, todayPlus } from './types';

// Clases disponibles y reserva (booking.v1, HU-13 y HU-14). El administrador usa
// la misma búsqueda y entra al detalle de cada clase. La reserva se valida
// siempre en el backend.
const PAGE_SIZE = 20;

export function ClassesPage() {
  const { authRequest, user } = useSession();
  const [activities, setActivities] = useState<Actividad[]>([]);
  const [filters, setFilters] = useState({ desde: todayPlus(), hasta: todayPlus(7), actividad: '', soloDisponibles: false });
  const [applied, setApplied] = useState(filters);
  const [page, setPage] = useState(1);
  const [data, setData] = useState<Pagina<Clase> | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState('');

  useEffect(() => {
    authRequest<{ items: Actividad[] }>('/api/v1/activities')
      .then((res) => setActivities(res.data.items.filter((a) => a.activa)))
      .catch(() => setActivities([]));
  }, [authRequest]);

  const load = useCallback(async () => {
    const params = new URLSearchParams({ fecha_desde: applied.desde, fecha_hasta: applied.hasta, pagina: String(page), tamano: String(PAGE_SIZE) });
    if (applied.actividad) params.set('actividad_id', applied.actividad);
    if (applied.soloDisponibles) params.set('solo_disponibles', 'true');
    try {
      const res = await authRequest<Pagina<Clase>>(`/api/v1/classes?${params.toString()}`);
      setData(res.data);
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron cargar las clases.'));
    }
  }, [authRequest, applied, page]);

  useEffect(() => {
    void load();
  }, [load]);

  function search(event: FormEvent) {
    event.preventDefault();
    setMessage('');
    setPage(1);
    setApplied(filters);
  }

  async function reserve(c: Clase) {
    setBusy(c.id);
    setMessage('');
    try {
      await authRequest('/api/v1/bookings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': newIdempotencyKey() },
        body: JSON.stringify({ clase_id: c.id }),
      });
      setMessage(`Reservaste ${c.actividad.nombre} del ${day(c.inicio)} a las ${range(c)}.`);
      await load();
    } catch (error) {
      setMessage(errorText(error, 'No se pudo reservar.'));
    } finally {
      setBusy('');
    }
  }

  const student = user?.role === 'ALUMNO';
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;

  return (
    <main className={`page${student ? '' : ' admin-page'}`}>
      {student ? <h1>Clases</h1> : (
        <header className="admin-page-heading">
          <div>
            <p className="admin-eyebrow">Agenda operativa</p>
            <h1>Clases</h1>
            <p>Consultá la programación, revisá la ocupación y abrí el detalle de cada clase.</p>
          </div>
          <span className="admin-page-index" aria-hidden="true">03</span>
        </header>
      )}
      <form className={`filters${student ? '' : ' admin-toolbar-panel'}`} onSubmit={search}>
        <label>Desde <input type="date" value={filters.desde} onChange={(e) => setFilters({ ...filters, desde: e.target.value })} required /></label>
        <label>Hasta <input type="date" value={filters.hasta} onChange={(e) => setFilters({ ...filters, hasta: e.target.value })} required /></label>
        <label>Actividad
          <select value={filters.actividad} onChange={(e) => setFilters({ ...filters, actividad: e.target.value })}>
            <option value="">Todas</option>
            {activities.map((a) => <option key={a.id} value={a.id}>{a.nombre}</option>)}
          </select>
        </label>
        <label className="check"><input type="checkbox" checked={filters.soloDisponibles} onChange={(e) => setFilters({ ...filters, soloDisponibles: e.target.checked })} /> Solo con lugar</label>
        <button type="submit">Buscar</button>
      </form>
      {message && <p role="status">{message}</p>}
      {data === null ? (
        !message && <p className="muted">Cargando…</p>
      ) : data.items.length === 0 ? (
        <p>No hay clases para esos filtros.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Día</th><th>Horario</th><th>Actividad</th><th>Profesor</th><th>Ocupación</th><th>Estado</th><th /></tr>
          </thead>
          <tbody>
            {data.items.map((c) => (
              <tr key={c.id}>
                <td>{day(c.inicio)}</td>
                <td>{range(c)}</td>
                <td>{c.actividad.nombre}</td>
                <td>{c.profesor.nombre}</td>
                <td>{occupancy(c)}</td>
                <td>{CLASS_STATES[c.estado]}</td>
                {!student && <td><a href={`#/admin/clases/${c.id}`}>Ver</a></td>}
                {student && (
                  <td>
                    {c.reservada_por_mi ? (
                      <span className="badge">Ya reservada</span>
                    ) : c.reservable ? (
                      <button type="button" disabled={busy !== ''} onClick={() => void reserve(c)}>{busy === c.id ? 'Reservando…' : 'Reservar'}</button>
                    ) : (
                      <span className="muted">{c.disponibles === 0 && c.estado === 'PROGRAMADA' ? 'Completa' : 'No disponible'}</span>
                    )}
                  </td>
                )}
              </tr>
            ))}
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
