import { useSession } from '../auth/session';
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
    <main className="page narrow">
      <h1>Mi cuenta</h1>
      <dl className="club-card">
        <dt>Nombre</dt>
        <dd>{user.name}</dd>
        <dt>Correo electrónico</dt>
        <dd>{user.email}</dd>
        <dt>Rol</dt>
        <dd>{ROLE_NAMES[user.role] ?? user.role}</dd>
      </dl>
      <PointsHistory />
      <button type="button" onClick={async () => { await logout(); navigate('/login'); }}>Cerrar sesión</button>
    </main>
  );
}
