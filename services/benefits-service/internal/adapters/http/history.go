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
		c.JSON(200, gin.H{"items": movimientosJSON(out, false), "total": out.Total, "pagina": page, "tamano": size})
	})
}

// movimientosJSON representa el ledger de benefits.v1. El administrador ve
// además el partner y el original revertido; el alumno ve estado y origen.
func movimientosJSON(p domain.PaginaMovimientos, admin bool) []gin.H {
	items := make([]gin.H, 0, len(p.Movimientos))
	for _, mov := range p.Movimientos {
		m := mov.Datos()
		origin := "PARTNER"
		switch m.Tipo {
		case domain.Asistencia:
			origin = "ASISTENCIA"
		case domain.PenalizacionInasistencia:
			origin = "INASISTENCIA"
		case domain.Reversion:
			origin = "REVERSION"
		case domain.Canje:
			origin = "CANJE"
		}
		item := gin.H{"id": m.ID, "fecha": m.OcurridoEn, "aplicado_en": m.AplicadoEn, "motivo": m.Motivo, "puntos": m.Puntos, "estado": p.EstadoMovimiento(m.ID), "origen": origin, "origen_id": m.OrigenID, "referencia": m.Referencia, "secuencia": m.Secuencia}
		if m.RevierteID != "" {
			item["revierte_id"] = m.RevierteID
		}
		if admin {
			item["partner_id"] = m.PartnerID
			if by := p.RevertidoPor[m.ID]; by != "" {
				item["revertido_por"] = by
			}
		}
		items = append(items, item)
	}
	return items
}
