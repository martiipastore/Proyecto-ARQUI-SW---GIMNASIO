// Tipos de las respuestas de users.v1 y memberships.v1 usados por las pantallas.

export interface Person {
  id: string;
  rol: string;
  nombre: string;
  apellido: string | null;
  dni: string | null;
  email: string;
  telefono: string | null;
  fecha_nacimiento: string | null;
  matricula: string | null;
  estado: 'ACTIVO' | 'INACTIVO';
  registro_externo: boolean;
  fecha_alta: string;
  dado_de_baja_en: string | null;
  activacion_pendiente: boolean;
}

export interface PersonDetail {
  id: string;
  nombre: string;
  apellido: string | null;
  email: string;
  telefono: string | null;
  rol: string;
  estado: string;
}

export interface Assignment {
  id: string;
  profesional_id: string;
  alumno_id: string;
  desde: string;
  hasta: string | null;
  alumno?: PersonDetail;
  profesional?: PersonDetail;
}

export interface MembershipType {
  codigo: string;
  nombre: string;
  duracion_meses: number | null;
  duracion_dias: number | null;
  clases: number | null;
  habilitado: boolean;
}

export interface Membership {
  id: string;
  tipo: MembershipType;
  fecha_inicio: string;
  fecha_vencimiento: string;
  estado: string;
  condicion: string;
  motivo: string;
}

export interface MembershipPage {
  activa: boolean;
  membresia_vigente: Membership | null;
  items: Membership[];
  total: number;
}

export const ROLE_LABELS: Record<string, string> = {
  ADMINISTRADOR: 'Administrador',
  PROFESOR: 'Profesor',
  NUTRICIONISTA: 'Nutricionista',
  ALUMNO: 'Alumno',
};

export function fullName(p: { nombre: string; apellido: string | null }): string {
  return p.apellido ? `${p.nombre} ${p.apellido}` : p.nombre;
}

/** Clave nueva por intento lógico (memberships.v1): repetir el envío no duplica. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
