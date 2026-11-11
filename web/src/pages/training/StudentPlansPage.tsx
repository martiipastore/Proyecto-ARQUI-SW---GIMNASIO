import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { EmptyState, Notice, NoticeState, PageHeading, SectionHeading } from '../../components/PageParts';
import { MeasurementsTable, NutritionPlanView, TrainingPlanView } from './TrainingViews';
import { Exercise, MEALS, Measurement, NutritionPlan, Plans, RoutineDay, today, trainingError, TrainingPlan } from './types';

// Ficha de un alumno asignado (etapa 10). El PROFESOR gestiona entrenamiento
// (HU-23); el NUTRICIONISTA, plan alimenticio y mediciones (HU-24, HU-25). El
// backend vuelve a verificar la asignación vigente en cada operación (RN-30).
export function StudentPlansPage({ id, role }: { id: string; role: 'PROFESOR' | 'NUTRICIONISTA' }) {
  const back = role === 'PROFESOR' ? '#/prof' : '#/nutri';
  return (
    <main className="page admin-page">
      <a className="admin-back-link" href={back}>← {role === 'PROFESOR' ? 'Mis alumnos' : 'Mis pacientes'}</a>
      <PageHeading
        eyebrow={role === 'PROFESOR' ? 'Ficha del alumno' : 'Ficha del paciente'}
        title={role === 'PROFESOR' ? 'Plan de entrenamiento' : 'Nutrición'}
        description="Cada operación verifica que la persona siga asignada a vos. Un plan nuevo reemplaza al vigente, que queda en el historial."
        index={role === 'PROFESOR' ? '02' : '01'}
      />
      {role === 'PROFESOR' ? <TrainingSection id={id} /> : <><NutritionSection id={id} /><MeasurementsSection id={id} /></>}
    </main>
  );
}

function useAction(reload: () => Promise<void>) {
  const [notice, setNotice] = useState<NoticeState | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async (action: () => Promise<unknown>, done: string) => {
    setBusy(true); setNotice(null);
    try { await action(); setNotice({ tone: 'success', text: done }); await reload(); return true; }
    catch (error) { setNotice({ tone: 'error', text: trainingError(error, 'No se pudo completar la operación.') }); return false; }
    finally { setBusy(false); }
  };
  const setMessage = (text: string) => setNotice({ tone: 'error', text });
  return { notice, setMessage, busy, run };
}

const post = (body: object) => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });

const NEW_EXERCISE: Exercise = { nombre: '', grupo_muscular: '', series: 3, repeticiones: 10, peso_sugerido: 0, descanso_segundos: 60, observaciones: '' };
const NEW_DAY = (n: number): RoutineDay => ({ nombre: `Día ${n}`, descripcion: '', ejercicios: [{ ...NEW_EXERCISE }] });

function TrainingSection({ id }: { id: string }) {
  const { authRequest } = useSession();
  const [plans, setPlans] = useState<Plans<TrainingPlan> | null>(null);
  const load = useCallback(async () => {
    try { setPlans((await authRequest<Plans<TrainingPlan>>(`/api/v1/training/plans?alumno_id=${id}`)).data); }
    catch (error) { setMessage(trainingError(error, 'No se pudieron cargar los planes.')); }
  }, [authRequest, id]);
  const { notice, setMessage, busy, run } = useAction(load);
  useEffect(() => { void load(); }, [load]);

  const [form, setForm] = useState({ objetivo: '', vigencia_desde: today(), vigencia_hasta: '', observaciones: '' });
  const [days, setDays] = useState<RoutineDay[]>([NEW_DAY(1)]);
  const setExercise = (d: number, e: number, patch: Partial<Exercise>) =>
    setDays(days.map((day, i) => i !== d ? day : { ...day, ejercicios: day.ejercicios.map((ex, j) => j !== e ? ex : { ...ex, ...patch }) }));

  async function submit(event: FormEvent) {
    event.preventDefault();
    const ok = await run(() => authRequest('/api/v1/training/plans', post({ alumno_id: id, ...form, dias: days })), plans?.vigente ? 'Plan creado. El anterior quedó finalizado en el historial.' : 'Plan creado. El alumno ya lo ve en “Mi plan”.');
    if (ok) { setDays([NEW_DAY(1)]); setForm({ objetivo: '', vigencia_desde: today(), vigencia_hasta: '', observaciones: '' }); }
  }

  return (
    <>
      <Notice key={notice?.text} notice={notice} />
      <section className="admin-section" aria-label="Plan vigente">
        <SectionHeading index="01" kicker="Lo que ve el alumno" title="Plan vigente" />
        {plans === null ? <div className="dashboard-skeleton" aria-label="Cargando plan" /> : plans.vigente ? <TrainingPlanView key={plans.vigente.id} plan={plans.vigente} /> : <EmptyState title="Sin plan vigente">Creá el primero con el formulario de abajo.</EmptyState>}
        {plans && plans.historial.length > 0 && (
          <details className="membership-history"><summary>Historial ({plans.historial.length})</summary>{plans.historial.map((p) => <TrainingPlanView key={p.id} plan={p} />)}</details>
        )}
      </section>
      <form className="club-card admin-section plan-form" aria-label="Nuevo plan" onSubmit={(e) => void submit(e)}>
        <SectionHeading index="02" kicker="Reemplaza al vigente" title="Nuevo plan" />
        <label>Objetivo<input required maxLength={200} value={form.objetivo} onChange={(e) => setForm({ ...form, objetivo: e.target.value })} /></label>
        <label>Desde<input type="date" required value={form.vigencia_desde} onChange={(e) => setForm({ ...form, vigencia_desde: e.target.value })} /></label>
        <label>Hasta<input type="date" required min={form.vigencia_desde} value={form.vigencia_hasta} onChange={(e) => setForm({ ...form, vigencia_hasta: e.target.value })} /></label>
        <label>Observaciones<textarea maxLength={1000} rows={2} value={form.observaciones} onChange={(e) => setForm({ ...form, observaciones: e.target.value })} /></label>
        {days.map((day, d) => (
          <fieldset key={d}>
            <legend>{day.nombre}</legend>
            <label>Nombre del día<input required maxLength={100} value={day.nombre} onChange={(e) => setDays(days.map((x, i) => i === d ? { ...x, nombre: e.target.value } : x))} /></label>
            {day.ejercicios.map((ex, e) => (
              <div className="inline-form" key={e}>
                <label>Ejercicio<input required maxLength={100} value={ex.nombre} onChange={(v) => setExercise(d, e, { nombre: v.target.value })} /></label>
                <label>Grupo<input maxLength={100} value={ex.grupo_muscular} onChange={(v) => setExercise(d, e, { grupo_muscular: v.target.value })} /></label>
                <label>Series<input type="number" required min={1} max={100} value={ex.series} onChange={(v) => setExercise(d, e, { series: Number(v.target.value) })} /></label>
                <label>Reps.<input type="number" required min={1} max={1000} value={ex.repeticiones} onChange={(v) => setExercise(d, e, { repeticiones: Number(v.target.value) })} /></label>
                <label>Peso (kg)<input type="number" min={0} step="0.5" value={ex.peso_sugerido} onChange={(v) => setExercise(d, e, { peso_sugerido: Number(v.target.value) })} /></label>
                <label>Descanso (s)<input type="number" min={0} value={ex.descanso_segundos} onChange={(v) => setExercise(d, e, { descanso_segundos: Number(v.target.value) })} /></label>
                {day.ejercicios.length > 1 && <button type="button" className="button-secondary" onClick={() => setDays(days.map((x, i) => i === d ? { ...x, ejercicios: x.ejercicios.filter((_, j) => j !== e) } : x))}>Quitar</button>}
              </div>
            ))}
            <div className="form-actions">
              <button type="button" className="button-secondary" onClick={() => setDays(days.map((x, i) => i === d ? { ...x, ejercicios: [...x.ejercicios, { ...NEW_EXERCISE }] } : x))}>+ Agregar ejercicio</button>
              {days.length > 1 && <button type="button" className="button-secondary" onClick={() => setDays(days.filter((_, i) => i !== d))}>Quitar día</button>}
            </div>
          </fieldset>
        ))}
        <div className="form-actions">
          <button type="button" className="button-secondary" onClick={() => setDays([...days, NEW_DAY(days.length + 1)])}>+ Agregar día</button>
          <button type="submit" disabled={busy}>{busy ? 'Creando…' : 'Crear plan'}</button>
        </div>
      </form>
    </>
  );
}

function NutritionSection({ id }: { id: string }) {
  const { authRequest } = useSession();
  const [plans, setPlans] = useState<Plans<NutritionPlan> | null>(null);
  const load = useCallback(async () => {
    try { setPlans((await authRequest<Plans<NutritionPlan>>(`/api/v1/nutrition/plans?alumno_id=${id}`)).data); }
    catch (error) { setMessage(trainingError(error, 'No se pudieron cargar los planes.')); }
  }, [authRequest, id]);
  const { notice, setMessage, busy, run } = useAction(load);
  useEffect(() => { void load(); }, [load]);
  const empty = { desayuno: '', colacion: '', almuerzo: '', merienda: '', cena: '', observaciones: '' };
  const [form, setForm] = useState(empty);

  return (
    <>
      <Notice key={notice?.text} notice={notice} />
      <section className="admin-section" aria-label="Plan alimenticio">
        <SectionHeading index="01" kicker="Lo que ve el paciente" title="Plan alimenticio" />
        {plans === null ? <div className="dashboard-skeleton" aria-label="Cargando plan" /> : plans.vigente ? <NutritionPlanView key={plans.vigente.id} plan={plans.vigente} /> : <EmptyState title="Sin plan vigente" />}
        {plans && plans.historial.length > 0 && <details className="membership-history"><summary>Historial ({plans.historial.length})</summary>{plans.historial.map((p) => <NutritionPlanView key={p.id} plan={p} />)}</details>}
        <form className="club-card plan-form nutrition-form" onSubmit={(e) => { e.preventDefault(); void run(() => authRequest('/api/v1/nutrition/plans', post({ alumno_id: id, ...form })), 'Plan cargado.').then((ok) => ok && setForm(empty)); }}>
          <h3>Nuevo plan (al menos una comida)</h3>
          {MEALS.map(([key, label]) => (
            <label key={key}>{label}<textarea maxLength={1000} rows={2} value={form[key as keyof typeof form]} onChange={(e) => setForm({ ...form, [key]: e.target.value })} /></label>
          ))}
          <label>Observaciones<textarea maxLength={1000} rows={2} value={form.observaciones} onChange={(e) => setForm({ ...form, observaciones: e.target.value })} /></label>
          <button type="submit" disabled={busy}>Cargar plan</button>
        </form>
      </section>
    </>
  );
}

function MeasurementsSection({ id }: { id: string }) {
  const { authRequest } = useSession();
  const [items, setItems] = useState<Measurement[] | null>(null);
  const load = useCallback(async () => {
    try { setItems((await authRequest<{ items: Measurement[] }>(`/api/v1/nutrition/measurements?alumno_id=${id}`)).data.items); }
    catch (error) { setMessage(trainingError(error, 'No se pudieron cargar las mediciones.')); }
  }, [authRequest, id]);
  const { notice, setMessage, busy, run } = useAction(load);
  useEffect(() => { void load(); }, [load]);
  const empty = { fecha: today(), peso: '', porcentaje_grasa: '', porcentaje_muscular: '', masa_muscular: '', observaciones: '' };
  const [form, setForm] = useState(empty);

  function submit(event: FormEvent) {
    event.preventDefault();
    const body = { alumno_id: id, fecha: form.fecha, peso: Number(form.peso), porcentaje_grasa: Number(form.porcentaje_grasa), porcentaje_muscular: Number(form.porcentaje_muscular), observaciones: form.observaciones, ...(form.masa_muscular ? { masa_muscular: Number(form.masa_muscular) } : {}) };
    void run(() => authRequest('/api/v1/nutrition/measurements', post(body)), 'Medición registrada.').then((ok) => ok && setForm(empty));
  }

  return (
    <section className="admin-section" aria-label="Mediciones">
      <SectionHeading index="02" kicker="Inmutables (RN-31)" title="Mediciones" />
      <Notice key={notice?.text} notice={notice} />
      {items === null ? <div className="dashboard-skeleton" aria-label="Cargando mediciones" /> : <MeasurementsTable items={items} />}
      <form className="inline-form" onSubmit={submit}>
        <label>Fecha<input type="date" required max={today()} value={form.fecha} onChange={(e) => setForm({ ...form, fecha: e.target.value })} /></label>
        <label>Peso (kg)<input type="number" required min={0.1} step="0.1" value={form.peso} onChange={(e) => setForm({ ...form, peso: e.target.value })} /></label>
        <label>% grasa<input type="number" required min={0} max={100} step="0.1" value={form.porcentaje_grasa} onChange={(e) => setForm({ ...form, porcentaje_grasa: e.target.value })} /></label>
        <label>% muscular<input type="number" required min={0} max={100} step="0.1" value={form.porcentaje_muscular} onChange={(e) => setForm({ ...form, porcentaje_muscular: e.target.value })} /></label>
        <label>Masa muscular (kg)<input type="number" min={0.1} step="0.1" value={form.masa_muscular} onChange={(e) => setForm({ ...form, masa_muscular: e.target.value })} /></label>
        <label>Observaciones<input maxLength={500} value={form.observaciones} onChange={(e) => setForm({ ...form, observaciones: e.target.value })} /></label>
        <button type="submit" disabled={busy}>Registrar</button>
      </form>
      <p className="muted">Las mediciones no se editan ni se borran: para corregir, registrá una nueva (RN-31).</p>
    </section>
  );
}
