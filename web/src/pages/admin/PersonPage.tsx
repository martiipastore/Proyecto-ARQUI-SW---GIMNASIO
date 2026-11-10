import { FormEvent, useCallback, useEffect, useState } from 'react';
import { ApiError } from '../../api/client';
import { useSession } from '../../auth/session';
import { ClubAccountSection } from './ClubAdmin';
import { Assignment, fullName, Membership, MembershipPage, MembershipType, newIdempotencyKey, Person, ROLE_LABELS } from './types';

// Detalle de persona (users.v1 y memberships.v1): datos, baja, membresías y
// asignaciones. Es donde el administrador prepara a un alumno (etapa 3).
const MEMBERSHIP_ERRORS: Record<string, string> = {
  MEMBRESIAS_SUPERPUESTAS: 'El período se superpone con otra membresía activa.',
  TRANSICION_MEMBRESIA_INVALIDA: 'La membresía no permite esa operación.',
  ASIGNACION_DUPLICADA: 'Ya existe una asignación vigente con ese profesional.',
  TRANSICION_INVALIDA: 'La persona ya estaba dada de baja.',
};

function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.problem && MEMBERSHIP_ERRORS[error.problem.code]) {
    return MEMBERSHIP_ERRORS[error.problem.code];
  }
  return error instanceof Error ? error.message : fallback;
}

function today(): string {
  // Fecha local en formato AAAA-MM-DD; el backend la valida en hora argentina.
  return new Date().toLocaleDateString('sv-SE');
}

export function PersonPage({ id }: { id: string }) {
  const { authRequest } = useSession();
  const [person, setPerson] = useState<Person | null>(null);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [memberships, setMemberships] = useState<MembershipPage | null>(null);
  const [types, setTypes] = useState<MembershipType[]>([]);
  const [professionals, setProfessionals] = useState<Person[]>([]);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const detail = await authRequest<{ persona: Person; asignaciones: Assignment[] }>(`/api/v1/users/${id}`);
      setPerson(detail.data.persona);
      setAssignments(detail.data.asignaciones);
      if (detail.data.persona.rol === 'ALUMNO') {
        const [page, typeList, teachers, nutritionists] = await Promise.all([
          authRequest<MembershipPage>(`/api/v1/memberships/users/${id}?tamano=20`),
          authRequest<{ items: MembershipType[] }>('/api/v1/memberships/types'),
          authRequest<{ items: Person[] }>('/api/v1/users?rol=PROFESOR&estado=ACTIVO&tamano=100'),
          authRequest<{ items: Person[] }>('/api/v1/users?rol=NUTRICIONISTA&estado=ACTIVO&tamano=100'),
        ]);
        setMemberships(page.data);
        setTypes(typeList.data.items.filter((t) => t.habilitado));
        setProfessionals([...teachers.data.items, ...nutritionists.data.items]);
      }
    } catch (error) {
      setMessage(errorText(error, 'No se pudo cargar la persona.'));
    }
  }, [authRequest, id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    setMessage('');
    try {
      await action();
      setMessage(done);
      await load();
    } catch (error) {
      setMessage(errorText(error, 'No se pudo completar la operación.'));
    } finally {
      setBusy(false);
    }
  }

  const post = (path: string, body: object, idempotent = false) =>
    authRequest(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(idempotent ? { 'Idempotency-Key': newIdempotencyKey() } : {}) },
      body: JSON.stringify(body),
    });

  if (!person) {
    return <main className="page">{message ? <p role="alert">{message}</p> : <p className="muted">Cargando…</p>}</main>;
  }
  const active = person.estado === 'ACTIVO';
  return (
    <main className="page">
      <p><a href="#/admin/personas">← Personas</a></p>
      <h1>{fullName(person)}</h1>
      {message && <p role="status" aria-live="polite">{message}</p>}
      <PersonData person={person} busy={busy} onSave={(body) => run(() => authRequest(`/api/v1/users/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }), 'Datos actualizados.')} />
      {active && person.dni && person.rol !== 'ADMINISTRADOR' && (
        <ReasonAction label="Dar de baja" busy={busy} onConfirm={(motivo) => run(() => post(`/api/v1/users/${id}/deactivate`, { motivo }), 'La persona quedó dada de baja.')} />
      )}

      {person.rol === 'ALUMNO' && (
        <Memberships
          page={memberships}
          types={types}
          busy={busy || !active}
          onAssign={(body) => run(() => post('/api/v1/memberships', { usuario_id: id, ...body }, true), 'Membresía asignada.')}
          onRenew={(body) => run(() => post(`/api/v1/memberships/users/${id}/renew`, body, true), 'Membresía renovada.')}
          onCancel={(membershipId, motivo) => run(() => post(`/api/v1/memberships/${membershipId}/cancel`, { motivo }, true), 'Membresía cancelada.')}
        />
      )}

      <section className="club-card" aria-label="Asignaciones">
        <h2>{person.rol === 'ALUMNO' ? 'Profesionales asignados' : 'Alumnos asignados'}</h2>
        {assignments.length === 0 ? <p>Sin asignaciones.</p> : (
          <ul>
            {assignments.map((a) => {
              const other = person.rol === 'ALUMNO' ? a.profesional : a.alumno;
              return (
                <li key={a.id}>
                  {other ? `${fullName(other)} (${ROLE_LABELS[other.rol] ?? other.rol})` : '—'} · desde {a.desde.slice(0, 10)}
                  {a.hasta ? ` · revocada el ${a.hasta.slice(0, 10)}` : (
                    <button type="button" disabled={busy} onClick={() => run(() => post(`/api/v1/users/${a.profesional_id}/assignments/${a.alumno_id}/revoke`, {}), 'Asignación revocada.')}>Revocar</button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
        {person.rol === 'ALUMNO' && active && (
          <AssignForm professionals={professionals} busy={busy} onAssign={(professionalId) => run(() => post(`/api/v1/users/${professionalId}/assignments`, { alumno_id: id }), 'Asignación registrada.')} />
        )}
      </section>

      {/* Etapa 7: el Club es para todas las personas (D-63/D-64), no solo alumnos. */}
      <ClubAccountSection userId={id} />
    </main>
  );
}

function PersonData({ person, busy, onSave }: { person: Person; busy: boolean; onSave: (body: Record<string, string>) => void }) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ nombre: person.nombre, apellido: person.apellido ?? '', telefono: person.telefono ?? '', fecha_nacimiento: person.fecha_nacimiento ?? '', matricula: person.matricula ?? '' });
  const canEdit = person.dni !== null && person.rol !== 'ADMINISTRADOR' && person.estado === 'ACTIVO';
  if (editing) {
    return (
      <form className="club-card" onSubmit={(e: FormEvent) => { e.preventDefault(); setEditing(false); onSave(form); }}>
        <label>Nombre<input required maxLength={80} value={form.nombre} onChange={(e) => setForm({ ...form, nombre: e.target.value })} /></label>
        <label>Apellido<input required maxLength={80} value={form.apellido} onChange={(e) => setForm({ ...form, apellido: e.target.value })} /></label>
        <label>Teléfono<input maxLength={30} value={form.telefono} onChange={(e) => setForm({ ...form, telefono: e.target.value })} /></label>
        <label>Fecha de nacimiento<input type="date" min="1900-01-01" required={person.rol === 'ALUMNO'} value={form.fecha_nacimiento} onChange={(e) => setForm({ ...form, fecha_nacimiento: e.target.value })} /></label>
        {person.rol === 'NUTRICIONISTA' && <label>Matrícula<input maxLength={40} value={form.matricula} onChange={(e) => setForm({ ...form, matricula: e.target.value })} /></label>}
        <button type="submit" disabled={busy}>Guardar</button>
        <button type="button" onClick={() => setEditing(false)}>Cancelar</button>
      </form>
    );
  }
  return (
    <dl className="club-card">
      <dt>Rol</dt><dd>{ROLE_LABELS[person.rol] ?? person.rol}</dd>
      <dt>DNI</dt><dd>{person.dni ?? 'Sin DNI (registro externo del Club)'}</dd>
      <dt>Email</dt><dd>{person.email}</dd>
      <dt>Teléfono</dt><dd>{person.telefono ?? '—'}</dd>
      {person.rol === 'ALUMNO' && <><dt>Fecha de nacimiento</dt><dd>{person.fecha_nacimiento ?? '—'}</dd></>}
      {person.rol === 'NUTRICIONISTA' && <><dt>Matrícula</dt><dd>{person.matricula ?? '—'}</dd></>}
      <dt>Estado</dt>
      <dd>{person.estado === 'ACTIVO' ? (person.activacion_pendiente ? 'Activo; todavía no eligió su contraseña' : 'Activo') : `Dado de baja el ${person.dado_de_baja_en?.slice(0, 10) ?? ''}`}</dd>
      {canEdit && <dd><button type="button" onClick={() => setEditing(true)}>Editar datos</button></dd>}
    </dl>
  );
}

function ReasonAction({ label, busy, onConfirm }: { label: string; busy: boolean; onConfirm: (reason: string) => void }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  if (!open) {
    return <p><button type="button" disabled={busy} onClick={() => setOpen(true)}>{label}</button></p>;
  }
  return (
    <form className="inline-form" onSubmit={(e) => { e.preventDefault(); setOpen(false); onConfirm(reason.trim()); setReason(''); }}>
      <label>Motivo<input required minLength={3} maxLength={200} value={reason} onChange={(e) => setReason(e.target.value)} /></label>
      <button type="submit" disabled={busy}>Confirmar</button>
      <button type="button" onClick={() => setOpen(false)}>Cancelar</button>
    </form>
  );
}

interface MembershipsProps {
  page: MembershipPage | null;
  types: MembershipType[];
  busy: boolean;
  onAssign: (body: Record<string, string>) => void;
  onRenew: (body: Record<string, string>) => void;
  onCancel: (membershipId: string, reason: string) => void;
}

function Memberships({ page, types, busy, onAssign, onRenew, onCancel }: MembershipsProps) {
  const [type, setType] = useState('MENSUAL');
  const [start, setStart] = useState(today());
  const [end, setEnd] = useState('');
  const [reason, setReason] = useState('');
  const selected = types.find((t) => t.codigo === type);
  // Solo un tipo sin duración configurada necesita vencimiento explícito (D-77).
  const needsEnd = selected !== undefined && selected.duracion_meses === null && selected.duracion_dias === null;
  const hasHistory = page !== null && page.items.length > 0;

  function submit(renew: boolean) {
    const body: Record<string, string> = { tipo_codigo: type, motivo: reason.trim() };
    if (needsEnd) body.fecha_vencimiento = end;
    if (renew) onRenew(body);
    else onAssign({ ...body, fecha_inicio: start });
    setReason('');
  }

  return (
    <section className="club-card" aria-label="Membresías">
      <h2>Membresías</h2>
      {page === null ? <p className="muted">Cargando…</p> : (
        <>
          <p>{page.activa && page.membresia_vigente
            ? `Vigente: ${page.membresia_vigente.tipo.nombre} hasta el ${page.membresia_vigente.fecha_vencimiento}.`
            : 'Sin membresía vigente.'}</p>
          {hasHistory && (
            <ul>
              {page.items.map((m: Membership) => (
                <li key={m.id}>
                  {m.tipo.nombre}: {m.fecha_inicio} al {m.fecha_vencimiento} — {m.condicion.toLocaleLowerCase('es-AR')}
                  {m.estado === 'ACTIVA' && m.condicion !== 'VENCIDA' && (
                    <ReasonAction label="Cancelar" busy={busy} onConfirm={(motivo) => onCancel(m.id, motivo)} />
                  )}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      <form className="inline-form" onSubmit={(e) => { e.preventDefault(); submit(false); }}>
        <label>
          Tipo
          <select value={type} onChange={(e) => setType(e.target.value)}>
            {types.map((t) => <option key={t.codigo} value={t.codigo}>{t.nombre}</option>)}
          </select>
        </label>
        <label>Inicio<input type="date" required min={today()} value={start} onChange={(e) => setStart(e.target.value)} /></label>
        {needsEnd && <label>Vencimiento<input type="date" required min={start} value={end} onChange={(e) => setEnd(e.target.value)} /></label>}
        <label>Motivo<input required minLength={3} maxLength={200} value={reason} onChange={(e) => setReason(e.target.value)} /></label>
        <button type="submit" disabled={busy}>Asignar</button>
        {hasHistory && <button type="button" disabled={busy || reason.trim().length < 3} onClick={() => submit(true)}>Renovar</button>}
      </form>
      <p className="muted">El vencimiento se calcula según el tipo (D-77). Renovar empieza al día siguiente de la última membresía activa.</p>
    </section>
  );
}

function AssignForm({ professionals, busy, onAssign }: { professionals: Person[]; busy: boolean; onAssign: (professionalId: string) => void }) {
  const [selected, setSelected] = useState('');
  if (professionals.length === 0) {
    return <p className="muted">No hay profesionales activos para asignar.</p>;
  }
  return (
    <form className="inline-form" onSubmit={(e) => { e.preventDefault(); if (selected) onAssign(selected); }}>
      <label>
        Asignar a
        <select required value={selected} onChange={(e) => setSelected(e.target.value)}>
          <option value="">Elegí un profesional</option>
          {professionals.map((p) => <option key={p.id} value={p.id}>{fullName(p)} ({ROLE_LABELS[p.rol] ?? p.rol})</option>)}
        </select>
      </label>
      <button type="submit" disabled={busy}>Asignar</button>
    </form>
  );
}
