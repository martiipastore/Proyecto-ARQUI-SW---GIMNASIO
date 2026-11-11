import { ReactNode } from 'react';
import { PlateLoad } from './PlateLoad';

// Piezas visuales comunes a las pantallas autenticadas (DISENO_FRONTEND §5:
// PageHeader, EmptyState, Toast). Solo presentación: no conocen contratos.

export function PageHeading({ eyebrow, title, description, index, children }: {
  eyebrow: string;
  title: ReactNode;
  description?: ReactNode;
  index?: string;
  children?: ReactNode;
}) {
  return (
    <header className="admin-page-heading">
      <div>
        <p className="admin-eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
        {children}
      </div>
      {index && <span className="admin-page-index" aria-hidden="true">{index}</span>}
    </header>
  );
}

export function SectionHeading({ index, kicker, title }: { index: string; kicker?: string; title: ReactNode }) {
  return (
    <div className="admin-section-heading">
      <span aria-hidden="true">{index}</span>
      <div>{kicker && <p>{kicker}</p>}<h2>{title}</h2></div>
    </div>
  );
}

export type NoticeTone = 'success' | 'error' | 'info';

export interface NoticeState {
  tone: NoticeTone;
  text: string;
  /** Muestra el disco que se carga (reserva confirmada). */
  plate?: boolean;
}

/** Aviso del resultado de una acción, anunciado a lectores de pantalla. */
export function Notice({ notice }: { notice: NoticeState | null }) {
  if (!notice) return null;
  const icon = notice.tone === 'success' ? '✓' : notice.tone === 'error' ? '!' : 'i';
  return (
    <div className={`notice notice-${notice.tone}`} role={notice.tone === 'error' ? 'alert' : 'status'} aria-live="polite">
      {notice.plate ? <PlateLoad /> : <span className="notice-icon" aria-hidden="true">{icon}</span>}
      <p>{notice.text}</p>
    </div>
  );
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="dashboard-empty empty-panel">
      <strong>{title}</strong>
      {children && <div>{children}</div>}
    </div>
  );
}
