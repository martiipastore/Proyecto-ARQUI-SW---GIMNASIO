import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError } from '../../api/client';
import { useSession } from '../../auth/session';
import { AnimatedCounter } from '../../components/AnimatedCounter';
import { useConfirm } from '../../components/ConfirmDialog';
import { EmptyState, Notice, NoticeState, PageHeading } from '../../components/PageParts';
import { DATE_TIME } from '../../format';
import { newIdempotencyKey } from '../admin/types';
import { calendarDay, Coupon, COUPON_STATES, CouponPage, points, Reward, rewardError } from './types';

// Cuponera del alumno (D-94): catálogo canjeable hoy (HU-09, RN-24), canje con
// Idempotency-Key (HU-20, CA-20.4) y cupones propios con su código.
export function RewardsPage() {
  const { authRequest } = useSession();
  const [dialog, confirm] = useConfirm();
  const [balance, setBalance] = useState<number | null>(null);
  const [rewards, setRewards] = useState<Reward[] | null>(null);
  const [coupons, setCoupons] = useState<CouponPage | null>(null);
  const [page, setPage] = useState(1);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [fresh, setFresh] = useState<Coupon | null>(null);
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);

  const load = useCallback(async () => {
    const current = ++generation.current;
    const [account, catalog, mine] = await Promise.allSettled([
      authRequest<{ balance: number }>('/api/v1/benefits/me'),
      authRequest<{ items: Reward[] }>('/api/v1/benefits/catalog'),
      authRequest<CouponPage>(`/api/v1/benefits/me/redemptions?pagina=${page}&tamano=10`),
    ]);
    if (current !== generation.current) return;
    // Sin cuenta de puntos todavía (404): el saldo es 0 hasta la primera asistencia.
    if (account.status === 'fulfilled') setBalance(account.value.data.balance);
    else setBalance(account.reason instanceof ApiError && account.reason.status === 404 ? 0 : null);
    if (catalog.status === 'fulfilled') setRewards(catalog.value.data.items);
    if (mine.status === 'fulfilled') setCoupons(mine.value.data);
    const failed = [catalog, mine].find((r) => r.status === 'rejected') as PromiseRejectedResult | undefined;
    if (failed) setNotice({ tone: 'error', text: rewardError(failed.reason, 'No se pudo cargar la cuponera.') });
  }, [authRequest, page]);

  useEffect(() => {
    void load();
    return () => { generation.current++; };
  }, [load]);

  async function redeem(reward: Reward) {
    const ok = await confirm({
      title: `Canjear ${reward.nombre}`,
      message: `Se descuentan ${points(reward.costo)} puntos de tu saldo y recibís un cupón con un código para mostrar en recepción.`,
      confirmLabel: 'Canjear',
    });
    if (ok === null) return;
    setBusy(true); setNotice(null); setFresh(null);
    try {
      const res = await authRequest<{ canje: Coupon; saldo: number }>('/api/v1/benefits/redemptions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': newIdempotencyKey() },
        body: JSON.stringify({ beneficio_id: reward.id }),
      });
      setFresh(res.data.canje);
      setBalance(res.data.saldo);
      setNotice({ tone: 'success', text: `¡Listo! Canjeaste ${reward.nombre}. Mostrá el código ${res.data.canje.codigo} en recepción.` });
      if (page !== 1) setPage(1); else await load();
    } catch (error) {
      setNotice({ tone: 'error', text: rewardError(error, 'No se pudo completar el canje.') });
      await load();
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="page admin-page rewards-page" aria-busy={busy}>
      {dialog}
      <PageHeading eyebrow="Club de beneficios" title="Cuponera" index="05"
        description="Cambiá tus puntos por beneficios del gimnasio. Cada canje te da un cupón con un código para presentar en recepción." />

      <section className="rewards-balance" aria-label="Tu saldo">
        <span>Saldo disponible</span>
        {balance === null ? <div className="points-loader" aria-label="Cargando puntos" />
          : <strong><AnimatedCounter value={balance} /><small>pts</small></strong>}
        <p>Sumás 500 puntos por cada clase a la que asistís.</p>
      </section>

      <Notice notice={notice} />

      {fresh && (
        <section className="coupon-ticket coupon-fresh" aria-label="Cupón recién canjeado">
          <div>
            <span>Tu cupón</span>
            <strong>{fresh.beneficio}</strong>
            <small>{points(fresh.puntos)} pts · mostralo en recepción</small>
          </div>
          <code className="coupon-code">{fresh.codigo}</code>
        </section>
      )}

      <h2 className="admin-content-title">Catálogo</h2>
      {rewards === null ? <div className="dashboard-skeleton" aria-label="Cargando catálogo" />
        : rewards.length === 0 ? <EmptyState title="Todavía no hay beneficios">El gimnasio va a publicar beneficios para canjear pronto.</EmptyState>
          : (
            <ul className="reward-grid">
              {rewards.map((reward) => {
                const missing = balance === null ? 0 : reward.costo - balance;
                return (
                  <li key={reward.id} className="reward-card">
                    <header><strong>{points(reward.costo)}</strong><span>pts</span></header>
                    <h3>{reward.nombre}</h3>
                    {reward.descripcion && <p>{reward.descripcion}</p>}
                    <small className="muted">{reward.vigente_hasta ? `Disponible hasta el ${calendarDay(reward.vigente_hasta)}` : 'Sin fecha de vencimiento'}</small>
                    <button type="button" className={missing > 0 ? 'button-secondary' : 'button-primary'} disabled={busy || balance === null || missing > 0} onClick={() => void redeem(reward)}>
                      {missing > 0 ? `Te faltan ${points(missing)} pts` : 'Canjear'}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}

      <h2 className="admin-content-title">Mis cupones</h2>
      {coupons === null ? <div className="dashboard-skeleton" aria-label="Cargando cupones" />
        : coupons.items.length === 0 ? <EmptyState title="Sin cupones">Cuando canjees un beneficio, tu cupón aparece acá.</EmptyState>
          : (
            <>
              <ol className="coupon-list">
                {coupons.items.map((coupon) => (
                  <li key={coupon.id} className={`coupon-ticket coupon-${coupon.estado.toLowerCase()}`}>
                    <div>
                      <span className={`status-label ${COUPON_STATES[coupon.estado].tone}`}>{COUPON_STATES[coupon.estado].label}</span>
                      <strong>{coupon.beneficio}</strong>
                      <small>
                        {points(coupon.puntos)} pts · canjeado el {new Date(coupon.creado_en).toLocaleString('es-AR', DATE_TIME)}
                        {coupon.usado_en && ` · usado el ${new Date(coupon.usado_en).toLocaleString('es-AR', DATE_TIME)}`}
                        {coupon.estado === 'ANULADO' && ' · te devolvimos los puntos'}
                      </small>
                    </div>
                    <code className="coupon-code">{coupon.codigo}</code>
                  </li>
                ))}
              </ol>
              {coupons.total > coupons.tamano && (
                <div className="points-pagination">
                  <button type="button" disabled={busy || page === 1} onClick={() => setPage((p) => p - 1)}>← Anterior</button>
                  <span>Página {page} de {Math.ceil(coupons.total / coupons.tamano)}</span>
                  <button type="button" disabled={busy || page * coupons.tamano >= coupons.total} onClick={() => setPage((p) => p + 1)}>Siguiente →</button>
                </div>
              )}
            </>
          )}
    </main>
  );
}
