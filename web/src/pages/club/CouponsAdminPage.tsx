import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { useConfirm } from '../../components/ConfirmDialog';
import { EmptyState, Notice, NoticeState, PageHeading, SectionHeading } from '../../components/PageParts';
import { DATE_TIME } from '../../format';
import { calendarDay, Coupon, COUPON_STATES, CouponPage, CouponState, points, Reward, rewardError } from './types';

// Cuponera para el administrador (D-94): validar cupones en recepción (USADO),
// anularlos devolviendo los puntos (RN-25) y gestionar el catálogo (HU-09).

type RewardForm = { nombre: string; descripcion: string; costo: number; vigente_desde: string; vigente_hasta: string; activo: boolean };

function today(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Argentina/Buenos_Aires' }).format(new Date());
}

const EMPTY: RewardForm = { nombre: '', descripcion: '', costo: 500, vigente_desde: today(), vigente_hasta: '', activo: true };

export function CouponsAdminPage() {
  const { authRequest } = useSession();
  const [dialog, confirm] = useConfirm();
  const [rewards, setRewards] = useState<Reward[] | null>(null);
  const [recent, setRecent] = useState<CouponPage | null>(null);
  const [filter, setFilter] = useState<CouponState | ''>('');
  const [code, setCode] = useState('');
  const [found, setFound] = useState<Coupon[] | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [catalog, coupons] = await Promise.all([
        authRequest<{ items: Reward[] }>('/api/v1/benefits/rewards'),
        authRequest<CouponPage>(`/api/v1/benefits/redemptions?pagina=1&tamano=20${filter ? `&estado=${filter}` : ''}`),
      ]);
      setRewards(catalog.data.items);
      setRecent(coupons.data);
    } catch (error) {
      setNotice({ tone: 'error', text: rewardError(error, 'No se pudo cargar la cuponera.') });
    }
  }, [authRequest, filter]);

  useEffect(() => { void load(); }, [load]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true); setNotice(null);
    try {
      await action();
      setNotice({ tone: 'success', text: done });
      await load();
      if (found) await search();
    } catch (error) {
      setNotice({ tone: 'error', text: rewardError(error, 'No se pudo completar la operación.') });
    } finally {
      setBusy(false);
    }
  }

  async function search(event?: FormEvent) {
    event?.preventDefault();
    const value = code.trim();
    if (!value) return;
    try {
      const res = await authRequest<CouponPage>(`/api/v1/benefits/redemptions?codigo=${encodeURIComponent(value)}`);
      setFound(res.data.items);
    } catch (error) {
      setFound(null);
      setNotice({ tone: 'error', text: rewardError(error, 'El código no tiene el formato XXXX-XXXX.') });
    }
  }

  const use = async (coupon: Coupon) => {
    const ok = await confirm({ title: `Entregar ${coupon.beneficio}`, message: `El cupón ${coupon.codigo} queda marcado como usado y ya no se puede anular.`, confirmLabel: 'Marcar como usado' });
    if (ok !== null) void run(() => authRequest(`/api/v1/benefits/redemptions/${coupon.id}/use`, { method: 'POST' }), `Cupón ${coupon.codigo} marcado como usado.`);
  };

  const cancel = async (coupon: Coupon) => {
    const motivo = await confirm({
      title: `Anular el cupón ${coupon.codigo}`,
      message: `Se devuelven ${points(coupon.puntos)} puntos a la cuenta del alumno.`,
      confirmLabel: 'Anular y devolver puntos', danger: true,
      input: { label: 'Motivo', minLength: 3 },
    });
    if (motivo !== null) {
      void run(() => authRequest(`/api/v1/benefits/redemptions/${coupon.id}/cancel`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ motivo }),
      }), `Cupón anulado: se devolvieron ${points(coupon.puntos)} puntos.`);
    }
  };

  const save = (form: RewardForm, id?: string) => run(async () => {
    const body = { ...form, nombre: form.nombre.trim(), descripcion: form.descripcion.trim(), vigente_hasta: form.vigente_hasta || null };
    await authRequest(id ? `/api/v1/benefits/rewards/${id}` : '/api/v1/benefits/rewards', {
      method: id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    setEditing(null);
  }, id ? 'Beneficio actualizado. Los canjes anteriores conservan sus puntos.' : 'Beneficio publicado en el catálogo.');

  const couponRow = (coupon: Coupon) => (
    <li key={coupon.id} className={`coupon-ticket coupon-${coupon.estado.toLowerCase()}`}>
      <div>
        <span className={`status-label ${COUPON_STATES[coupon.estado].tone}`}>{COUPON_STATES[coupon.estado].label}</span>
        <strong>{coupon.beneficio}</strong>
        <small>
          {points(coupon.puntos)} pts · {new Date(coupon.creado_en).toLocaleString('es-AR', DATE_TIME)}
          {coupon.usuario_id && <> · <a href={`#/admin/personas/${coupon.usuario_id}`}>ver alumno</a></>}
          {coupon.motivo_anulacion && ` · anulado: ${coupon.motivo_anulacion}`}
        </small>
      </div>
      <code className="coupon-code">{coupon.codigo}</code>
      {coupon.estado === 'CONFIRMADO' && (
        <div className="coupon-actions">
          <button type="button" className="button-primary" disabled={busy} onClick={() => void use(coupon)}>Marcar como usado</button>
          <button type="button" className="button-secondary" disabled={busy} onClick={() => void cancel(coupon)}>Anular</button>
        </div>
      )}
    </li>
  );

  return (
    <main className="page admin-page" aria-busy={busy}>
      {dialog}
      <PageHeading eyebrow="Club de beneficios" title="Cuponera" index="06"
        description="Validá los cupones que presentan los alumnos, gestioná el catálogo y anulá canjes devolviendo los puntos." />
      <Notice notice={notice} />

      <section className="club-card wide" aria-label="Validar cupón">
        <SectionHeading index="01" kicker="Recepción" title="Validar un cupón" />
        <form className="inline-form" onSubmit={(e) => void search(e)}>
          <label>Código del cupón<input required value={code} placeholder="ABCD-EF23" maxLength={9} autoComplete="off" onChange={(e) => setCode(e.target.value.toUpperCase())} /></label>
          <button type="submit" disabled={busy}>Buscar</button>
        </form>
        {found && (found.length === 0 ? <p>No existe un cupón con ese código.</p> : <ol className="coupon-list">{found.map(couponRow)}</ol>)}
      </section>

      <SectionHeading index="02" kicker="Catálogo" title="Beneficios" />
      <p className="muted">Sin stock: un beneficio activo y vigente se puede canjear sin límite. Cambiar el costo no modifica los canjes ya hechos.</p>
      {rewards === null ? <p className="muted">Cargando…</p> : rewards.length === 0 ? <EmptyState title="Catálogo vacío">Publicá el primer beneficio con el formulario de abajo.</EmptyState> : (
        <ul className="plain-list">
          {rewards.map((reward) => (
            <li key={reward.id} className="club-card wide">
              <h3>{reward.nombre} <span className="muted">· {points(reward.costo)} pts · {reward.activo ? 'Activo' : 'Inactivo'}</span></h3>
              {reward.descripcion && <p>{reward.descripcion}</p>}
              <p className="muted">Vigente desde {calendarDay(reward.vigente_desde)}{reward.vigente_hasta ? ` hasta ${calendarDay(reward.vigente_hasta)}` : ', sin vencimiento'} · {reward.canjes ?? 0} canjes</p>
              {editing === reward.id
                ? <RewardEditor initial={{ nombre: reward.nombre, descripcion: reward.descripcion, costo: reward.costo, vigente_desde: reward.vigente_desde, vigente_hasta: reward.vigente_hasta ?? '', activo: reward.activo ?? true }} busy={busy} submitLabel="Guardar" showActive onSubmit={(f) => void save(f, reward.id)} onCancel={() => setEditing(null)} />
                : <p><button type="button" disabled={busy} onClick={() => setEditing(reward.id)}>Editar</button></p>}
            </li>
          ))}
        </ul>
      )}
      <section className="club-card wide" aria-label="Nuevo beneficio">
        <h3>Nuevo beneficio</h3>
        <RewardEditor initial={EMPTY} busy={busy} submitLabel="Publicar beneficio" onSubmit={(f) => void save(f)} />
      </section>

      <SectionHeading index="03" kicker="Historial" title="Últimos canjes" />
      <label className="coupon-filter">Estado
        <select value={filter} onChange={(e) => setFilter(e.target.value as CouponState | '')}>
          <option value="">Todos</option>
          {(Object.keys(COUPON_STATES) as CouponState[]).map((s) => <option key={s} value={s}>{COUPON_STATES[s].label}</option>)}
        </select>
      </label>
      {recent === null ? <p className="muted">Cargando…</p> : recent.items.length === 0 ? <p>No hay canjes{filter ? ' en ese estado' : ''}.</p> : <ol className="coupon-list">{recent.items.map(couponRow)}</ol>}
    </main>
  );
}

function RewardEditor({ initial, busy, submitLabel, showActive, onSubmit, onCancel }: {
  initial: RewardForm; busy: boolean; submitLabel: string; showActive?: boolean;
  onSubmit: (form: RewardForm) => void; onCancel?: () => void;
}) {
  const [form, setForm] = useState<RewardForm>(initial);
  return (
    <form className="inline-form" onSubmit={(e) => { e.preventDefault(); onSubmit(form); if (!showActive) setForm(initial); }}>
      <label>Nombre<input required minLength={3} maxLength={100} value={form.nombre} onChange={(e) => setForm({ ...form, nombre: e.target.value })} /></label>
      <label>Descripción<textarea maxLength={500} rows={2} value={form.descripcion} onChange={(e) => setForm({ ...form, descripcion: e.target.value })} /></label>
      <label>Costo en puntos<input type="number" required min={1} max={1000000} value={form.costo} onChange={(e) => setForm({ ...form, costo: Number(e.target.value) })} /></label>
      <label>Vigente desde<input type="date" required value={form.vigente_desde} onChange={(e) => setForm({ ...form, vigente_desde: e.target.value })} /></label>
      <label>Vigente hasta (opcional)<input type="date" min={form.vigente_desde} value={form.vigente_hasta} onChange={(e) => setForm({ ...form, vigente_hasta: e.target.value })} /></label>
      {showActive && <label className="check"><input type="checkbox" checked={form.activo} onChange={(e) => setForm({ ...form, activo: e.target.checked })} /> Activo (visible en el catálogo)</label>}
      <button type="submit" disabled={busy}>{submitLabel}</button>
      {onCancel && <button type="button" onClick={onCancel}>Cancelar</button>}
    </form>
  );
}
