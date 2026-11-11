import { ReactNode, useEffect } from 'react';
import { homeFor, Role, SessionProvider, useSession } from './auth/session';
import { AccountPage } from './pages/AccountPage';
import { ActivationPage } from './pages/ActivationPage';
import { ClubAdminPage } from './pages/admin/ClubAdmin';
import { PeoplePage } from './pages/admin/PeoplePage';
import { PersonPage } from './pages/admin/PersonPage';
import { ActivitiesPage } from './pages/booking/ActivitiesPage';
import { ClassDetailPage } from './pages/booking/ClassDetailPage';
import { ClassesPage } from './pages/booking/ClassesPage';
import { MyBookingsPage } from './pages/booking/MyBookingsPage';
import { MyClassesPage } from './pages/booking/MyClassesPage';
import { ReceptionPage } from './pages/booking/ReceptionPage';
import { AccessDeniedPage, NotFoundPage } from './pages/InfoPages';
import { LoginPage } from './pages/LoginPage';
import { MyStudentsPage } from './pages/MyStudentsPage';
import { InboxPage } from './pages/training/InboxPage';
import { MyPlanPage } from './pages/training/MyPlanPage';
import { StudentPlansPage } from './pages/training/StudentPlansPage';
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
  '/app/clases': { roles: ['ALUMNO'], render: () => <ClassesPage /> },
  '/app/reservas': { roles: ['ALUMNO'], render: () => <MyBookingsPage /> },
  // Etapa 10: entrenamiento, nutrición y consultas (training.v1).
  '/app/plan': { roles: ['ALUMNO'], render: () => <MyPlanPage /> },
  '/nutri/consultas': { roles: ['NUTRICIONISTA'], render: () => <InboxPage /> },
  '/admin': { roles: ['ADMINISTRADOR'], render: () => <Redirect to="/admin/personas" /> },
  '/admin/personas': { roles: ['ADMINISTRADOR'], render: () => <PeoplePage /> },
  '/admin/actividades': { roles: ['ADMINISTRADOR'], render: () => <ActivitiesPage /> },
  '/admin/clases': { roles: ['ADMINISTRADOR'], render: () => <ClassesPage /> },
  // Integraciones del Club (etapa 7); la cuenta de cada persona está en su ficha.
  '/admin/club': { roles: ['ADMINISTRADOR'], render: () => <ClubAdminPage /> },
  // Ingreso por DNI: lo opera el administrador (D-82).
  '/recepcion': { roles: ['ADMINISTRADOR'], render: () => <ReceptionPage /> },
  '/prof/clases': { roles: ['PROFESOR'], render: () => <MyClassesPage /> },
  // Pública como /api/v1/status: sirve para verificar el stack aunque la
  // identidad esté apagada en local (valor por defecto del Compose).
  '/estado': { roles: [], render: () => <SystemStatusPage /> },
  // Los planes (etapa 10) se suman a estos inicios.
  '/prof': { roles: ['PROFESOR'], render: () => <MyStudentsPage title="Mis alumnos" detailBase="#/prof/alumnos/" detailLabel="Plan de entrenamiento" /> },
  '/nutri': { roles: ['NUTRICIONISTA'], render: () => <MyStudentsPage title="Mis pacientes" detailBase="#/nutri/pacientes/" detailLabel="Nutrición" /> },
};

// Rutas con parámetro: el detalle de una persona o una clase lleva su id.
const PERSON_ROUTE = /^\/admin\/personas\/([0-9a-fA-F-]{36})$/;
const CLASS_ROUTE = /^\/(admin|prof)\/clases\/([0-9a-fA-F-]{36})$/;
const PLAN_ROUTE = /^\/(prof\/alumnos|nutri\/pacientes)\/([0-9a-fA-F-]{36})$/;

const NAV: Record<Role, { path: string; label: string }[]> = {
  ALUMNO: [{ path: '/app', label: 'Inicio' }, { path: '/app/clases', label: 'Clases' }, { path: '/app/reservas', label: 'Mis reservas' }, { path: '/app/plan', label: 'Mi plan' }],
  ADMINISTRADOR: [
    { path: '/admin/personas', label: 'Personas' },
    { path: '/admin/actividades', label: 'Actividades' },
    { path: '/admin/clases', label: 'Clases' },
    { path: '/recepcion', label: 'Recepción' },
    { path: '/admin/club', label: 'Club' },
    { path: '/estado', label: 'Estado del sistema' },
  ],
  PROFESOR: [{ path: '/prof/clases', label: 'Mis clases' }, { path: '/prof', label: 'Mis alumnos' }],
  NUTRICIONISTA: [{ path: '/nutri', label: 'Mis pacientes' }, { path: '/nutri/consultas', label: 'Consultas' }],
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
  const clase = CLASS_ROUTE.exec(path);
  if (clase) {
    if (!user) return <Redirect to="/login" />;
    const allowed = clase[1] === 'admin' ? user.role === 'ADMINISTRADOR' : user.role === 'PROFESOR';
    if (!allowed) return <AccessDeniedPage />;
    return <ClassDetailPage key={clase[2]} id={clase[2]} />;
  }
  const plan = PLAN_ROUTE.exec(path);
  if (plan) {
    if (!user) return <Redirect to="/login" />;
    const role = plan[1] === 'prof/alumnos' ? 'PROFESOR' : 'NUTRICIONISTA';
    if (user.role !== role) return <AccessDeniedPage />;
    return <StudentPlansPage key={plan[2]} id={plan[2]} role={role} />;
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
