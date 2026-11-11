import { useEffect, useState } from 'react';
import { useSession } from '../auth/session';
import { EmptyState, Notice, PageHeading } from '../components/PageParts';
import { Assignment, fullName } from './admin/types';

// Mis alumnos o pacientes asignados (users.v1, HU-21, CA-02.4, RN-30). Los
// alumnos con reserva en las clases del profesor se suman con booking (D-09).
export function MyStudentsPage({ title, detailBase, detailLabel }: { title: string; detailBase: string; detailLabel: string }) {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Assignment[] | null>(null);
  const [message, setMessage] = useState('');
  const nutrition = detailBase.includes('nutri');

  useEffect(() => {
    authRequest<{ items: Assignment[] }>('/api/v1/users/me/students')
      .then((res) => setItems(res.data.items))
      .catch((error: unknown) => setMessage(error instanceof Error ? error.message : 'No se pudo cargar el listado.'));
  }, [authRequest]);

  return (
    <main className="page admin-page">
      <PageHeading
        eyebrow={nutrition ? 'Nutrición' : 'Equipo docente'}
        title={title}
        description={nutrition ? 'Las personas que tenés asignadas. Abrí cada ficha para cargar su plan alimenticio y sus mediciones.' : 'Las personas que tenés asignadas. Abrí cada ficha para ver o crear su plan de entrenamiento.'}
        index={nutrition ? '01' : '02'}
      />
      <Notice notice={message ? { tone: 'error', text: message } : null} />
      {items === null ? (
        !message && <div className="dashboard-skeleton" aria-label="Cargando personas" />
      ) : items.length === 0 ? (
        <EmptyState title="Sin personas asignadas">El administrador asigna alumnos desde su ficha.</EmptyState>
      ) : (
        <table className="data-table responsive-table">
          <thead>
            <tr><th>Nombre</th><th>Email</th><th>Teléfono</th><th>Asignado desde</th><th><span className="sr-only">{detailLabel}</span></th></tr>
          </thead>
          <tbody>
            {items.map((a) => a.alumno && (
              <tr key={a.id}>
                <td data-label="Nombre"><a href={`${detailBase}${a.alumno_id}`}>{fullName(a.alumno)}</a></td>
                <td data-label="Email">{a.alumno.email}</td>
                <td data-label="Teléfono">{a.alumno.telefono ?? '—'}</td>
                <td data-label="Desde">{a.desde.slice(0, 10)}</td>
                <td><a href={`${detailBase}${a.alumno_id}`}>{detailLabel} →</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
