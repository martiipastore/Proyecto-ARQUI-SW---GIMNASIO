package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Indexador mantiene el modelo de lectura de clases (etapa 8, ADR-014): aplica
// clase.actualizada solo si trae una versión más nueva que la indexada.
type Indexador struct {
	indice ports.IndiceEscritura
	reloj  ports.Reloj
}

func NewIndexador(indice ports.IndiceEscritura, reloj ports.Reloj) *Indexador {
	return &Indexador{indice: indice, reloj: reloj}
}

// ResultadoIndexado informa si el evento cambió el índice y cuánto tardó desde
// el commit en booking_db (occurred_at) hasta quedar indexado.
type ResultadoIndexado struct {
	Aplicado bool
	Retraso  time.Duration
}

func (i *Indexador) Aplicar(ctx context.Context, e domain.EventoClase) (ResultadoIndexado, error) {
	if !e.Valido() {
		return ResultadoIndexado{}, domain.ErrValidacion
	}
	aplicado, err := i.indice.Indexar(ctx, e.Clase)
	if err != nil {
		return ResultadoIndexado{}, err
	}
	return ResultadoIndexado{Aplicado: aplicado, Retraso: i.reloj.Ahora().Sub(e.OcurridoEn)}, nil
}

// ErrSinConvergencia indica que el índice nuevo no coincide con booking_db.
var ErrSinConvergencia = errors.New("el índice reconstruido no coincide con booking_db")

// ResultadoReindexado resume una reconstrucción completa.
type ResultadoReindexado struct {
	Indice, Anterior string
	Clases           int64
}

// Reindexar reconstruye el índice sin pausar Booking (decisión del equipo del
// 10/11/2026: snapshot + alias + repaso):
//  1. carga todas las clases de booking_db en un índice nuevo;
//  2. mueve el alias: desde ahí el indexer escribe en el índice nuevo;
//  3. repasa todas las clases otra vez. Con versionado externo, una versión
//     vieja nunca pisa una nueva, así que el repaso recupera los cambios
//     confirmados durante la carga sin deshacer eventos más recientes;
//  4. verifica convergencia (cantidad y suma de versiones) y borra el anterior.
func Reindexar(ctx context.Context, fuente ports.FuenteClases, destino ports.Reindexado, nombre string) (ResultadoReindexado, error) {
	out := ResultadoReindexado{Indice: nombre}
	if err := destino.CrearIndice(ctx, nombre); err != nil {
		return out, err
	}
	cargar := func() error {
		return fuente.RecorrerClases(ctx, 500, func(lote []domain.Clase) error { return destino.Cargar(ctx, nombre, lote) })
	}
	if err := cargar(); err != nil {
		return out, err
	}
	anterior, err := destino.Apuntar(ctx, nombre)
	if err != nil {
		return out, err
	}
	out.Anterior = anterior
	if err := cargar(); err != nil {
		return out, err
	}
	// Los eventos siguen llegando mientras se verifica: se reintenta unas veces
	// hasta que la base y el índice coinciden en el mismo instante.
	for intento := 0; ; intento++ {
		cantidad, suma, err := fuente.ResumenClases(ctx)
		if err != nil {
			return out, err
		}
		indexadas, sumaIndice, err := destino.Resumen(ctx, nombre)
		if err != nil {
			return out, err
		}
		if cantidad == indexadas && suma == sumaIndice {
			out.Clases = cantidad
			break
		}
		if intento == 20 {
			return out, fmt.Errorf("%w: %d/%d clases, versiones %d/%d", ErrSinConvergencia, indexadas, cantidad, sumaIndice, suma)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if anterior != "" && anterior != nombre {
		if err := destino.Eliminar(ctx, anterior); err != nil {
			return out, err
		}
	}
	return out, nil
}
