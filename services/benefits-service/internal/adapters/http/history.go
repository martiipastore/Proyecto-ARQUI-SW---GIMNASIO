package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/gin-gonic/gin"
	"strconv"
)

type AccountHistory interface {
	Historial(context.Context, string, int, int) (domain.PaginaMovimientos, error)
}

func RegisterAccountHistory(r *gin.Engine, app AccountHistory, secret string) {
	r.GET("/api/v1/benefits/me/movements", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || !domain.UUIDValido(c.GetHeader("X-User-Id")) {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para consultar tus movimientos."))
			return
		}
		page, size := 1, 20
		var err error
		for key, values := range c.Request.URL.Query() {
			if len(values) != 1 || (key != "pagina" && key != "tamano") || values[0] == "" {
				problem.Write(c, problem.New(400, problem.CodeValidation, "Solicitud inválida", "Usá pagina y tamano."))
				return
			}
		}
		if v, ok := c.GetQuery("pagina"); ok {
			page, err = strconv.Atoi(v)
			if err != nil {
				page = 0
			}
		}
		if v, ok := c.GetQuery("tamano"); ok {
			size, err = strconv.Atoi(v)
			if err != nil {
				size = 0
			}
		}
		out, err := app.Historial(c.Request.Context(), c.GetHeader("X-User-Id"), page, size)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrCuentaNoVinculada):
				problem.Write(c, problem.New(404, "CUENTA_NO_ENCONTRADA", "Cuenta pendiente", "Tu cuenta de puntos todavía no está disponible."))
			case errors.Is(err, domain.ErrOperacionInvalida):
				problem.Write(c, problem.New(400, problem.CodeValidation, "Solicitud inválida", "Revisá la paginación."))
			default:
				problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudieron consultar los movimientos."))
			}
			return
		}
		items := make([]gin.H, 0, len(out.Movimientos))
		for _, mov := range out.Movimientos {
			m := mov.Datos()
			origin := "PARTNER"
			if m.Tipo == domain.Asistencia {
				origin = "ASISTENCIA"
			} else if m.Tipo == domain.PenalizacionInasistencia {
				origin = "INASISTENCIA"
			}
			items = append(items, gin.H{"id": m.ID, "fecha": m.OcurridoEn, "aplicado_en": m.AplicadoEn, "motivo": m.Motivo, "puntos": m.Puntos, "estado": "CONFIRMADO", "origen": origin, "origen_id": m.OrigenID, "referencia": m.Referencia, "secuencia": m.Secuencia})
		}
		c.JSON(200, gin.H{"items": items, "total": out.Total, "pagina": page, "tamano": size})
	})
}
