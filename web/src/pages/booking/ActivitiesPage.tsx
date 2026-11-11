import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSession } from '../../auth/session';
import { useConfirm } from '../../components/ConfirmDialog';
import { fullName, Person } from '../admin/types';
import { Actividad, DAYS, errorText, Horario, todayPlus } from './types';

// Actividades, horarios recurrentes y generación de clases (booking.v1, HU-06 y
// HU-07). Topes, franjas de Musculación, responsable PROFESOR y superposiciones
// los valida el backend (RN-07, RN-08, CA-07.3/4/6).
const MUSCLE_SLOTS = ['08:00', '10:00', '12:00', '14:00', '16:00', '18:00', '20:00'];
const EMPTY_SCHEDULE = { dia: '1', inicio: '09:00', duracion: '60', profesor: '', capacidad: '' };

export function ActivitiesPage() {
  const { authRequest } = useSession();
  const [dialog, confirm] = useConfirm();
  const [activities, setActivities] = useState<Actividad[]>([]);
  const [selected, setSelected] = useState('');
  const [schedules, setSchedules] = useState<Horario[]>([]);
  const [teachers, setTeachers] = useState<Person[]>([]);
  const [newActivity, setNewActivity] = useState({ nombre: '', capacidad: '30' });
  const [schedule, setSchedule] = useState(EMPTY_SCHEDULE);
  const [period, setPeriod] = useState({ desde: todayPlus(), hasta: todayPlus(13) });
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const current = activities.find((a) => a.id === selected);

  const loadActivities = useCallback(async () => {
    try {
      const res = await authRequest<{ items: Actividad[] }>('/api/v1/activities');
      setActivities(res.data.items);
      setSelected((prev) => prev || res.data.items[0]?.id || '');
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron cargar las actividades.'));
    }
  }, [authRequest]);

  const loadSchedules = useCallback(async () => {
    if (!selected) return;
    try {
      const res = await authRequest<{ items: Horario[] }>(`/api/v1/activities/${selected}/schedules`);
      setSchedules(res.data.items);
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron cargar los horarios.'));
    }
  }, [authRequest, selected]);

  useEffect(() => {
    void loadActivities();
    authRequest<{ items: Person[] }>('/api/v1/users?rol=PROFESOR&estado=ACTIVO&tamano=100')
      .then((res) => setTeachers(res.data.items))
      .catch(() => setTeachers([]));
  }, [authRequest, loadActivities]);

  useEffect(() => {
    void loadSchedules();
  }, [loadSchedules]);

  useEffect(() => {
    if (current) {
      setSchedule((s) => ({ ...s, inicio: current.musculacion ? '08:00' : s.inicio, duracion: current.musculacion ? '120' : s.duracion, capacidad: String(current.capacidad_maxima) }));
    }
  }, [current]);

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    setMessage('');
    try {
      await action();
      setMessage(done);
      await loadActivities();
      await loadSchedules();
    } catch (error) {
      setMessage(errorText(error, 'No se pudo completar la operación.'));
    } finally {
      setBusy(false);
    }
  }

  const send = (path: string, method: string, body: object) =>
    authRequest(path, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });

  function createActivity(event: FormEvent) {
    event.preventDefault();
    void run(() => send('/api/v1/activities', 'POST', { nombre: newActivity.nombre, capacidad_maxima: Number(newActivity.capacidad) }), 'Actividad creada.')
      .then(() => setNewActivity({ nombre: '', capacidad: '30' }));
  }

  function toggle(a: Actividad) {
    void run(() => send(`/api/v1/activities/${a.id}`, 'PUT', { activa: !a.activa }), a.activa ? 'Actividad inactivada.' : 'Actividad activada.');
  }

  async function changeMax(a: Actividad) {
    const value = await confirm({
      title: `Capacidad de ${a.nombre}`,
      message: `Nueva capacidad máxima de la actividad. Tope: ${a.musculacion ? 50 : 30} personas.`,
      confirmLabel: 'Guardar capacidad',
      input: { label: 'Capacidad máxima', type: 'number', initial: String(a.capacidad_maxima), min: 1, max: a.musculacion ? 50 : 30 },
    });
    if (value === null) return;
    void run(() => send(`/api/v1/activities/${a.id}`, 'PUT', { capacidad_maxima: Number(value) }), 'Capacidad máxima actualizada.');
  }

  function createSchedule(event: FormEvent) {
    event.preventDefault();
    void run(() => send(`/api/v1/activities/${selected}/schedules`, 'POST', {
      dia_semana: Number(schedule.dia),
      inicio: schedule.inicio,
      duracion_minutos: Number(schedule.duracion),
      profesor_id: schedule.profesor,
      capacidad: Number(schedule.capacidad),
    }), 'Horario creado.');
  }

  async function generate(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setMessage('');
    try {
      const res = await authRequest<{ creadas: number }>('/api/v1/classes/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(period),
      });
      setMessage(`Se generaron ${res.data.creadas} clases nuevas; las existentes no se duplican.`);
    } catch (error) {
      setMessage(errorText(error, 'No se pudieron generar las clases.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="page admin-page">
      {dialog}
      <header className="admin-page-heading">
        <div>
          <p className="admin-eyebrow">Programación del gimnasio</p>
          <h1>Actividades y horarios</h1>
          <p>Configurá capacidades, profesores y franjas horarias antes de generar las clases.</p>
        </div>
        <span className="admin-page-index" aria-hidden="true">02</span>
      </header>
      {message && <p role="status">{message}</p>}

      <table className="data-table">
        <thead>
          <tr><th>Actividad</th><th>Capacidad máxima</th><th>Puntos por asistencia</th><th>Estado</th><th /></tr>
        </thead>
        <tbody>
          {activities.map((a) => (
            <tr key={a.id} aria-selected={a.id === selected}>
              <td><button type="button" className="link" onClick={() => setSelected(a.id)}>{a.nombre}</button></td>
              <td>{a.capacidad_maxima}</td>
              <td>{a.puntos_asistencia}</td>
              <td>{a.activa ? 'Activa' : 'Inactiva'}</td>
              <td>
                <button type="button" disabled={busy} onClick={() => void changeMax(a)}>Capacidad</button>{' '}
                <button type="button" disabled={busy} onClick={() => toggle(a)}>{a.activa ? 'Inactivar' : 'Activar'}</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <form className="inline-form admin-toolbar-panel" onSubmit={createActivity}>
        <label>Nueva actividad <input value={newActivity.nombre} onChange={(e) => setNewActivity({ ...newActivity, nombre: e.target.value })} maxLength={60} required /></label>
        <label>Capacidad máxima <input type="number" min={1} max={30} value={newActivity.capacidad} onChange={(e) => setNewActivity({ ...newActivity, capacidad: e.target.value })} required /></label>
        <button type="submit" disabled={busy}>Crear</button>
      </form>

      {current && (
        <section className="admin-section">
          <div className="admin-section-heading">
            <span aria-hidden="true">03</span>
            <div><p>Actividad seleccionada</p><h2>Horarios de {current.nombre}</h2></div>
          </div>
          {schedules.length === 0 ? (
            <p>Sin horarios.</p>
          ) : (
            <table className="data-table">
              <thead><tr><th>Día</th><th>Horario</th><th>Profesor</th><th>Capacidad</th></tr></thead>
              <tbody>
                {schedules.map((h) => (
                  <tr key={h.id}><td>{DAYS[h.dia_semana]}</td><td>{h.inicio}–{h.fin}</td><td>{h.profesor.nombre}</td><td>{h.capacidad}</td></tr>
                ))}
              </tbody>
            </table>
          )}
          <form className="inline-form" onSubmit={createSchedule}>
            <label>Día
              <select value={schedule.dia} onChange={(e) => setSchedule({ ...schedule, dia: e.target.value })}>
                {DAYS.slice(1).map((d, i) => <option key={d} value={i + 1}>{d}</option>)}
              </select>
            </label>
            <label>Inicio
              {current.musculacion ? (
                <select value={schedule.inicio} onChange={(e) => setSchedule({ ...schedule, inicio: e.target.value })}>
                  {MUSCLE_SLOTS.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              ) : (
                <input type="time" value={schedule.inicio} onChange={(e) => setSchedule({ ...schedule, inicio: e.target.value })} required />
              )}
            </label>
            {!current.musculacion && (
              <label>Duración (min) <input type="number" min={15} max={240} value={schedule.duracion} onChange={(e) => setSchedule({ ...schedule, duracion: e.target.value })} required /></label>
            )}
            <label>Profesor
              <select value={schedule.profesor} onChange={(e) => setSchedule({ ...schedule, profesor: e.target.value })} required>
                <option value="">Elegir…</option>
                {teachers.map((t) => <option key={t.id} value={t.id}>{fullName(t)}</option>)}
              </select>
            </label>
            <label>Capacidad <input type="number" min={1} max={current.capacidad_maxima} value={schedule.capacidad} onChange={(e) => setSchedule({ ...schedule, capacidad: e.target.value })} required /></label>
            <button type="submit" disabled={busy || !current.activa}>Agregar horario</button>
          </form>
        </section>
      )}

      <section className="admin-section">
        <div className="admin-section-heading">
          <span aria-hidden="true">04</span>
          <div><p>Último paso</p><h2>Generar clases</h2></div>
        </div>
        <form className="inline-form" onSubmit={generate}>
          <label>Desde <input type="date" value={period.desde} onChange={(e) => setPeriod({ ...period, desde: e.target.value })} required /></label>
          <label>Hasta <input type="date" value={period.hasta} onChange={(e) => setPeriod({ ...period, hasta: e.target.value })} required /></label>
          <button type="submit" disabled={busy}>Generar</button>
        </form>
        <p className="muted">Se generan las clases futuras de todas las actividades activas; regenerar un período no duplica clases.</p>
      </section>
    </main>
  );
}
