import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '../api/client';
import { useSession } from '../auth/session';
import { DATE_TIME } from '../format';

// Etapa 13 (D-92): cada asistencia acredita además 100 puntos en MELT, la
// capacidad del grupo proveedor. Benefits hace el envío; acá el alumno ve el
// estado y, si MELT lo pide, el enlace privado para registrarse.
interface Envio {
  id: string;
  puntos: number;
  estado: 'PENDIENTE_ENVIO' | 'ACREDITADO' | 'PENDIENTE_REGISTRO' | 'RECHAZADO';
  intentos: number;
  motivo: string;
  creado_en: string;
  registro_url?: string;
  registro_vence?: string;
}

const STATE: Record<Envio['estado'], { label: string; icon: string; detail: string }> = {
  ACREDITADO: { label: 'Acreditados en MELT', icon: '✓', detail: 'Ya están en tu cuenta de MELT.' },
  PENDIENTE_REGISTRO: { label: 'Esperan tu registro', icon: '◆', detail: 'Registrate en MELT con tu mismo email para recibirlos.' },
  PENDIENTE_ENVIO: { label: 'Enviando a MELT', icon: '…', detail: 'MELT no respondió todavía; se reintenta automáticamente sin duplicar.' },
  RECHAZADO: { label: 'No acreditados', icon: '✕', detail: 'MELT no aceptó el envío.' },
};

export function MeltPoints() {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Envio[] | null>(null);
  const [perAttendance, setPerAttendance] = useState(100);
  const [error, setError] = useState('');
  const [available, setAvailable] = useState(true);

  const load = useCallback(async () => {
    try {
      const res = await authRequest<{ items: Envio[]; puntos_por_asistencia: number }>('/api/v1/benefits/me/melt');
      setItems(res.data.items);
      setPerAttendance(res.data.puntos_por_asistencia);
      setError('');
    } catch (cause) {
      // 404: la integración no está activa en este entorno; no se muestra la tarjeta.
      if (cause instanceof ApiError && cause.status === 404) { setAvailable(false); return; }
      setError(cause instanceof Error ? cause.message : 'No se pudieron consultar tus puntos MELT.');
    }
  }, [authRequest]);

  useEffect(() => { void load(); }, [load]);
  if (!available) return null;

  const credited = items?.filter((e) => e.estado === 'ACREDITADO').reduce((sum, e) => sum + e.puntos, 0) ?? 0;
  const link = items?.find((e) => e.estado === 'PENDIENTE_REGISTRO' && e.registro_url);

  return (
    <section className="melt-panel" aria-label="Puntos MELT">
      <header className="points-heading">
        <div><span>04</span><p>Puntos MELT · beneficio de un club aliado</p></div>
        <button type="button" onClick={() => void load()}>Actualizar</button>
      </header>
      <div className="melt-layout">
        <div className="melt-summary">
          <span>Por cada asistencia</span>
          <strong>+{perAttendance}<small>pts MELT</small></strong>
          <p>Además de tus puntos del gimnasio, cada clase a la que asistís suma puntos en MELT, el club de beneficios aliado.</p>
          <p className="melt-total">{credited.toLocaleString('es-AR')} pts ya acreditados</p>
          {link?.registro_url && (
            <a className="button button-primary" href={link.registro_url} target="_blank" rel="noopener noreferrer">
              Registrarme en MELT <span aria-hidden="true">↗</span>
            </a>
          )}
        </div>
        <div className="melt-list">
          {error && <div className="dashboard-error" role="alert"><strong>No pudimos consultar MELT.</strong><span>{error}</span></div>}
          {items === null ? (!error && <div className="dashboard-skeleton" aria-label="Cargando puntos MELT" />)
            : items.length === 0 ? <p className="dashboard-empty">Todavía no hay envíos. Se generan al registrar tu ingreso a una clase.</p>
              : (
                <ol className="movement-list">
                  {items.map((e) => {
                    const s = STATE[e.estado];
                    return (
                      <li key={e.id} className={`melt-${e.estado.toLowerCase()}`}>
                        <span className={`movement-value${e.estado === 'ACREDITADO' ? ' movement-positive' : ''}`}>+{e.puntos}</span>
                        <div>
                          <strong><span aria-hidden="true">{s.icon} </span>{s.label}</strong>
                          <span>{new Date(e.creado_en).toLocaleString('es-AR', DATE_TIME)} · {e.estado === 'RECHAZADO' && e.motivo ? e.motivo : s.detail}</span>
                          {e.estado === 'PENDIENTE_REGISTRO' && e.registro_vence && <small>El enlace vence el {new Date(e.registro_vence).toLocaleString('es-AR', DATE_TIME)}</small>}
                        </div>
                        <span className="movement-origin">MELT</span>
                      </li>
                    );
                  })}
                </ol>
              )}
        </div>
      </div>
    </section>
  );
}
