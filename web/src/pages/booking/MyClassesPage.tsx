import { useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { EmptyState, Notice, PageHeading } from '../../components/PageParts';
import { Clase, CLASS_STATES, day, errorText, occupancy, range } from './types';

// Mis clases del profesor con su ocupación (booking.v1, HU-21, CA-21.1). El
// detalle muestra los alumnos con reserva.
export function MyClassesPage() {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Clase[] | null>(null);
  const [message, setMessage] = useState('');

  useEffect(() => {
    authRequest<{ items: Clase[] }>('/api/v1/classes/mine')
      .then((res) => setItems(res.data.items))
      .catch((error: unknown) => setMessage(errorText(error, 'No se pudieron cargar tus clases.')));
  }, [authRequest]);

  return (
    <main className="page admin-page">
      <PageHeading eyebrow="Equipo docente" title="Mis clases" description="Las clases a tu cargo desde hoy, con su ocupación. Abrí cada una para ver quién reservó y quién ingresó." index="01" />
      <Notice notice={message ? { tone: 'error', text: message } : null} />
      {items === null ? (
        !message && <div className="dashboard-skeleton" aria-label="Cargando clases" />
      ) : items.length === 0 ? (
        <EmptyState title="Sin clases a cargo">No tenés clases programadas desde hoy.</EmptyState>
      ) : (
        <table className="data-table responsive-table">
          <thead>
            <tr><th>Día</th><th>Horario</th><th>Actividad</th><th>Ocupación</th><th>Estado</th><th><span className="sr-only">Acciones</span></th></tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id}>
                <td data-label="Día">{day(c.inicio)}</td>
                <td data-label="Horario">{range(c)}</td>
                <td data-label="Actividad">{c.actividad.nombre}</td>
                <td data-label="Ocupación">{occupancy(c)}</td>
                <td data-label="Estado">{CLASS_STATES[c.estado]}</td>
                <td><a href={`#/prof/clases/${c.id}`}>Ver alumnos</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
