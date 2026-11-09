import { ReactNode, useEffect } from 'react';
import { homeFor, Role, SessionProvider, useSession } from './auth/session';
import { AccountPage } from './pages/AccountPage';
import { ActivationPage } from './pages/ActivationPage';
import { AccessDeniedPage, NotFoundPage, PendingPage } from './pages/InfoPages';
import { LoginPage } from './pages/LoginPage';
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
  '/admin': {
    roles: ['ADMINISTRADOR'],
    render: () => <PendingPage title="Administración" items={['Alumnos, profesionales y asignaciones (etapa 3)', 'Actividades, horarios y clases (etapa 4)', 'Integraciones del Club (etapa 7)']} />,
  },
  // Pública como /api/v1/status: sirve para verificar el stack aunque la
  // identidad esté apagada en local (valor por defecto del Compose).
  '/estado': { roles: [], render: () => <SystemStatusPage /> },
  '/prof': {
    roles: ['PROFESOR'],
    render: () => <PendingPage title="Profesor" items={['Mis clases y sus alumnos (etapa 4)', 'Mis alumnos y planes de entrenamiento (etapa 10)']} />,
  },
  '/nutri': {
    roles: ['NUTRICIONISTA'],
    render: () => <PendingPage title="Nutricionista" items={['Mis pacientes, planes alimenticios y mediciones (etapa 10)', 'Consultas de pacientes (etapa 10)']} />,
  },
};

const NAV: Record<Role, { path: string; label: string }[]> = {
  ALUMNO: [{ path: '/app', label: 'Inicio' }],
  ADMINISTRADOR: [{ path: '/admin', label: 'Administración' }, { path: '/estado', label: 'Estado del sistema' }],
  PROFESOR: [{ path: '/prof', label: 'Inicio' }],
  NUTRICIONISTA: [{ path: '/nutri', label: 'Inicio' }],
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
