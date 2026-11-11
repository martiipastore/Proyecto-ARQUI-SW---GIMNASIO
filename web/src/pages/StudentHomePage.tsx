import { useCallback, useEffect, useState } from 'react';
import { useSession } from '../auth/session';
import { day, Pagina, range, Reserva } from './booking/types';
import { PointsHistory } from './PointsHistory';

interface MembershipSummary {
  activa: boolean;
  membresia_vigente: { tipo: { nombre: string }; fecha_vencimiento: string } | null;
  total: number;
  items: { id: string; tipo: { nombre: string; clases: number | null }; fecha_inicio: string; fecha_vencimiento: string; condicion: string }[];
}

export function StudentHomePage() {
  const { user, authRequest } = useSession();
  const [memberships, setMemberships] = useState<MembershipSummary | null>(null);
  const [bookings, setBookings] = useState<Reserva[] | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    setMessage('');
    const [membershipResult, bookingResult] = await Promise.allSettled([
      authRequest<MembershipSummary>('/api/v1/memberships/me'),
      authRequest<Pagina<Reserva>>('/api/v1/bookings/me?estado=CONFIRMADA&pagina=1&tamano=3'),
    ]);

    const errors: string[] = [];
    if (membershipResult.status === 'fulfilled') setMemberships(membershipResult.value.data);
    else errors.push(membershipResult.reason instanceof Error ? membershipResult.reason.message : 'No se pudo cargar tu membresía.');
    if (bookingResult.status === 'fulfilled') setBookings(bookingResult.value.data.items);
    else errors.push(bookingResult.reason instanceof Error ? bookingResult.reason.message : 'No se pudieron cargar tus próximas clases.');

    setMessage(errors.join(' '));
    setBusy(false);
  }, [authRequest]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const current = memberships?.membresia_vigente;
  const today = new Intl.DateTimeFormat('es-AR', { weekday: 'long', day: 'numeric', month: 'long' }).format(new Date());

  return (
    <main className="page student-dashboard">
      <header className="student-welcome">
        <div>
          <p className="dashboard-kicker">{today}</p>
          <h1>{user ? `Hola, ${user.name}` : 'Inicio'}</h1>
          <p>Todo lo que necesitás para organizar tu próxima sesión.</p>
        </div>
        <div className="dashboard-actions">
          <a className="button button-primary" href="#/app/clases">Buscar una clase <span aria-hidden="true">→</span></a>
          <button type="button" disabled={busy} onClick={() => void refresh()}>{busy ? 'Actualizando…' : 'Actualizar resumen'}</button>
        </div>
      </header>

      {message && <div className="dashboard-error" role="alert"><strong>No pudimos actualizar todo el resumen.</strong><span>{message}</span></div>}

      <section className="student-summary-grid" aria-label="Resumen de tu cuenta">
        <article className="summary-card membership-summary">
          <header><span>01</span><p>Membresía</p><i aria-hidden="true">◆</i></header>
          {memberships === null ? (
            <div className="dashboard-skeleton" aria-label="Cargando membresía" />
          ) : memberships.activa && current ? (
            <div className="membership-current">
              <span className="status-label status-active">Activa</span>
              <h2>{current.tipo.nombre}</h2>
              <p>Vigente hasta <strong>{current.fecha_vencimiento}</strong></p>
            </div>
          ) : (
            <div className="membership-current">
              <span className="status-label status-inactive">Sin vigencia</span>
              <h2>Sin membresía activa</h2>
              <p>Todavía no podés reservar clases. Tus puntos se conservan.</p>
            </div>
          )}
          {memberships && memberships.items.length > 0 && (
            <details className="membership-history">
              <summary>Ver historial ({memberships.total})</summary>
              <ul>{memberships.items.map((membership) => (
                <li key={membership.id}>
                  <strong>{membership.tipo.nombre}</strong>
                  <span>{membership.fecha_inicio} — {membership.fecha_vencimiento}</span>
                  <small>{membership.condicion.toLocaleLowerCase('es-AR')}{membership.tipo.clases !== null && ` · ${membership.tipo.clases} clases`}</small>
                </li>
              ))}</ul>
            </details>
          )}
        </article>

        <article className="summary-card bookings-summary">
          <header><span>02</span><p>Próximas clases</p><a href="#/app/reservas">Ver todas</a></header>
          {bookings === null ? (
            <div className="dashboard-skeleton" aria-label="Cargando próximas clases" />
          ) : bookings.length === 0 ? (
            <div className="dashboard-empty"><strong>Tu agenda está libre.</strong><p>Buscá una clase y reservá tu próximo entrenamiento.</p><a href="#/app/clases">Explorar clases →</a></div>
          ) : (
            <ol className="upcoming-bookings">
              {bookings.map((booking) => (
                <li key={booking.id}>
                  <time dateTime={booking.clase.inicio}><strong>{day(booking.clase.inicio)}</strong><span>{range(booking.clase)}</span></time>
                  <div><strong>{booking.clase.actividad}</strong><span>Reserva confirmada</span></div>
                  <i aria-hidden="true">→</i>
                </li>
              ))}
            </ol>
          )}
        </article>
      </section>

      <PointsHistory />
    </main>
  );
}
