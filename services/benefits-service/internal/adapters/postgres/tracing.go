package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// startDB mide el repositorio completo, incluidos pool, transaccion y commit.
// No captura SQL, parametros, DSN ni mensajes de error del driver.
func startDB(ctx context.Context, operation string) (context.Context, func(error)) {
	ctx, span := otel.Tracer("gym/benefits/postgres").Start(ctx, operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", "postgresql")))
	return ctx, func(err error) {
		defer span.End()
		if err == nil {
			return
		}
		kind := "operation_rejected"
		var database *pgconn.PgError
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			kind = "deadline_exceeded"
		case errors.Is(err, context.Canceled):
			kind = "canceled"
		case errors.As(err, &database):
			kind = "postgres_error"
		}
		span.SetAttributes(attribute.String("error.type", kind))
		span.SetStatus(codes.Error, kind)
	}
}
