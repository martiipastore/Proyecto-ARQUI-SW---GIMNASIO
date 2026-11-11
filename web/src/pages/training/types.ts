// Tipos de training.v1 (etapa 10): planes, mediciones y consultas.
import { ApiError } from '../../api/client';

export interface Exercise {
  nombre: string; grupo_muscular: string; series: number; repeticiones: number;
  peso_sugerido: number; descanso_segundos: number; observaciones: string;
}
export interface RoutineDay { nombre: string; descripcion: string; ejercicios: Exercise[] }
export interface TrainingPlan {
  id: string; alumno_id: string; objetivo: string; vigencia_desde: string; vigencia_hasta: string;
  observaciones: string; dias: RoutineDay[]; estado: 'VIGENTE' | 'FINALIZADO'; creado_en: string;
}
export interface NutritionPlan {
  id: string; alumno_id: string; fecha: string; desayuno: string; colacion: string; almuerzo: string;
  merienda: string; cena: string; observaciones: string; estado: 'VIGENTE' | 'FINALIZADO'; creado_en: string;
}
export interface Measurement {
  id: string; fecha: string; peso: number; porcentaje_grasa: number; porcentaje_muscular: number;
  masa_muscular: number | null; observaciones: string; registrada_en: string;
}
export interface Question {
  id: string; alumno_id: string; pregunta: string; enviada_en: string; estado: 'PENDIENTE' | 'RESPONDIDA';
  respuesta: string | null; respondida_en: string | null; nutricionista_id: string | null;
}
export interface Plans<T> { vigente: T | null; historial: T[] }

export const MEALS: [keyof NutritionPlan, string][] = [
  ['desayuno', 'Desayuno'], ['colacion', 'Colación'], ['almuerzo', 'Almuerzo'], ['merienda', 'Merienda'], ['cena', 'Cena'],
];

const TRAINING_ERRORS: Record<string, string> = {
  SIN_PERMISO: 'No tenés asignada a esta persona o tu rol no permite la operación.',
  SIN_NUTRICIONISTA: 'Todavía no tenés un nutricionista asignado.',
  TRANSICION_INVALIDA: 'La consulta ya fue respondida.',
  VALIDACION: 'Revisá los datos: algún campo está vacío o fuera de rango.',
  DEPENDENCIA_NO_DISPONIBLE: 'No se pudo confirmar la asignación. Reintentá en unos segundos.',
};

export function trainingError(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.problem && TRAINING_ERRORS[error.problem.code]) return TRAINING_ERRORS[error.problem.code];
  return error instanceof Error ? error.message : fallback;
}

export function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

export function today(): string {
  return new Date().toLocaleDateString('sv-SE');
}
