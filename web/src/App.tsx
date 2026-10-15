import { SystemStatusPage } from './pages/SystemStatusPage';

export function App() {
  return (
    <>
      <nav className="topbar">
        <span className="brand">Gimnasio</span>
        <span className="muted">Sistema integral de gestión</span>
      </nav>
      <SystemStatusPage />
    </>
  );
}
