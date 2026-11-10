// Tipos de booking.v1 y utilidades de formato compartidas por las pantallas de
// clases, reservas, administración e ingreso (etapa 4 de la entrega final).
import { ApiError } from '../../api/client';

export interface Ref {
  id: string;
  nombre: string;
}

export interface Clase {
  id: string;
  actividad: Ref;
  inicio: string;
  fin: string;
  franja: string | null;
  profesor: Ref;
  capacidad: number;
  ocupacion: number;
  disponibles: number;
  estado: 'PROGRAMADA' | 'EN_CURSO' | 'FINALIZADA' | 'CANCELADA';
  reservable: boolean;
  reservada_por_mi: boolean;
}

export interface Reserva {
  id: string;
  clase: { id: string; actividad: string; inicio: string; fin: string };
  estado: 'CONFIRMADA' | 'CANCELADA' | 'ASISTIDA' | 'AUSENTE';
  creada_en: string;
  ingreso_en: string | null;
  cancelada_en: string | null;
  motivo_cancelacion: string | null;
}

export interface ReservaDeClase {
  id: string;
  alumno_id: string;
  alumno_nombre: string | null;
  estado: Reserva['estado'];
  creada_en: string;
  ingreso_en: string | null;
}

export interface Actividad {
  id: string;
  nombre: string;
  capacidad_maxima: number;
  musculacion: boolean;
  activa: boolean;
  puntos_asistencia: number;
}

export interface Horario {
  id: string;
  actividad_id: string;
  dia_semana: number;
  inicio: string;
  fin: string;
  duracion_minutos: number;
  profesor: Ref;
  capacidad: number;
}

export interface Pagina<T> {
  items: T[];
  total: number;
  pagina: number;
  tamano: number;
}

const ZONE = 'America/Argentina/Buenos_Aires';
const dateFormat = new Intl.DateTimeFormat('es-AR', { timeZone: ZONE, weekday: 'short', day: '2-digit', month: '2-digit' });
const timeFormat = new Intl.DateTimeFormat('es-AR', { timeZone: ZONE, hour: '2-digit', minute: '2-digit', hour12: false });
const isoDate = new Intl.DateTimeFormat('en-CA', { timeZone: ZONE, year: 'numeric', month: '2-digit', day: '2-digit' });

export const DAYS = ['', 'Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado', 'Domingo'];

export const BOOKING_STATES: Record<string, string> = {
  CONFIRMADA: 'Confirmada',
  CANCELADA: 'Cancelada',
  ASISTIDA: 'Asistida',
  AUSENTE: 'Ausente',
};

export const CLASS_STATES: Record<string, string> = {
  PROGRAMADA: 'Programada',
  EN_CURSO: 'En curso',
  FINALIZADA: 'Finalizada',
  CANCELADA: 'Cancelada',
};

/** "mar. 10/11" en hora de Argentina. */
export function day(iso: string): string {
  return dateFormat.format(new Date(iso));
}

/** "19:00" en hora de Argentina. */
export function time(iso: string): string {
  return timeFormat.format(new Date(iso));
}

/** "19:00–20:00". */
export function range(c: { inicio: string; fin: string }): string {
  return `${time(c.inicio)}–${time(c.fin)}`;
}

/** Fecha AAAA-MM-DD de hoy (o dentro de n días) en Argentina. */
export function todayPlus(days = 0): string {
  return isoDate.format(new Date(Date.now() + days * 24 * 60 * 60 * 1000));
}

/** Ocupación como la pide CA-13.1: "21/30 ocupados - 9 disponibles". */
export function occupancy(c: Clase): string {
  return `${c.ocupacion}/${c.capacidad} ocupados - ${c.disponibles} disponibles`;
}

/** Mensaje del problema RFC 7807 o el texto por defecto. */
export function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message;
  return fallback;
}

/** Clave nueva por intento lógico (RN-15): repetir el envío no duplica. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
