const activities = [
  { code: '01', name: 'Musculación', detail: 'Fuerza, técnica y progreso con turnos organizados.' },
  { code: '02', name: 'Funcional', detail: 'Entrenamiento dinámico para moverte mejor cada día.' },
  { code: '03', name: 'GAP', detail: 'Trabajo localizado con intensidad y control.' },
  { code: '04', name: 'Strong Nation', detail: 'Música, potencia y movimientos sincronizados.' },
  { code: '05', name: 'Zumba', detail: 'Energía grupal para entrenar disfrutando.' },
];

export function PublicHomePage() {
  return (
    <main className="public-home">
      <section className="hero" aria-labelledby="hero-title">
        <div className="hero-copy">
          <p className="eyebrow"><span aria-hidden="true">●</span> Entrenamiento · Constancia · Comunidad</p>
          <h1 id="hero-title">Construí<br /><em>tu fuerza.</em></h1>
          <p className="hero-lead">Un gimnasio donde cada reserva, cada entrenamiento y cada punto muestran tu progreso.</p>
          <div className="hero-actions">
            <a className="button button-primary" href="#/login">Ingresar al gimnasio <span aria-hidden="true">↗</span></a>
            <a className="button button-quiet" href="#actividades">Conocer actividades</a>
          </div>
          <dl className="hero-facts" aria-label="Características de Metal Fitness">
            <div><dt>05</dt><dd>actividades</dd></div>
            <div><dt>01</dt><dd>equipo</dd></div>
            <div><dt>∞</dt><dd>progreso</dd></div>
          </dl>
        </div>

        <div className="hero-machine" aria-hidden="true">
          <div className="hero-ring hero-ring-one" />
          <div className="hero-ring hero-ring-two" />
          <div className="hero-weight">
            <span className="hero-weight-label">MF</span>
            <span className="hero-weight-number">25</span>
          </div>
          <div className="hero-bar" />
          <p className="hero-stamp">Disciplina<br />en movimiento</p>
        </div>
      </section>

      <div className="ticker" aria-hidden="true">
        <div>FUERZA <span>◆</span> CONSTANCIA <span>◆</span> COMUNIDAD <span>◆</span> MOVIMIENTO <span>◆</span> FUERZA <span>◆</span> CONSTANCIA</div>
      </div>

      <section className="public-section activities-section" id="actividades" aria-labelledby="activities-title">
        <header className="section-heading">
          <p className="eyebrow">Entrená a tu manera</p>
          <h2 id="activities-title">Cinco formas de<br /><em>superarte.</em></h2>
          <p>Elegí la actividad que acompañe tu objetivo. Desde el trabajo de fuerza hasta la energía de una clase grupal.</p>
        </header>
        <ol className="activity-list">
          {activities.map((activity) => (
            <li key={activity.code}>
              <span className="activity-code">{activity.code}</span>
              <h3>{activity.name}</h3>
              <p>{activity.detail}</p>
              <span className="activity-arrow" aria-hidden="true">↗</span>
            </li>
          ))}
        </ol>
      </section>

      <section className="public-section progress-section" aria-labelledby="progress-title">
        <div className="progress-graphic" aria-hidden="true">
          <span className="progress-line" />
          <span className="progress-number">500</span>
          <span className="progress-unit">puntos por asistencia</span>
        </div>
        <div className="progress-copy">
          <p className="eyebrow">Tu recorrido, conectado</p>
          <h2 id="progress-title">Reservá. Entrená.<br /><em>Volvé más fuerte.</em></h2>
          <p>Consultá tus clases, membresía, planes y puntos desde un mismo lugar. Tu actividad queda organizada para que vos te concentres en entrenar.</p>
          <a className="text-link" href="#/login">Ver mi cuenta <span aria-hidden="true">→</span></a>
        </div>
      </section>

      <section className="public-section club-section" aria-labelledby="club-title">
        <div>
          <p className="eyebrow eyebrow-dark">Club de Beneficios</p>
          <h2 id="club-title">Tu constancia<br />también suma.</h2>
        </div>
        <div className="club-public-copy">
          <p>Acumulá puntos con tus asistencias y consultá todos tus movimientos. Tus puntos permanecen en tu cuenta aunque no tengas una membresía activa.</p>
          <a className="button button-dark" href="#/login">Consultar mis puntos</a>
        </div>
      </section>

      <footer className="public-footer">
        <div className="footer-brand"><strong>METAL</strong><strong>FITNESS</strong></div>
        <p>Entrenamiento, constancia y comunidad.</p>
        <a href="#/estado">Estado del sistema</a>
      </footer>
    </main>
  );
}
