import { useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
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
    <main className="page">
      <h1>Mis clases</h1>
      {message && <p role="alert">{message}</p>}
      {items === null ? (
        !message && <p className="muted">Cargando…</p>
      ) : items.length === 0 ? (
        <p>No tenés clases a cargo desde hoy.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Día</th><th>Horario</th><th>Actividad</th><th>Ocupación</th><th>Estado</th><th /></tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id}>
                <td>{day(c.inicio)}</td>
                <td>{range(c)}</td>
                <td>{c.actividad.nombre}</td>
                <td>{occupancy(c)}</td>
                <td>{CLASS_STATES[c.estado]}</td>
                <td><a href={`#/prof/clases/${c.id}`}>Alumnos</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
