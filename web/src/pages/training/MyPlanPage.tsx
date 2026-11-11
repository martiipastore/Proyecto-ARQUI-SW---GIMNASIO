import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { MeasurementsTable, NutritionPlanView, TrainingPlanView } from './TrainingViews';
import { isNotFound, Measurement, NutritionPlan, Question, trainingError, TrainingPlan } from './types';

// Mi plan (alumno): HU-17, HU-18 y HU-33. Lo que no existe se informa sin error.
export function MyPlanPage() {
  const { authRequest } = useSession();
  const [training, setTraining] = useState<TrainingPlan | null | undefined>(undefined);
  const [nutrition, setNutrition] = useState<NutritionPlan | null | undefined>(undefined);
  const [measurements, setMeasurements] = useState<Measurement[] | null>(null);
  const [questions, setQuestions] = useState<Question[] | null>(null);
  const [message, setMessage] = useState('');
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
      setMessage(trainingError(error, 'No se pudo cargar tu plan.'));
    }
  }, [authRequest]);

  useEffect(() => { void load(); }, [load]);

  async function send(event: FormEvent) {
    event.preventDefault();
    setBusy(true); setMessage('');
    try {
      await authRequest('/api/v1/nutrition/questions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pregunta: question.trim() }) });
      setQuestion(''); setMessage('Consulta enviada a tu nutricionista.');
      await load();
    } catch (error) {
      setMessage(trainingError(error, 'No se pudo enviar la consulta.'));
    } finally { setBusy(false); }
  }

  return (
    <main className="page">
      <h1>Mi plan</h1>
      {message && <p role="status" aria-live="polite">{message}</p>}
      <section className="club-card wide" aria-label="Plan de entrenamiento">
        <h2>Entrenamiento</h2>
        {training === undefined ? <p className="muted">Cargando…</p> : training === null ? <p>No tenés un plan de entrenamiento asignado.</p> : <TrainingPlanView plan={training} />}
      </section>
      <section className="club-card wide" aria-label="Plan alimenticio">
        <h2>Alimentación</h2>
        {nutrition === undefined ? <p className="muted">Cargando…</p> : nutrition === null ? <p>No tenés un plan alimenticio asignado.</p> : <NutritionPlanView plan={nutrition} />}
      </section>
      <section className="club-card wide" aria-label="Mediciones">
        <h2>Mis mediciones</h2>
        {measurements === null ? <p className="muted">Cargando…</p> : <MeasurementsTable items={measurements} />}
      </section>
      <section className="club-card wide" aria-label="Consultas a mi nutricionista">
        <h2>Consultas a mi nutricionista</h2>
        <form className="inline-form" onSubmit={(e) => void send(e)}>
          <label>Tu consulta<textarea required maxLength={1000} rows={3} value={question} onChange={(e) => setQuestion(e.target.value)} /></label>
          <button type="submit" disabled={busy || question.trim() === ''}>Enviar</button>
        </form>
        {questions === null ? null : questions.length === 0 ? <p>Todavía no enviaste consultas.</p> : (
          <ul className="plain-list">
            {questions.map((q) => (
              <li key={q.id}>
                <strong>{q.pregunta}</strong><br />
                <span className="muted">Enviada el {new Date(q.enviada_en).toLocaleString('es-AR')}</span><br />
                {q.estado === 'RESPONDIDA' ? <>Respuesta: {q.respuesta} <span className="muted">({new Date(q.respondida_en ?? '').toLocaleString('es-AR')})</span></> : <em>Pendiente de respuesta</em>}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
