// Fecha y hora en 24 h y zona de Argentina (convención del proyecto): el formato
// por defecto de es-AR muestra 12 h sin a. m./p. m. y resulta ambiguo.
export const DATE_TIME: Intl.DateTimeFormatOptions = {
  timeZone: 'America/Argentina/Buenos_Aires',
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
};
