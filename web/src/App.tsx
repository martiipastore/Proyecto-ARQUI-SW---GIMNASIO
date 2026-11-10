import { ReactNode, useEffect } from 'react';
import { homeFor, Role, SessionProvider, useSession } from './auth/session';
import { AccountPage } from './pages/AccountPage';
import { ActivationPage } from './pages/ActivationPage';
import { PeoplePage } from './pages/admin/PeoplePage';
import { PersonPage } from './pages/admin/PersonPage';
import { AccessDeniedPage, NotFoundPage } from './pages/InfoPages';
import { LoginPage } from './pages/LoginPage';
import { MyStudentsPage } from './pages/MyStudentsPage';
import { StudentHomePage } from './pages/StudentHomePage';
import { SystemStatusPage } from './pages/SystemStatusPage';
import { navigate, takeActivationToken, useRoute } from './router';

// El token de "#activate=" se toma una sola vez al cargar (router.ts).
const activationToken = takeActivationToken();

const ALL_ROLES: Role[] = ['ADMINISTRADOR', 'PROFESOR', 'NUTRICIONISTA', 'ALUMNO'];

interface RouteDef {
  /** Roles permitidos; vacío significa ruta pública. */
  roles: Role[];
  render: () => ReactNode;
}

// Rutas de la etapa 1 (§2) implementadas en la etapa 2; el resto se agrega en la
// etapa que implementa cada contrato. El backend vuelve a autorizar siempre.
const ROUTES: Record<string, RouteDef> = {
  '/login': { roles: [], render: () => <LoginPage /> },
  '/activar': { roles: [], render: () => <ActivationPage token={activationToken} /> },
  '/cuenta': { roles: ALL_ROLES, render: () => <AccountPage /> },
  '/app': { roles: ['ALUMNO'], render: () => <StudentHomePage /> },
  '/admin': { roles: ['ADMINISTRADOR'], render: () => <Redirect to="/admin/personas" /> },
  '/admin/personas': { roles: ['ADMINISTRADOR'], render: () => <PeoplePage /> },
  // Pública como /api/v1/status: sirve para verificar el stack aunque la
  // identidad esté apagada en local (valor por defecto del Compose).
  '/estado': { roles: [], render: () => <SystemStatusPage /> },
  // Mis clases (etapa 4) y planes (etapa 10) se suman a estos inicios.
  '/prof': { roles: ['PROFESOR'], render: () => <MyStudentsPage title="Mis alumnos" /> },
  '/nutri': { roles: ['NUTRICIONISTA'], render: () => <MyStudentsPage title="Mis pacientes" /> },
};

// Rutas con parámetro: el detalle de una persona lleva su id en la dirección.
const PERSON_ROUTE = /^\/admin\/personas\/([0-9a-fA-F-]{36})$/;

const NAV: Record<Role, { path: string; label: string }[]> = {
  ALUMNO: [{ path: '/app', label: 'Inicio' }],
  ADMINISTRADOR: [{ path: '/admin/personas', label: 'Personas' }, { path: '/estado', label: 'Estado del sistema' }],
  PROFESOR: [{ path: '/prof', label: 'Mis alumnos' }],
  NUTRICIONISTA: [{ path: '/nutri', label: 'Mis pacientes' }],
};

function Redirect({ to }: { to: string }) {
  useEffect(() => navigate(to), [to]);
  return null;
}

function Screen({ path }: { path: string }) {
  const { user } = useSession();
  if (path === '/') {
    return <Redirect to={user ? homeFor(user.role) : '/login'} />;
  }
  const person = PERSON_ROUTE.exec(path);
  if (person) {
    if (!user) return <Redirect to="/login" />;
    if (user.role !== 'ADMINISTRADOR') return <AccessDeniedPage />;
    return <PersonPage key={person[1]} id={person[1]} />;
  }
  const route = ROUTES[path];
  if (!route) return <NotFoundPage />;
  if (route.roles.length === 0) {
    // Con sesión iniciada, el login lleva directo al inicio del rol.
    if (path === '/login' && user) return <Redirect to={homeFor(user.role)} />;
    return <>{route.render()}</>;
  }
  if (!user) return <Redirect to="/login" />;
  if (!route.roles.includes(user.role)) return <AccessDeniedPage />;
  return <>{route.render()}</>;
}

function Shell() {
  const path = useRoute();
  const { user, logout } = useSession();
  return (
    <>
      <nav className="topbar" aria-label="Principal">
        <span className="brand">METALFITNESS</span>
        {user && NAV[user.role].map((item) => (
          <a key={item.path} href={`#${item.path}`} aria-current={path === item.path ? 'page' : undefined}>{item.label}</a>
        ))}
        <span className="topbar-end">
          {user ? (
            <>
              <a href="#/cuenta" aria-current={path === '/cuenta' ? 'page' : undefined}>{user.name}</a>
              <button type="button" onClick={async () => { await logout(); navigate('/login'); }}>Salir</button>
            </>
          ) : (
            <>
              <a href="#/estado">Estado del sistema</a>
              <a href="#/login">Ingresar</a>
            </>
          )}
        </span>
      </nav>
      <Screen path={path} />
    </>
  );
}

export function App() {
  return (
    <SessionProvider>
      <Shell />
    </SessionProvider>
  );
}
