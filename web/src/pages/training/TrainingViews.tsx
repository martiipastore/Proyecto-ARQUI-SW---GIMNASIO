import { MEALS, Measurement, NutritionPlan, TrainingPlan } from './types';

// Vistas de solo lectura compartidas por alumno y profesionales (CA-17.1, CA-18.1/2).
// La clase "cut-in" es la firma visual §3.3 (plan nuevo, medición o respuesta):
// se repite cuando cambia la key del elemento, es decir, cuando llega un dato nuevo.

export function TrainingPlanView({ plan }: { plan: TrainingPlan }) {
  return (
    <article className="plan-view cut-in">
      <header className="plan-view-header">
        <span className={`status-label ${plan.estado === 'VIGENTE' ? 'status-active' : 'status-inactive'}`}>{plan.estado === 'VIGENTE' ? 'Vigente' : 'Finalizado'}</span>
        <h3>{plan.objetivo}</h3>
        <p>Del <strong>{plan.vigencia_desde}</strong> al <strong>{plan.vigencia_hasta}</strong></p>
        {plan.observaciones && <p className="plan-notes">{plan.observaciones}</p>}
      </header>
      {plan.dias.map((d, i) => (
        <section key={i} className="routine-day">
          <h4><span aria-hidden="true">{String(i + 1).padStart(2, '0')}</span> {d.nombre}{d.descripcion ? ` — ${d.descripcion}` : ''}</h4>
          <table className="data-table responsive-table">
            <thead><tr><th>Ejercicio</th><th>Grupo</th><th>Series</th><th>Reps.</th><th>Peso</th><th>Descanso</th><th>Notas</th></tr></thead>
            <tbody>
              {d.ejercicios.map((e, j) => (
                <tr key={j}>
                  <td data-label="Ejercicio"><strong>{e.nombre}</strong></td><td data-label="Grupo">{e.grupo_muscular || '—'}</td>
                  <td data-label="Series">{e.series}</td><td data-label="Reps.">{e.repeticiones}</td>
                  <td data-label="Peso">{e.peso_sugerido} kg</td><td data-label="Descanso">{e.descanso_segundos} s</td><td data-label="Notas">{e.observaciones || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      ))}
    </article>
  );
}

export function NutritionPlanView({ plan }: { plan: NutritionPlan }) {
  return (
    <article className="plan-view cut-in">
      <header className="plan-view-header">
        <span className={`status-label ${plan.estado === 'VIGENTE' ? 'status-active' : 'status-inactive'}`}>{plan.estado === 'VIGENTE' ? 'Vigente' : 'Finalizado'}</span>
        <p>Cargado el <strong>{plan.fecha}</strong></p>
      </header>
      <dl className="meal-grid">
        {MEALS.map(([key, label]) => plan[key] ? <div key={key}><dt>{label}</dt><dd>{String(plan[key])}</dd></div> : null)}
        {plan.observaciones && <div className="meal-notes"><dt>Observaciones</dt><dd>{plan.observaciones}</dd></div>}
      </dl>
    </article>
  );
}

export function MeasurementsTable({ items }: { items: Measurement[] }) {
  if (items.length === 0) return <p className="muted">Todavía no hay mediciones.</p>;
  return (
    <div className="cut-in" key={items.length}>
      <table className="data-table responsive-table">
        <thead><tr><th>Fecha</th><th>Peso</th><th>% grasa</th><th>% muscular</th><th>Masa muscular</th><th>Observaciones</th></tr></thead>
        <tbody>
          {items.map((m) => (
            <tr key={m.id}>
              <td data-label="Fecha">{m.fecha}</td><td data-label="Peso">{m.peso} kg</td><td data-label="% grasa">{m.porcentaje_grasa}%</td><td data-label="% muscular">{m.porcentaje_muscular}%</td>
              <td data-label="Masa muscular">{m.masa_muscular === null ? '—' : `${m.masa_muscular} kg`}</td><td data-label="Observaciones">{m.observaciones || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
