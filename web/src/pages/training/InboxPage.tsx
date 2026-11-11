import { useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { Question, trainingError } from './types';

// Bandeja del nutricionista (HU-34): consultas de sus pacientes actuales, de la
// más antigua a la más reciente (CA-34.1); cada una se responde una vez (CA-34.4).
export function InboxPage() {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Question[] | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    try { setItems((await authRequest<{ items: Question[] }>('/api/v1/nutrition/questions')).data.items); }
    catch (error) { setMessage(trainingError(error, 'No se pudieron cargar las consultas.')); }
  }, [authRequest]);
  useEffect(() => { void load(); }, [load]);

  async function answer(id: string) {
    setBusy(true); setMessage('');
    try {
      await authRequest(`/api/v1/nutrition/questions/${id}/answer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ respuesta: (drafts[id] ?? '').trim() }) });
      setMessage('Respuesta enviada.');
      setDrafts({ ...drafts, [id]: '' });
      await load();
    } catch (error) {
      setMessage(trainingError(error, 'No se pudo responder.'));
    } finally { setBusy(false); }
  }

  const pending = items?.filter((q) => q.estado === 'PENDIENTE') ?? [];
  const answered = items?.filter((q) => q.estado === 'RESPONDIDA') ?? [];
  return (
    <main className="page">
      <h1>Consultas de mis pacientes</h1>
      {message && <p role="status" aria-live="polite">{message}</p>}
      {items === null ? <p className="muted">Cargando…</p> : (
        <>
          <h2>Pendientes ({pending.length})</h2>
          {pending.length === 0 ? <p>No hay consultas pendientes.</p> : (
            <ul className="plain-list">
              {pending.map((q) => (
                <li key={q.id} className="club-card wide">
                  <p><strong>{q.pregunta}</strong></p>
                  <p className="muted">Enviada el {new Date(q.enviada_en).toLocaleString('es-AR')} · <a href={`#/nutri/pacientes/${q.alumno_id}`}>Ver paciente</a></p>
                  <label>Respuesta<textarea maxLength={2000} rows={3} value={drafts[q.id] ?? ''} onChange={(e) => setDrafts({ ...drafts, [q.id]: e.target.value })} /></label>
                  <button type="button" disabled={busy || !(drafts[q.id] ?? '').trim()} onClick={() => void answer(q.id)}>Responder</button>
                </li>
              ))}
            </ul>
          )}
          <h2>Respondidas ({answered.length})</h2>
          <ul>
            {answered.map((q) => <li key={q.id}>{q.pregunta} — <em>{q.respuesta}</em></li>)}
          </ul>
        </>
      )}
    </main>
  );
}
