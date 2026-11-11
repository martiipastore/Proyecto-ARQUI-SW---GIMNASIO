package domain

import (
	"strings"
	"time"
)

// Etapa 13 (D-92, ADR-009 v2): cada asistencia acredita además puntos en MELT,
// la capacidad del grupo proveedor. El gimnasio decide la cantidad; MELT no
// conoce membresías ni asistencias.
const PuntosMeltPorAsistencia = int64(100)

// Estados del envío al proveedor.
const (
	EnvioPendiente         = "PENDIENTE_ENVIO"    // todavía no confirmado por MELT
	EnvioAcreditado        = "ACREDITADO"         // MELT acreditó (o ya lo había hecho)
	EnvioPendienteRegistro = "PENDIENTE_REGISTRO" // la persona debe registrarse en MELT con el enlace
	EnvioRechazado         = "RECHAZADO"          // MELT rechazó los datos; no se reintenta
)

// EsperaMaximaMelt acota el reintento exponencial ante fallas transitorias.
const EsperaMaximaMelt = 10 * time.Minute

// ReconsultaRegistroMelt es cada cuánto se repite la solicitud de un envío
// pendiente de registro para enterarse de que la persona ya se registró.
const ReconsultaRegistroMelt = 2 * time.Minute

// EnvioMelt es la acreditación pendiente o resuelta en el proveedor. La
// referencia identifica la operación en MELT y nunca cambia entre reintentos.
type EnvioMelt struct {
	ID, AlumnoID, OrigenID, Referencia, Estado string
	Puntos                                     int64
	Intentos                                   int
	ProximoIntento                             time.Time
	RegistroURL                                string
	RegistroVence                              time.Time
	Motivo                                     string
	CreadoEn, ActualizadoEn                    time.Time
}

// EnvioMeltDeAsistencia crea el envío de una asistencia nueva. Las ausencias no
// acreditan en el proveedor.
func EnvioMeltDeAsistencia(h HechoBooking, id string, ahora time.Time) (EnvioMelt, bool) {
	if h.Tipo != Asistencia || !h.Valido() || !UUIDValido(id) {
		return EnvioMelt{}, false
	}
	return EnvioMelt{
		ID: id, AlumnoID: strings.ToLower(h.AlumnoID), OrigenID: strings.ToLower(h.OrigenID),
		Referencia: "metalfitness-asistencia-" + strings.ToLower(h.OrigenID),
		Estado:     EnvioPendiente, Puntos: PuntosMeltPorAsistencia,
		ProximoIntento: ahora, CreadoEn: ahora, ActualizadoEn: ahora,
	}, true
}

// ResultadoMelt es la respuesta del proveedor ya traducida por el adaptador
// anticorrupción: el dominio no conoce HTTP ni los códigos de MELT.
type ResultadoMelt struct {
	Tipo          string // ACREDITADO | PENDIENTE_REGISTRO | RECHAZADO | REINTENTAR
	RegistroURL   string
	RegistroVence time.Time
	Motivo        string
}

// Tipos de resultado.
const (
	ResultadoAcreditado = EnvioAcreditado
	ResultadoPendiente  = EnvioPendienteRegistro
	ResultadoRechazado  = EnvioRechazado
	ResultadoReintentar = "REINTENTAR"
)

// Registrar aplica el resultado de un intento. Un resultado transitorio vuelve
// a programar el envío con espera creciente (10 s, 20 s, 40 s… hasta 10 min)
// conservando la misma referencia, como exige el contrato de MELT.
func (e EnvioMelt) Registrar(r ResultadoMelt, ahora time.Time) EnvioMelt {
	e.Intentos++
	e.ActualizadoEn = ahora
	e.Motivo = r.Motivo
	switch r.Tipo {
	case ResultadoAcreditado:
		e.Estado = EnvioAcreditado
		e.RegistroURL = ""
		e.RegistroVence = time.Time{}
	case ResultadoPendiente:
		e.Estado = EnvioPendienteRegistro
		e.RegistroURL = r.RegistroURL
		e.RegistroVence = r.RegistroVence
		// Se reconsulta repitiendo la misma solicitud: mientras el enlace está
		// vigente MELT devuelve el mismo pendiente, y cuando la persona se
		// registra responde 200 (acreditado). Así el alumno ve el cambio.
		e.ProximoIntento = ahora.Add(ReconsultaRegistroMelt)
	case ResultadoRechazado:
		e.Estado = EnvioRechazado
	default:
		e.Estado = EnvioPendiente
		e.ProximoIntento = ahora.Add(EsperaMelt(e.Intentos))
	}
	return e
}

// EsperaMelt devuelve la espera antes del intento siguiente.
func EsperaMelt(intentos int) time.Duration {
	espera := 10 * time.Second
	for i := 1; i < intentos && espera < EsperaMaximaMelt; i++ {
		espera *= 2
	}
	if espera > EsperaMaximaMelt {
		return EsperaMaximaMelt
	}
	return espera
}

// RequiereEnvio indica si el envío debe intentarse ahora. Un enlace de registro
// vencido se renueva repitiendo la misma solicitud (contrato de MELT).
func (e EnvioMelt) RequiereEnvio(ahora time.Time) bool {
	switch e.Estado {
	case EnvioPendiente:
		return !ahora.Before(e.ProximoIntento)
	case EnvioPendienteRegistro:
		return !ahora.Before(e.ProximoIntento) || (!e.RegistroVence.IsZero() && ahora.After(e.RegistroVence))
	}
	return false
}
