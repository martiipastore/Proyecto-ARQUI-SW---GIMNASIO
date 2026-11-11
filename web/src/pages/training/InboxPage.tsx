import { useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { EmptyState, Notice, NoticeState, PageHeading, SectionHeading } from '../../components/PageParts';
import { Question, trainingError } from './types';

// Bandeja del nutricionista (HU-34): consultas de sus pacientes actuales, de la
// más antigua a la más reciente (CA-34.1); cada una se responde una vez (CA-34.4).
export function InboxPage() {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Question[] | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [busy, setBusy] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    try { setItems((await authRequest<{ items: Question[] }>('/api/v1/nutrition/questions')).data.items); }
    catch (error) { setNotice({ tone: 'error', text: trainingError(error, 'No se pudieron cargar las consultas.') }); }
  }, [authRequest]);
  useEffect(() => { void load(); }, [load]);

  async function answer(id: string) {
    setBusy(true); setNotice(null);
    try {
      await authRequest(`/api/v1/nutrition/questions/${id}/answer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ respuesta: (drafts[id] ?? '').trim() }) });
      setNotice({ tone: 'success', text: 'Respuesta enviada.' });
      setDrafts({ ...drafts, [id]: '' });
      await load();
    } catch (error) {
      setNotice({ tone: 'error', text: trainingError(error, 'No se pudo responder.') });
    } finally { setBusy(false); }
  }

  const pending = items?.filter((q) => q.estado === 'PENDIENTE') ?? [];
  const answered = items?.filter((q) => q.estado === 'RESPONDIDA') ?? [];
  return (
    <main className="page admin-page">
      <PageHeading eyebrow="Nutrición" title="Consultas" description="Las preguntas de tus pacientes actuales, de la más antigua a la más reciente. Cada consulta se responde una sola vez." index="02" />
      <Notice key={notice?.text} notice={notice} />
      {items === null ? (notice ? null : <div className="dashboard-skeleton" aria-label="Cargando consultas" />) : (
        <>
          <section className="admin-section">
            <SectionHeading index={String(pending.length).padStart(2, '0')} kicker="Por responder" title="Pendientes" />
            {pending.length === 0 ? <EmptyState title="Bandeja al día">No hay consultas pendientes.</EmptyState> : (
              <ul className="question-list">
                {pending.map((q) => (
                  <li key={q.id} className="question-pending">
                    <p className="question-text">{q.pregunta}</p>
                    <small>Enviada el {new Date(q.enviada_en).toLocaleString('es-AR')} · <a href={`#/nutri/pacientes/${q.alumno_id}`}>Ver paciente</a></small>
                    <label>Respuesta<textarea maxLength={2000} rows={3} value={drafts[q.id] ?? ''} onChange={(e) => setDrafts({ ...drafts, [q.id]: e.target.value })} /></label>
                    <button type="button" disabled={busy || !(drafts[q.id] ?? '').trim()} onClick={() => void answer(q.id)}>Responder</button>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section className="admin-section">
            <SectionHeading index={String(answered.length).padStart(2, '0')} kicker="Historial" title="Respondidas" />
            {answered.length === 0 ? <p className="muted">Todavía no respondiste consultas.</p> : (
              <ul className="question-list">
                {answered.map((q) => (
                  <li key={q.id} className="question-answered cut-in">
                    <p className="question-text">{q.pregunta}</p>
                    <div className="question-answer"><span>✓ Tu respuesta</span><p>{q.respuesta}</p></div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}
    </main>
  );
}
