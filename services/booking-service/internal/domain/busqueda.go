package domain

import "time"

// Búsqueda de clases (etapa 8 de la entrega final, ADR-014, D-45). El índice es
// un modelo de lectura: nunca decide cupo, permisos ni reservas (RN-14).

// MaxProfundidadBusqueda limita pagina×tamano (decisión del equipo del
// 10/11/2026: 10.000 resultados, el límite por defecto del motor).
const MaxProfundidadBusqueda = 10000

// ValidarProfundidad rechaza páginas que superan la profundidad aprobada.
func (f FiltroClases) ValidarProfundidad() error {
	if f.Pagina < 1 || f.Tamano < 1 || int64(f.Pagina)*int64(f.Tamano) > MaxProfundidadBusqueda {
		return ErrValidacion
	}
	return nil
}

// HoraInicio es el "HH:MM" local del inicio, el valor que filtra franja (HU-13).
func (c Clase) HoraInicio() string { return c.Inicio.In(Zona).Format("15:04") }

// EventoClase es clase.actualizada traducido por el adaptador AMQP: el estado
// completo de la clase en una versión, sin envelope ni JSON.
type EventoClase struct {
	Clase      Clase
	OcurridoEn time.Time
}

// Valido verifica los campos que el índice necesita para filtrar y ordenar.
func (e EventoClase) Valido() bool {
	c := e.Clase
	estados := c.Estado == ClaseProgramada || c.Estado == ClaseCancelada || c.Estado == ClaseFinalizada
	return c.ID != "" && c.ActividadID != "" && c.ActividadNombre != "" && c.ProfesorID != "" && c.Version >= 1 &&
		estados && !c.Inicio.IsZero() && c.Fin.After(c.Inicio) && c.Capacidad >= 1 && c.Ocupacion >= 0 &&
		c.Ocupacion <= c.Capacidad && !e.OcurridoEn.IsZero()
}
