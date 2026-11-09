import { homeFor, useSession } from '../auth/session';
import { navigate } from '../router';

function BackHome() {
  const { user } = useSession();
  return (
    <button type="button" onClick={() => navigate(user ? homeFor(user.role) : '/login')}>
      {user ? 'Volver a mi inicio' : 'Ir al ingreso'}
    </button>
  );
}

export function AccessDeniedPage() {
  return (
    <main className="page narrow">
      <h1>Acceso denegado</h1>
      <p>Tu rol no permite ver esta sección.</p>
      <BackHome />
    </main>
  );
}

export function NotFoundPage() {
  return (
    <main className="page narrow">
      <h1>Página inexistente</h1>
      <p>La dirección no corresponde a ninguna sección.</p>
      <BackHome />
    </main>
  );
}

// Inicio de los roles cuyas pantallas se implementan en etapas posteriores. No
// simula funcionalidad: indica qué etapa del plan la incorpora.
export function PendingPage({ title, items }: { title: string; items: string[] }) {
  return (
    <main className="page">
      <h1>{title}</h1>
      <p>Estas secciones se incorporan en las próximas etapas de la entrega final:</p>
      <ul>
        {items.map((item) => <li key={item}>{item}</li>)}
      </ul>
    </main>
  );
}
