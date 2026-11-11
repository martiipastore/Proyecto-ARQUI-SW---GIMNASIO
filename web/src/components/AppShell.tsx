import { KeyboardEvent, ReactNode, useEffect, useRef } from 'react';
import { Role, SessionUser } from '../auth/session';

export interface AppNavigationItem {
  path: string;
  label: string;
}

interface AppShellProps {
  children: ReactNode;
  items: AppNavigationItem[];
  path: string;
  user: SessionUser;
  onLogout(): Promise<void>;
}

const ROLE_LABELS: Record<Role, string> = {
  ADMINISTRADOR: 'Administración',
  PROFESOR: 'Equipo docente',
  NUTRICIONISTA: 'Nutrición',
  ALUMNO: 'Área de alumno',
};

function activePath(items: AppNavigationItem[], path: string): string | undefined {
  return items
    .filter((item) => path === item.path || path.startsWith(`${item.path}/`))
    .sort((a, b) => b.path.length - a.path.length)[0]?.path;
}

function Navigation({ items, path }: { items: AppNavigationItem[]; path: string }) {
  const active = activePath(items, path);
  return (
    <nav className="app-navigation" aria-label="Secciones de la aplicación">
      {items.map((item, index) => (
        <a key={item.path} href={`#${item.path}`} aria-current={active === item.path ? 'page' : undefined}>
          <span aria-hidden="true">{String(index + 1).padStart(2, '0')}</span>
          <strong>{item.label}</strong>
          <i aria-hidden="true">→</i>
        </a>
      ))}
    </nav>
  );
}

export function AppShell({ children, items, path, user, onLogout }: AppShellProps) {
  const active = items.find((item) => item.path === activePath(items, path));
  const initial = user.name.trim().charAt(0).toUpperCase() || 'M';
  const menu = useRef<HTMLDetailsElement>(null);

  // El menú móvil se cierra al navegar y con Escape, devolviendo el foco al botón (DISENO §5).
  useEffect(() => {
    if (menu.current) menu.current.open = false;
    document.querySelector<HTMLElement>('.app-content')?.focus({ preventScroll: true });
  }, [path]);

  function closeMenu(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !menu.current?.open) return;
    menu.current.open = false;
    menu.current.querySelector('summary')?.focus();
  }

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <a className="app-sidebar-brand" href={`#${items[0]?.path ?? '/'}`} aria-label="Ir al inicio de mi espacio">
          <span aria-hidden="true">MF</span>
          <strong>Metal<br /><em>Fitness</em></strong>
        </a>

        <div className="app-role-label">
          <span>Portal</span>
          <strong>{ROLE_LABELS[user.role]}</strong>
        </div>

        <Navigation items={items} path={path} />

        <div className="app-sidebar-footer">
          <a className="app-system-link" href="#/estado"><span aria-hidden="true" /> Estado del sistema</a>
          <div className="app-user-card">
            <span className="app-user-avatar" aria-hidden="true">{initial}</span>
            <a href="#/cuenta">
              <strong>{user.name}</strong>
              <small>Ver mi cuenta</small>
            </a>
            <button type="button" onClick={onLogout} aria-label="Cerrar sesión">↗</button>
          </div>
        </div>
      </aside>

      <div className="app-workspace">
        <header className="app-mobile-bar">
          <a className="app-mobile-brand" href={`#${items[0]?.path ?? '/'}`} aria-label="Ir al inicio de mi espacio">MF</a>
          <div><span>{ROLE_LABELS[user.role]}</span><strong>{active?.label ?? (path === '/cuenta' ? 'Mi cuenta' : 'Detalle')}</strong></div>
          <details className="app-mobile-menu" ref={menu} onKeyDown={closeMenu}>
            <summary>Menú</summary>
            <div>
              <Navigation items={items} path={path} />
              <a className="app-mobile-account" href="#/cuenta">Mi cuenta</a>
              <button type="button" onClick={onLogout}>Cerrar sesión</button>
            </div>
          </details>
        </header>

        <header className="app-toolbar">
          <div>
            <span>{ROLE_LABELS[user.role]}</span>
            <strong>{active?.label ?? (path === '/cuenta' ? 'Mi cuenta' : 'Detalle')}</strong>
          </div>
          <span className="app-toolbar-line" aria-hidden="true" />
          <p><span aria-hidden="true">●</span> Sesión activa</p>
          <a href="#/cuenta">{user.name}</a>
          <button type="button" onClick={onLogout}>Salir</button>
        </header>

        <div className="app-content" tabIndex={-1}>{children}</div>
      </div>
    </div>
  );
}
