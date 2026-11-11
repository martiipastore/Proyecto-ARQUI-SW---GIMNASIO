import { DATE_TIME } from '../../format';
import { FormEvent, useCallback, useEffect, useState } from 'react';
import { ApiError } from '../../api/client';
import { useSession } from '../../auth/session';
import { useConfirm } from '../../components/ConfirmDialog';
import { newIdempotencyKey } from './types';

// Administración del Club (etapa 7, benefits.v1): integraciones (HU-10),
// cuenta de una persona y reversión de movimientos (HU-11, HU-35, HU-CB-07).
// Catálogo, canjes y débitos quedan fuera por D-76.

export interface Integration {
  id: string;
  nombre: string;
  activo: boolean;
  estado: 'ACTIVO' | 'INACTIVO';
  permite_consulta: boolean;
  permite_puntos: boolean;
  permite_registro: boolean;
  requiere_evento: boolean;
  max_puntos_operacion: number;
  max_puntos_dia: number | null;
  max_solicitudes_minuto: number;
  vinculaciones: number;
  creada_en: string;
}

interface AdminMovement {
  id: string; fecha: string; motivo: string; puntos: number; secuencia: number;
  estado: 'CONFIRMADO' | 'REVERTIDO';
  origen: 'ASISTENCIA' | 'INASISTENCIA' | 'PARTNER' | 'REVERSION';
  partner_id: string; revierte_id?: string; revertido_por?: string;
}

interface AdminAccount {
  usuario_id: string; saldo: number; creada_en: string;
  vinculaciones: { partner_id: string; partner: string; external_user_id: string }[];
  items: AdminMovement[]; total: number; pagina: number; tamano: number;
}

const CLUB_ERRORS: Record<string, string> = {
  SALDO_INSUFICIENTE: 'No se puede revertir: el saldo quedaría negativo.',
  TRANSICION_INVALIDA: 'El movimiento ya fue revertido o es una reversión.',
  CONFLICTO_IDENTIDAD: 'Ese identificador externo o esta cuenta ya están vinculados de otra forma en la integración.',
  CONFLICTO_IDEMPOTENCIA: 'La operación ya se envió con otros datos. Recargá e intentá de nuevo.',
  VALIDACION: 'Revisá los datos: algún campo está fuera de los límites.',
};

function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.problem && CLUB_ERRORS[error.problem.code]) {
    return CLUB_ERRORS[error.problem.code];
  }
  return error instanceof Error ? error.message : fallback;
}

const ORIGIN_LABELS: Record<AdminMovement['origen'], string> = {
  ASISTENCIA: 'Asistencia', INASISTENCIA: 'Ausencia', PARTNER: 'Integración', REVERSION: 'Reversión',
};

type ConfigForm = Omit<Integration, 'id' | 'estado' | 'vinculaciones' | 'creada_en'>;

const NEW_CONFIG: ConfigForm = {
  nombre: '', activo: true, permite_consulta: true, permite_puntos: true, permite_registro: false,
  requiere_evento: true, max_puntos_operacion: 1000000, max_puntos_dia: null, max_solicitudes_minuto: 120,
};

/** Pantalla /admin/club: alta, configuración y rotación de credenciales. */
export function ClubAdminPage() {
  const { authRequest } = useSession();
  const [dialog, confirm] = useConfirm();
  const [items, setItems] = useState<Integration[] | null>(null);
  const [message, setMessage] = useState('');
  const [issued, setIssued] = useState<{ nombre: string; key: string } | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await authRequest<{ items: Integration[] }>('/api/v1/benefits/partners');
      setItems(res.data.items);
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron cargar las integraciones.'));
    }
  }, [authRequest]);

  useEffect(() => { void load(); }, [load]);

  async function run(action: () => Promise<void>, done: string) {
    setBusy(true); setMessage('');
    try { await action(); setMessage(done); await load(); }
    catch (error) { setMessage(errorText(error, 'No se pudo completar la operación.')); }
    finally { setBusy(false); }
  }

  const create = (config: ConfigForm) => run(async () => {
    const { activo: _activo, ...body } = config;
    const res = await authRequest<Integration & { api_key: string }>('/api/v1/benefits/partners', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    setIssued({ nombre: res.data.nombre, key: res.data.api_key });
  }, 'Integración creada y activa.');

  const update = (id: string, config: ConfigForm) => run(async () => {
    await authRequest(`/api/v1/benefits/partners/${id}`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config),
    });
    setEditing(null);
  }, 'Configuración guardada.');

  const rotate = async (item: Integration) => {
    const ok = await confirm({ title: `Rotar API key de ${item.nombre}`, message: 'La key actual deja de funcionar en el momento. La nueva se muestra una sola vez.', confirmLabel: 'Rotar key', danger: true });
    if (ok === null) return;
    void run(async () => {
      const res = await authRequest<{ id: string; api_key: string }>(`/api/v1/benefits/partners/${item.id}/rotate-key`, { method: 'POST' });
      setIssued({ nombre: item.nombre, key: res.data.api_key });
    }, 'Credencial rotada.');
  };

  return (
    <main className="page admin-page">
      {dialog}
      <header className="admin-page-heading">
        <div>
          <p className="admin-eyebrow">Capacidad compartida</p>
          <h1>Club de Beneficios</h1>
          <p>Administrá las integraciones y credenciales que conectan sistemas externos con el club.</p>
        </div>
        <span className="admin-page-index" aria-hidden="true">05</span>
      </header>
      <h2 className="admin-content-title">Integraciones</h2>
      <p className="muted">Sistemas de otros grupos que acreditan puntos por la API de partners v2. Para ver la cuenta de una persona, revertir movimientos o vincularla, abrí su ficha en <a href="#/admin/personas">Personas</a>.</p>
      {message && <p role="status" aria-live="polite">{message}</p>}
      {issued && (
        <section className="club-card" role="alert" aria-label="API key emitida">
          <h2>API key de {issued.nombre}</h2>
          <p><strong>Copiala ahora: no se vuelve a mostrar.</strong> El sistema guarda solo su hash.</p>
          <p><code className="secret">{issued.key}</code></p>
          <button type="button" onClick={() => void navigator.clipboard?.writeText(issued.key)}>Copiar</button>{' '}
          <button type="button" onClick={() => setIssued(null)}>Ya la guardé</button>
        </section>
      )}
      {items === null ? <p className="muted">Cargando…</p> : items.length === 0 ? <p>Todavía no hay integraciones.</p> : (
        <ul className="plain-list">
          {items.map((item) => (
            <li key={item.id} className="club-card">
              <h2>{item.nombre} <span className="muted">· {item.estado === 'ACTIVO' ? 'Activa' : 'Inactiva'}</span></h2>
              <p className="muted">ID {item.id} · {item.vinculaciones} cuentas vinculadas · alta {new Date(item.creada_en).toLocaleDateString('es-AR')}</p>
              <p>
                Permisos: {[item.permite_puntos && 'acreditar', item.permite_consulta && 'consultar', item.permite_registro && 'registro externo'].filter(Boolean).join(', ') || 'ninguno'}
                {item.requiere_evento ? ' · exige eventId' : ''}
                <br />
                Límites: {item.max_puntos_operacion} puntos por operación · {item.max_puntos_dia ?? 'sin límite'} por día · {item.max_solicitudes_minuto} solicitudes por minuto
              </p>
              {editing === item.id ? (
                <IntegrationForm initial={item} busy={busy} submitLabel="Guardar" showActive onSubmit={(c) => void update(item.id, c)} onCancel={() => setEditing(null)} />
              ) : (
                <p>
                  <button type="button" disabled={busy} onClick={() => setEditing(item.id)}>Editar</button>{' '}
                  <button type="button" disabled={busy} onClick={() => void run(async () => { await authRequest(`/api/v1/benefits/partners/${item.id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...configOf(item), activo: !item.activo }) }); }, item.activo ? 'Integración desactivada.' : 'Integración activada.')}>
                    {item.activo ? 'Desactivar' : 'Activar'}
                  </button>{' '}
                  <button type="button" disabled={busy} onClick={() => void rotate(item)}>Rotar API key</button>
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
      <section className="club-card" aria-label="Nueva integración">
        <h2>Nueva integración</h2>
        <IntegrationForm initial={NEW_CONFIG} busy={busy} submitLabel="Crear y emitir API key" onSubmit={(c) => void create(c)} />
      </section>
    </main>
  );
}

function configOf(item: Integration): ConfigForm {
  const { id: _id, estado: _estado, vinculaciones: _v, creada_en: _c, ...config } = item;
  return config;
}

interface IntegrationFormProps {
  initial: ConfigForm;
  busy: boolean;
  submitLabel: string;
  showActive?: boolean;
  onSubmit: (config: ConfigForm) => void;
  onCancel?: () => void;
}

function IntegrationForm({ initial, busy, submitLabel, showActive, onSubmit, onCancel }: IntegrationFormProps) {
  const [form, setForm] = useState<ConfigForm>(configOfForm(initial));
  const flag = (key: 'activo' | 'permite_consulta' | 'permite_puntos' | 'permite_registro' | 'requiere_evento', label: string) => (
    <label className="check"><input type="checkbox" checked={form[key]} onChange={(e) => setForm({ ...form, [key]: e.target.checked })} /> {label}</label>
  );
  return (
    <form className="inline-form" onSubmit={(e: FormEvent) => { e.preventDefault(); onSubmit(form); if (!showActive) setForm(configOfForm(initial)); }}>
      <label>Nombre<input required minLength={3} maxLength={100} value={form.nombre} onChange={(e) => setForm({ ...form, nombre: e.target.value })} /></label>
      {showActive && flag('activo', 'Activa')}
      {flag('permite_puntos', 'Acreditar puntos')}
      {flag('permite_consulta', 'Consultar saldo y movimientos')}
      {flag('permite_registro', 'Registro externo')}
      {flag('requiere_evento', 'Exigir eventId')}
      <label>Máximo por operación<input type="number" required min={1} max={1000000} value={form.max_puntos_operacion} onChange={(e) => setForm({ ...form, max_puntos_operacion: Number(e.target.value) })} /></label>
      <label>Máximo por día (vacío = sin límite)<input type="number" min={1} value={form.max_puntos_dia ?? ''} onChange={(e) => setForm({ ...form, max_puntos_dia: e.target.value === '' ? null : Number(e.target.value) })} /></label>
      <label>Solicitudes por minuto<input type="number" required min={1} max={60000} value={form.max_solicitudes_minuto} onChange={(e) => setForm({ ...form, max_solicitudes_minuto: Number(e.target.value) })} /></label>
      <button type="submit" disabled={busy}>{submitLabel}</button>
      {onCancel && <button type="button" onClick={onCancel}>Cancelar</button>}
    </form>
  );
}

function configOfForm(initial: ConfigForm): ConfigForm {
  return { ...initial };
}

/** Cuenta de puntos de una persona dentro de su ficha (HU-CB-07, HU-11, HU-35). */
export function ClubAccountSection({ userId }: { userId: string }) {
  const { authRequest } = useSession();
  const [account, setAccount] = useState<AdminAccount | null>(null);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [missing, setMissing] = useState(false);
  const [page, setPage] = useState(1);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [res, partners] = await Promise.all([
        authRequest<AdminAccount>(`/api/v1/benefits/accounts/${userId}?pagina=${page}&tamano=20`),
        authRequest<{ items: Integration[] }>('/api/v1/benefits/partners'),
      ]);
      setAccount(res.data); setIntegrations(partners.data.items); setMissing(false);
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) { setMissing(true); setAccount(null); }
      else setMessage(errorText(error, 'No se pudo cargar la cuenta de puntos.'));
    }
  }, [authRequest, userId, page]);

  useEffect(() => { void load(); }, [load]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true); setMessage('');
    try { await action(); setMessage(done); await load(); }
    catch (error) { setMessage(errorText(error, 'No se pudo completar la operación.')); }
    finally { setBusy(false); }
  }

  const revert = (movement: AdminMovement, motivo: string) => run(() => authRequest(`/api/v1/benefits/movements/${movement.id}/revert`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': newIdempotencyKey() },
    body: JSON.stringify({ motivo }),
  }), 'Movimiento revertido.');

  const link = (partnerId: string, external: string) => run(() => authRequest(`/api/v1/benefits/partners/${partnerId}/links`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ usuario_id: userId, external_user_id: external }),
  }), 'Cuenta vinculada.');

  return (
    <section className="club-card" aria-label="Club de Beneficios" aria-busy={busy}>
      <h2>Club de Beneficios</h2>
      {message && <p role="status" aria-live="polite">{message}</p>}
      {missing ? <p>La persona todavía no tiene cuenta de puntos. Se crea con su primera asistencia o con un registro externo.</p> : account === null ? <p className="muted">Cargando…</p> : (
        <>
          <p>Saldo: <strong>{account.saldo}</strong> puntos · cuenta creada el {new Date(account.creada_en).toLocaleDateString('es-AR')}</p>
          <h3>Vinculaciones</h3>
          {account.vinculaciones.length === 0 ? <p>Sin vinculaciones con integraciones.</p> : (
            <ul>{account.vinculaciones.map((v) => <li key={v.partner_id}>{v.partner}: <code>{v.external_user_id}</code></li>)}</ul>
          )}
          <LinkForm integrations={integrations} busy={busy} onLink={(p, e) => void link(p, e)} />
          <h3>Movimientos</h3>
          {account.items.length === 0 ? <p>Sin movimientos.</p> : (
            <ol>
              {account.items.map((m) => (
                <li key={m.id}>
                  <strong>{m.puntos > 0 ? '+' : ''}{m.puntos} puntos</strong> · {m.motivo} · {ORIGIN_LABELS[m.origen]}
                  {m.estado === 'REVERTIDO' && <strong> · REVERTIDO</strong>}
                  <br />
                  <span className="muted">{new Date(m.fecha).toLocaleString('es-AR', DATE_TIME)} · Movimiento {m.secuencia}{m.revierte_id ? ' · compensa otro movimiento' : ''}</span>
                  {m.estado === 'CONFIRMADO' && m.origen !== 'REVERSION' && m.puntos !== 0 && (
                    <RevertAction busy={busy} onConfirm={(motivo) => void revert(m, motivo)} />
                  )}
                </li>
              ))}
            </ol>
          )}
          {account.total > account.tamano ? (
            <nav className="pagination" aria-label="Páginas de movimientos">
              <button type="button" disabled={busy || page === 1} onClick={() => setPage((p) => p - 1)}>Anterior</button>
              <span>Página {page} de {Math.max(1, Math.ceil(account.total / account.tamano))} · {account.total} movimientos</span>
              <button type="button" disabled={busy || page * account.tamano >= account.total} onClick={() => setPage((p) => p + 1)}>Siguiente</button>
            </nav>
          ) : <p className="muted">{account.total} {account.total === 1 ? 'movimiento' : 'movimientos'}</p>}
          <p className="muted">Revertir crea un movimiento compensatorio; el original y la asistencia no se modifican (RN-22, RN-25).</p>
        </>
      )}
    </section>
  );
}

function RevertAction({ busy, onConfirm }: { busy: boolean; onConfirm: (reason: string) => void }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  if (!open) return <> <button type="button" className="button-secondary" disabled={busy} onClick={() => setOpen(true)}>Revertir</button></>;
  return (
    <form className="inline-form" onSubmit={(e) => { e.preventDefault(); setOpen(false); onConfirm(reason.trim()); setReason(''); }}>
      <label>Motivo<input required minLength={3} maxLength={200} value={reason} onChange={(e) => setReason(e.target.value)} autoFocus /></label>
      <button type="submit" className="button-danger" disabled={busy}>Confirmar reversión</button>
      <button type="button" className="button-secondary" onClick={() => setOpen(false)}>Volver</button>
    </form>
  );
}

function LinkForm({ integrations, busy, onLink }: { integrations: Integration[]; busy: boolean; onLink: (partnerId: string, external: string) => void }) {
  const [partner, setPartner] = useState('');
  const [external, setExternal] = useState('');
  if (integrations.length === 0) return null;
  return (
    <form className="inline-form" onSubmit={(e) => { e.preventDefault(); onLink(partner, external.trim()); setExternal(''); }}>
      <label>
        Integración
        <select required value={partner} onChange={(e) => setPartner(e.target.value)}>
          <option value="">Elegí una integración</option>
          {integrations.map((i) => <option key={i.id} value={i.id}>{i.nombre}</option>)}
        </select>
      </label>
      <label>Identificador externo<input required maxLength={64} pattern="[A-Za-z0-9._:@\-]{1,64}" value={external} onChange={(e) => setExternal(e.target.value)} /></label>
      <button type="submit" disabled={busy}>Vincular</button>
    </form>
  );
}
