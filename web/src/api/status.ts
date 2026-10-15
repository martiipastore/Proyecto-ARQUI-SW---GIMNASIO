import { request, type ApiResponse } from './client';

/** Estado de un servicio en el resumen (ADR-007). */
export type ServiceState = 'ready' | 'degraded' | 'not_ready' | 'unreachable';

/** Estado global: ready y degraded responden 200; not_ready responde 503. */
export type SystemState = 'ready' | 'degraded' | 'not_ready';

export interface ServiceStatus {
  name: string;
  status: ServiceState;
  latency_ms: number;
  checks: Record<string, string>;
}

export interface SystemStatus {
  status: SystemState;
  checked_at: string;
  services: ServiceStatus[];
}

/** GET /api/v1/status. 503 también trae el resumen (sistema no disponible). */
export function getSystemStatus(): Promise<ApiResponse<SystemStatus>> {
  return request<SystemStatus>('/api/v1/status', { acceptStatuses: [200, 503] });
}
