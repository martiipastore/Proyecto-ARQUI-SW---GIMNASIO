import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { EmptyState, Notice, NoticeState, PageHeading, SectionHeading } from '../../components/PageParts';
import { MeasurementsTable, NutritionPlanView, TrainingPlanView } from './TrainingViews';
import { isNotFound, Measurement, NutritionPlan, Question, trainingError, TrainingPlan } from './types';

// Mi plan (alumno): HU-17, HU-18 y HU-33. Lo que no existe se informa sin error.
export function MyPlanPage() {
  const { authRequest } = useSession();
  const [training, setTraining] = useState<TrainingPlan | null | undefined>(undefined);
  const [nutrition, setNutrition] = useState<NutritionPlan | null | undefined>(undefined);
  const [measurements, setMeasurements] = useState<Measurement[] | null>(null);
  const [questions, setQuestions] = useState<Question[] | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [question, setQuestion] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const optional = async <T,>(path: string): Promise<T | null> => {
      try { return (await authRequest<T>(path)).data; } catch (error) { if (isNotFound(error)) return null; throw error; }
    };
    try {
      const [t, n, m, q] = await Promise.all([
        optional<TrainingPlan>('/api/v1/training/plans/me'),
        optional<NutritionPlan>('/api/v1/nutrition/plans/me'),
        authRequest<{ items: Measurement[] }>('/api/v1/nutrition/measurements/me'),
        authRequest<{ items: Question[] }>('/api/v1/nutrition/questions/me'),
      ]);
      setTraining(t); setNutrition(n); setMeasurements(m.data.items); setQuestions(q.data.items);
    } catch (error) {
      setNotice({ tone: 'error', text: trainingError(error, 'No se pudo cargar tu plan.') });
    }
  }, [authRequest]);

  useEffect(() => { void load(); }, [load]);

  async function send(event: FormEvent) {
    event.preventDefault();
    setBusy(true); setNotice(null);
    try {
      await authRequest('/api/v1/nutrition/questions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pregunta: question.trim() }) });
      setQuestion(''); setNotice({ tone: 'success', text: 'Consulta enviada a tu nutricionista.' });
      await load();
    } catch (error) {
      setNotice({ tone: 'error', text: trainingError(error, 'No se pudo enviar la consulta.') });
    } finally { setBusy(false); }
  }

  const loading = <div className="dashboard-skeleton" aria-label="Cargando" />;

  return (
    <main className="page admin-page">
      <PageHeading eyebrow="Tu progreso" title="Mi plan" description="Tu rutina, tu alimentación, tus mediciones y las consultas a tu nutricionista, en un mismo lugar." index="04" />
      <Notice key={notice?.text} notice={notice} />

      <section className="admin-section" aria-label="Plan de entrenamiento">
        <SectionHeading index="01" kicker="Preparado por tu profesor" title="Entrenamiento" />
        {training === undefined ? loading : training === null ? <EmptyState title="Sin plan de entrenamiento">Tu profesor todavía no te asignó una rutina.</EmptyState> : <TrainingPlanView key={training.id} plan={training} />}
      </section>

      <div className="plan-columns">
        <section className="admin-section" aria-label="Plan alimenticio">
          <SectionHeading index="02" kicker="Preparado por tu nutricionista" title="Alimentación" />
          {nutrition === undefined ? loading : nutrition === null ? <EmptyState title="Sin plan alimenticio" /> : <NutritionPlanView key={nutrition.id} plan={nutrition} />}
        </section>

        <section className="admin-section" aria-label="Consultas a mi nutricionista">
          <SectionHeading index="03" kicker="Respuesta de tu nutricionista" title="Consultas" />
          <form className="question-form" onSubmit={(e) => void send(e)}>
            <label>Tu consulta<textarea required maxLength={1000} rows={3} value={question} onChange={(e) => setQuestion(e.target.value)} /></label>
            <button type="submit" disabled={busy || question.trim() === ''}>{busy ? 'Enviando…' : 'Enviar consulta'}</button>
          </form>
          {questions === null ? null : questions.length === 0 ? <p className="muted">Todavía no enviaste consultas.</p> : (
            <ul className="question-list">
              {questions.map((q) => (
                <li key={`${q.id}-${q.estado}`} className={q.estado === 'RESPONDIDA' ? 'question-answered cut-in' : 'question-pending'}>
                  <p className="question-text">{q.pregunta}</p>
                  <small>Enviada el {new Date(q.enviada_en).toLocaleString('es-AR')}</small>
                  {q.estado === 'RESPONDIDA'
                    ? <div className="question-answer"><span>✓ Respuesta</span><p>{q.respuesta}</p><small>{new Date(q.respondida_en ?? '').toLocaleString('es-AR')}</small></div>
                    : <span className="status-label status-inactive">Pendiente de respuesta</span>}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      <section className="admin-section" aria-label="Mediciones">
        <SectionHeading index="04" kicker="Registradas por tu nutricionista" title="Mis mediciones" />
        {measurements === null ? loading : <MeasurementsTable items={measurements} />}
      </section>
    </main>
  );
}
