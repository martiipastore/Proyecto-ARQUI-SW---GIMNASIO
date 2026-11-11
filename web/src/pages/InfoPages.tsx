import { homeFor, useSession } from '../auth/session';
import { navigate } from '../router';

function BackHome() {
  const { user } = useSession();
  return (
    <button type="button" className="button button-primary" onClick={() => navigate(user ? homeFor(user.role) : '/login')}>
      {user ? 'Volver a mi inicio' : 'Ir al ingreso'} <span aria-hidden="true">→</span>
    </button>
  );
}

function InfoScreen({ code, title, text }: { code: string; title: string; text: string }) {
  return (
    <main className="page info-screen">
      <span className="info-code" aria-hidden="true">{code}</span>
      <h1>{title}</h1>
      <p>{text}</p>
      <BackHome />
    </main>
  );
}

export function AccessDeniedPage() {
  return <InfoScreen code="403" title="Acceso denegado" text="Tu rol no permite ver esta sección." />;
}

export function NotFoundPage() {
  return <InfoScreen code="404" title="Página inexistente" text="La dirección no corresponde a ninguna sección." />;
}
