// Contrato de la cuponera interna (benefits.v1, D-94). Lo comparten la pantalla
// del alumno y la del administrador.
import { ApiError } from '../../api/client';

export interface Reward {
  id: string;
  nombre: string;
  descripcion: string;
  costo: number;
  vigente_desde: string;
  vigente_hasta: string | null;
  /** Solo para el administrador. */
  activo?: boolean;
  canjes?: number;
}

export type CouponState = 'CONFIRMADO' | 'USADO' | 'ANULADO';

export interface Coupon {
  id: string;
  beneficio_id: string;
  beneficio: string;
  codigo: string;
  puntos: number;
  estado: CouponState;
  creado_en: string;
  usado_en: string | null;
  anulado_en: string | null;
  motivo_anulacion?: string;
  /** Solo para el administrador. */
  usuario_id?: string;
}

export interface CouponPage { items: Coupon[]; total: number; pagina: number; tamano: number }

export const COUPON_STATES: Record<CouponState, { label: string; tone: string }> = {
  CONFIRMADO: { label: 'Para usar', tone: 'status-active' },
  USADO: { label: 'Usado', tone: 'status-used' },
  ANULADO: { label: 'Anulado', tone: 'status-inactive' },
};

const ERRORS: Record<string, string> = {
  SALDO_INSUFICIENTE: 'No tenés puntos suficientes para este beneficio.',
  BENEFICIO_NO_DISPONIBLE: 'Este beneficio ya no está disponible. Actualizá el catálogo.',
  TRANSICION_INVALIDA: 'El cupón ya fue usado o anulado.',
  CONFLICTO_IDEMPOTENCIA: 'La operación ya se envió con otros datos. Recargá e intentá de nuevo.',
  VALIDACION: 'Revisá los datos: algún campo está fuera de los límites.',
};

export function rewardError(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.problem && ERRORS[error.problem.code]) return ERRORS[error.problem.code];
  return error instanceof Error ? error.message : fallback;
}

/** "2026-12-31" → "31/12/2026", sin pasar por Date para no correr el día por zona horaria. */
export function calendarDay(value: string): string {
  const [year, month, day] = value.split('-');
  return `${day}/${month}/${year}`;
}

export function points(value: number): string {
  return value.toLocaleString('es-AR');
}
