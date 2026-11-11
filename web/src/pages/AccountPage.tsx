import { useSession } from '../auth/session';
import { PageHeading } from '../components/PageParts';
import { navigate } from '../router';
import { PointsHistory } from './PointsHistory';

const ROLE_NAMES: Record<string, string> = {
  ADMINISTRADOR: 'Administrador',
  PROFESOR: 'Profesor',
  NUTRICIONISTA: 'Nutricionista',
  ALUMNO: 'Alumno',
};

// Mi cuenta (auth.v1): datos de la sesión verificada y cierre de sesión.
export function AccountPage() {
  const { user, logout } = useSession();
  if (!user) return null;
  return (
    <main className="page admin-page">
      <PageHeading eyebrow="Tu sesión" title="Mi cuenta" description="Datos verificados de tu sesión. La sesión vence a los 15 minutos; después volvés a ingresar." />
      <dl className="club-card">
        <dt>Nombre</dt>
        <dd>{user.name}</dd>
        <dt>Correo electrónico</dt>
        <dd>{user.email}</dd>
        <dt>Rol</dt>
        <dd>{ROLE_NAMES[user.role] ?? user.role}</dd>
      </dl>
      <PointsHistory />
      <button type="button" className="button-secondary account-logout" onClick={async () => { await logout(); navigate('/login'); }}>Cerrar sesión</button>
    </main>
  );
}
