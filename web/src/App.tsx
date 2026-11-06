import { SystemStatusPage } from './pages/SystemStatusPage';
import { ClubAccountPage } from './pages/ClubAccountPage';

export function App() {
  return (
    <>
      <nav className="topbar">
        <span className="brand">Gimnasio</span>
        <span className="muted">Sistema integral de gestión</span>
      </nav>
      <ClubAccountPage />
      <details><summary className="page">Estado del sistema</summary><SystemStatusPage /></details>
    </>
  );
}
