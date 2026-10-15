// Cliente HTTP del frontend. Habla SOLO con el api-gateway (VITE_API_URL).

export const API_URL = (import.meta.env.VITE_API_URL ?? 'http://localhost:8080').replace(/\/+$/, '');

const CORRELATION_HEADER = 'X-Correlation-ID';

/** Error RFC 7807 devuelto por el backend. */
export interface Problem {
  type: string;
  title: string;
  status: number;
  code: string;
  detail?: string;
  instance?: string;
  correlation_id?: string;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly correlationId: string | null,
    readonly problem?: Problem,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface ApiResponse<T> {
  status: number;
  data: T;
  correlationId: string | null;
}

interface RequestOptions extends RequestInit {
  /** Status HTTP que se consideran respuesta válida (por defecto, 2xx). */
  acceptStatuses?: number[];
}

/** Hace una solicitud al gateway con un correlation ID nuevo. */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<ApiResponse<T>> {
  const { acceptStatuses, headers, ...init } = options;
  const correlationId = crypto.randomUUID();

  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      ...init,
      headers: { Accept: 'application/json', [CORRELATION_HEADER]: correlationId, ...headers },
    });
  } catch {
    throw new ApiError(`No se pudo conectar con el gateway (${API_URL}).`, 0, correlationId);
  }

  const returnedId = res.headers.get(CORRELATION_HEADER) ?? correlationId;
  const body: unknown = await res.json().catch(() => null);
  const accepted = acceptStatuses ? acceptStatuses.includes(res.status) : res.ok;

  if (!accepted) {
    const problem = isProblem(body) ? body : undefined;
    throw new ApiError(problem?.detail ?? problem?.title ?? `Error HTTP ${res.status}`, res.status, returnedId, problem);
  }
  return { status: res.status, data: body as T, correlationId: returnedId };
}

function isProblem(body: unknown): body is Problem {
  return typeof body === 'object' && body !== null && 'code' in body && 'status' in body;
}
