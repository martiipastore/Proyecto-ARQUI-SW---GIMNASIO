import { useEffect, useState } from 'react';
import { useSession } from '../auth/session';
import { Assignment, fullName } from './admin/types';

// Mis alumnos o pacientes asignados (users.v1, HU-21, CA-02.4, RN-30). Los
// alumnos con reserva en las clases del profesor se suman con booking (D-09).
export function MyStudentsPage({ title, detailBase, detailLabel }: { title: string; detailBase: string; detailLabel: string }) {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Assignment[] | null>(null);
  const [message, setMessage] = useState('');

  useEffect(() => {
    authRequest<{ items: Assignment[] }>('/api/v1/users/me/students')
      .then((res) => setItems(res.data.items))
      .catch((error: unknown) => setMessage(error instanceof Error ? error.message : 'No se pudo cargar el listado.'));
  }, [authRequest]);

  return (
    <main className="page">
      <h1>{title}</h1>
      {message && <p role="alert">{message}</p>}
      {items === null ? (
        !message && <p className="muted">Cargando…</p>
      ) : items.length === 0 ? (
        <p>Todavía no tenés personas asignadas.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Nombre</th><th>Email</th><th>Teléfono</th><th>Asignado desde</th><th>{detailLabel}</th></tr>
          </thead>
          <tbody>
            {items.map((a) => a.alumno && (
              <tr key={a.id}>
                <td>{fullName(a.alumno)}</td>
                <td>{a.alumno.email}</td>
                <td>{a.alumno.telefono ?? '—'}</td>
                <td>{a.desde.slice(0, 10)}</td>
                <td><a href={`${detailBase}${a.alumno_id}`}>Ver</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
