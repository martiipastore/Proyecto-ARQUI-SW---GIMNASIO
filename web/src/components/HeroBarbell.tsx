export function HeroBarbell() {
  return (
    <div className="barbell-scene barbell-scene--play" aria-hidden="true">
      <div className="barbell-orbit barbell-orbit-outer" />
      <div className="barbell-orbit barbell-orbit-inner" />
      <div className="barbell-impact" />

      <div className="barbell-assembly">
        <div className="plate-stack plate-stack-left">
          <i className="plate plate-thin" />
          <i className="plate plate-medium" />
          <i className="plate plate-heavy"><span>25</span></i>
        </div>
        <div className="barbell-collar barbell-collar-left" />
        <div className="barbell-shaft" />
        <div className="barbell-collar barbell-collar-right" />
        <div className="plate-stack plate-stack-right">
          <i className="plate plate-heavy"><span>25</span></i>
          <i className="plate plate-medium" />
          <i className="plate plate-thin" />
        </div>
      </div>

      <div className="barbell-wordmark">
        <span>METAL</span>
        <span>FITNESS</span>
      </div>
      <div className="barbell-spark"><i /></div>
      <p className="hero-stamp">Disciplina<br />en movimiento</p>
    </div>
  );
}
