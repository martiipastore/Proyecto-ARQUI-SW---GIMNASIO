package domain

// PoliticaAcreditacion separa las reglas configurables de los bloqueos y SQL.
// Un límite diario nil conserva la configuración histórica sin presupuesto.
type PoliticaAcreditacion struct {
	Permitido       bool
	MaximoOperacion int64
	MaximoDia       *int64
	UsadosHoy       int64
}

// ValidarEvento permite exigir identidad del hecho sin romper clientes previos.
func ValidarEvento(requerido bool, evento string) error {
	if (requerido && evento == "") || (evento != "" && !TextoValido(evento, 1, 128)) {
		return ErrOperacionInvalida
	}
	return nil
}

func (p PoliticaAcreditacion) Validar(puntos int64) error {
	if !p.Permitido {
		return ErrPartnerNoAutorizado
	}
	// Cero representa verificar permiso para replay, sin consumir presupuesto.
	if puntos == 0 {
		return nil
	}
	if puntos < 0 || puntos > p.MaximoOperacion {
		return ErrOperacionInvalida
	}
	if p.MaximoDia != nil && (puntos > *p.MaximoDia || p.UsadosHoy > *p.MaximoDia-puntos) {
		return ErrOperacionInvalida
	}
	return nil
}
