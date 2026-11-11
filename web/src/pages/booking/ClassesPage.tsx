import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { EmptyState, Notice, NoticeState, PageHeading } from '../../components/PageParts';
import { Actividad, Clase, CLASS_STATES, day, errorText, newIdempotencyKey, occupancy, Pagina, range, todayPlus } from './types';

// Clases disponibles y reserva (booking.v1, HU-13 y HU-14). El administrador usa
// la misma búsqueda y entra al detalle de cada clase. La reserva se valida
// siempre en el backend.
const PAGE_SIZE = 20;

export function ClassesPage() {
  const { authRequest, user } = useSession();
  const [activities, setActivities] = useState<Actividad[]>([]);
  const initial = { desde: todayPlus(), hasta: todayPlus(7), actividad: '', soloDisponibles: false };
  const [filters, setFilters] = useState(initial);
  const [applied, setApplied] = useState(initial);
  const [page, setPage] = useState(1);
  const [data, setData] = useState<Pagina<Clase> | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(null);
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
      setNotice({ tone: 'error', text: errorText(error, 'No se pudieron cargar las clases.') });
    }
  }, [authRequest, applied, page]);

  useEffect(() => {
    void load();
  }, [load]);

  function search(event: FormEvent) {
    event.preventDefault();
    setNotice(null);
    setPage(1);
    setApplied(filters);
  }

  function clearFilters() {
    setNotice(null);
    setPage(1);
    setFilters(initial);
    setApplied(initial);
  }

  async function reserve(c: Clase) {
    setBusy(c.id);
    setNotice(null);
    try {
      await authRequest('/api/v1/bookings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': newIdempotencyKey() },
        body: JSON.stringify({ clase_id: c.id }),
      });
      setNotice({ tone: 'success', plate: true, text: `Reserva confirmada: ${c.actividad.nombre} del ${day(c.inicio)}, ${range(c)}.` });
      await load();
    } catch (error) {
      setNotice({ tone: 'error', text: errorText(error, 'No se pudo reservar.') });
    } finally {
      setBusy('');
    }
  }

  const student = user?.role === 'ALUMNO';
  const pages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;
  const activeName = activities.find((a) => a.id === applied.actividad)?.nombre;
  const filtered = applied.actividad !== '' || applied.soloDisponibles || applied.desde !== initial.desde || applied.hasta !== initial.hasta;

  return (
    <main className="page admin-page">
      {student ? (
        <PageHeading eyebrow="Agenda de entrenamiento" title="Clases" description="Elegí actividad y fechas, mirá cuántos lugares quedan y reservá en un paso." index="02" />
      ) : (
        <PageHeading eyebrow="Agenda operativa" title="Clases" description="Consultá la programación, revisá la ocupación y abrí el detalle de cada clase." index="03" />
      )}
      <form className="filters admin-toolbar-panel" onSubmit={search} aria-label="Filtros de clases">
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
      {filtered && (
        <p className="active-filters">
          <span>Filtros activos:</span>
          <b>{applied.desde} → {applied.hasta}</b>
          {activeName && <b>{activeName}</b>}
          {applied.soloDisponibles && <b>Solo con lugar</b>}
          <button type="button" className="button-secondary" onClick={clearFilters}>Limpiar filtros</button>
        </p>
      )}
      <Notice key={notice?.text} notice={notice} />
      {data === null ? (
        notice?.tone !== 'error' && <div className="dashboard-skeleton" aria-label="Cargando clases" />
      ) : data.items.length === 0 ? (
        <EmptyState title="Sin clases para esos filtros">Probá con otras fechas, otra actividad o sin “Solo con lugar”.</EmptyState>
      ) : student ? (
        <ol className="class-grid" aria-label="Clases encontradas">
          {data.items.map((c) => <ClassCard key={c.id} clase={c} busy={busy} onReserve={() => void reserve(c)} />)}
        </ol>
      ) : (
        <table className="data-table responsive-table">
          <thead>
            <tr><th>Día</th><th>Horario</th><th>Actividad</th><th>Profesor</th><th>Ocupación</th><th>Estado</th><th><span className="sr-only">Acciones</span></th></tr>
          </thead>
          <tbody>
            {data.items.map((c) => (
              <tr key={c.id}>
                <td data-label="Día">{day(c.inicio)}</td>
                <td data-label="Horario">{range(c)}</td>
                <td data-label="Actividad">{c.actividad.nombre}</td>
                <td data-label="Profesor">{c.profesor.nombre}</td>
                <td data-label="Ocupación">{occupancy(c)}</td>
                <td data-label="Estado">{CLASS_STATES[c.estado]}</td>
                <td><a href={`#/admin/clases/${c.id}`}>Ver detalle</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {data && pages > 1 && (
        <nav className="pagination" aria-label="Páginas de clases">
          <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>Anterior</button>
          <span>Página {page} de {pages}</span>
          <button type="button" disabled={page >= pages} onClick={() => setPage(page + 1)}>Siguiente</button>
        </nav>
      )}
    </main>
  );
}

function ClassCard({ clase: c, busy, onReserve }: { clase: Clase; busy: string; onReserve: () => void }) {
  const ratio = c.capacidad > 0 ? Math.min(1, c.ocupacion / c.capacidad) : 1;
  const full = c.disponibles === 0 && c.estado === 'PROGRAMADA';
  const tone = c.reservada_por_mi ? 'mine' : full ? 'full' : c.reservable ? (c.disponibles <= 3 ? 'few' : 'open') : 'closed';
  const status = c.reservada_por_mi ? 'Ya reservada' : full ? 'Completa' : c.reservable ? `${c.disponibles} ${c.disponibles === 1 ? 'lugar libre' : 'lugares libres'}` : 'No disponible';
  return (
    <li className={`class-card class-card-${tone}`}>
      <header>
        <time dateTime={c.inicio}><strong>{day(c.inicio)}</strong><span>{range(c)}</span></time>
        <span className="class-card-state">{c.reservada_por_mi ? '✓ ' : full ? '■ ' : ''}{status}</span>
      </header>
      <h2>{c.actividad.nombre}</h2>
      <p>Profesor: {c.profesor.nombre}</p>
      <div className="occupancy" aria-label={occupancy(c)}>
        <span className="occupancy-track"><i style={{ transform: `scaleX(${ratio})` }} /></span>
        <small>{c.ocupacion}/{c.capacidad} ocupados</small>
      </div>
      {c.reservada_por_mi ? (
        <a className="class-card-link" href="#/app/reservas">Ver en mis reservas →</a>
      ) : c.reservable ? (
        <button type="button" disabled={busy !== ''} onClick={onReserve}>{busy === c.id ? 'Reservando…' : 'Reservar'}</button>
      ) : (
        <button type="button" disabled>{full ? 'Sin lugar' : CLASS_STATES[c.estado]}</button>
      )}
    </li>
  );
}
