// Firma visual §3.3, momento 2: al confirmar una reserva, un disco se carga en
// una barra pequeña junto al mensaje de éxito (≈450 ms, solo transform/opacity).
// Con movimiento reducido se ve directamente el disco cargado. Es decorativo: el
// texto del aviso transmite el resultado.
export function PlateLoad() {
  return (
    <span className="plate-load" aria-hidden="true">
      <i className="plate-load-bar" />
      <i className="plate-load-disc" />
      <i className="plate-load-collar" />
    </span>
  );
}
