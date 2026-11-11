import { MEALS, Measurement, NutritionPlan, TrainingPlan } from './types';

// Vistas de solo lectura compartidas por alumno y profesionales (CA-17.1, CA-18.1/2).

export function TrainingPlanView({ plan }: { plan: TrainingPlan }) {
  return (
    <div>
      <p><strong>{plan.objetivo}</strong> · del {plan.vigencia_desde} al {plan.vigencia_hasta} · {plan.estado === 'VIGENTE' ? 'vigente' : 'finalizado'}</p>
      {plan.observaciones && <p>{plan.observaciones}</p>}
      {plan.dias.map((d, i) => (
        <div key={i}>
          <h4>{d.nombre}{d.descripcion ? ` — ${d.descripcion}` : ''}</h4>
          <table className="data-table">
            <thead><tr><th>Ejercicio</th><th>Grupo</th><th>Series</th><th>Reps.</th><th>Peso</th><th>Descanso</th><th>Notas</th></tr></thead>
            <tbody>
              {d.ejercicios.map((e, j) => (
                <tr key={j}>
                  <td>{e.nombre}</td><td>{e.grupo_muscular || '—'}</td><td>{e.series}</td><td>{e.repeticiones}</td>
                  <td>{e.peso_sugerido} kg</td><td>{e.descanso_segundos} s</td><td>{e.observaciones || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  );
}

export function NutritionPlanView({ plan }: { plan: NutritionPlan }) {
  return (
    <dl>
      {MEALS.map(([key, label]) => plan[key] ? <div key={key}><dt>{label}</dt><dd>{String(plan[key])}</dd></div> : null)}
      {plan.observaciones && <><dt>Observaciones</dt><dd>{plan.observaciones}</dd></>}
      <dd className="muted">Cargado el {plan.fecha} · {plan.estado === 'VIGENTE' ? 'vigente' : 'finalizado'}</dd>
    </dl>
  );
}

export function MeasurementsTable({ items }: { items: Measurement[] }) {
  if (items.length === 0) return <p>Todavía no hay mediciones.</p>;
  return (
    <table className="data-table">
      <thead><tr><th>Fecha</th><th>Peso</th><th>% grasa</th><th>% muscular</th><th>Masa muscular</th><th>Observaciones</th></tr></thead>
      <tbody>
        {items.map((m) => (
          <tr key={m.id}>
            <td>{m.fecha}</td><td>{m.peso} kg</td><td>{m.porcentaje_grasa}%</td><td>{m.porcentaje_muscular}%</td>
            <td>{m.masa_muscular === null ? '—' : `${m.masa_muscular} kg`}</td><td>{m.observaciones || '—'}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
