import { useCallback, useEffect, useState } from 'react';
import { API_URL, ApiError } from '../api/client';
import { getSystemStatus, type ServiceState, type SystemState, type SystemStatus } from '../api/status';

const REFRESH_MS = 10_000;

const SERVICE_LABEL: Record<ServiceState, string> = {
  ready: 'Listo',
  degraded: 'Degradado',
  not_ready: 'No listo',
  unreachable: 'Sin respuesta',
};

const SUMMARY: Record<SystemState, { text: string; tone: 'ok' | 'warn' | 'bad' }> = {
  ready: { text: 'Todos los servicios están listos', tone: 'ok' },
  degraded: { text: 'Sistema operativo con funciones degradadas', tone: 'warn' },
  not_ready: { text: 'Sistema no disponible: hay servicios que no están listos', tone: 'bad' },
};

const CHECK_LABEL: Record<string, string> = {
  up: 'OK',
  down: 'Caída',
};

export function SystemStatusPage() {
  const [data, setData] = useState<SystemStatus | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getSystemStatus();
      setData(res.data);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err : new ApiError(String(err), 0, null));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const id = window.setInterval(() => void load(), REFRESH_MS);
    return () => window.clearInterval(id);
  }, [load]);

  const summary = data ? (SUMMARY[data.status] ?? SUMMARY.not_ready) : null;

  return (
    <main className="page">
      <header className="page-header">
        <div>
          <h1>Estado del sistema</h1>
          <p className="muted">
            Consulta <code>GET {API_URL}/api/v1/status</code> cada {REFRESH_MS / 1000} s.
          </p>
        </div>
        <button type="button" onClick={() => void load()} disabled={loading}>
          {loading ? 'Actualizando…' : 'Actualizar'}
        </button>
      </header>

      {error && (
        <div className="alert" role="alert">
          <strong>No se pudo obtener el estado.</strong> {error.message}
          {error.correlationId && <span className="muted"> (correlation ID: {error.correlationId})</span>}
        </div>
      )}

      {data && summary && (
        <>
          <section className={`summary ${summary.tone}`} aria-live="polite">
            <span className="summary-dot" aria-hidden="true" />
            <div>
              <strong>{summary.text}</strong>
              <div className="muted">Última verificación: {new Date(data.checked_at).toLocaleString('es-AR')}</div>
            </div>
          </section>

          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Servicio</th>
                  <th>Estado</th>
                  <th className="num">Latencia</th>
                  <th>Dependencias</th>
                </tr>
              </thead>
              <tbody>
                {data.services.map((svc) => (
                  <tr key={svc.name}>
                    <td>
                      <code>{svc.name}</code>
                    </td>
                    <td>
                      <span className={`badge ${svc.status}`}>{SERVICE_LABEL[svc.status] ?? svc.status}</span>
                    </td>
                    <td className="num">{svc.latency_ms} ms</td>
                    <td>
                      {Object.keys(svc.checks).length === 0 && <span className="muted">—</span>}
                      {Object.entries(svc.checks).map(([dep, st]) => (
                        <span key={dep} className={`chip ${st}`}>
                          {dep}: {CHECK_LABEL[st] ?? st}
                        </span>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {!data && !error && <p className="muted">Cargando…</p>}
    </main>
  );
}
