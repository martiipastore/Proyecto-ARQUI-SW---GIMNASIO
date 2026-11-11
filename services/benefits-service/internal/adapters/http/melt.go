package http

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/gin-gonic/gin"
)

type MisEnviosMelt interface {
	MisEnvios(context.Context, string) ([]domain.EnvioMelt, error)
}

// RegisterMeltHistory expone al alumno sus acreditaciones en MELT (etapa 13).
// El enlace de registro solo se entrega a su destinatario autenticado y
// mientras está vigente; nunca se registra en logs (contrato MELT).
func RegisterMeltHistory(r *gin.Engine, app MisEnviosMelt, secret string) {
	r.GET("/api/v1/benefits/me/melt", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || !domain.UUIDValido(c.GetHeader("X-User-Id")) {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para consultar tus puntos MELT."))
			return
		}
		envios, err := app.MisEnvios(c.Request.Context(), c.GetHeader("X-User-Id"))
		if err != nil {
			problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudieron consultar los envíos a MELT."))
			return
		}
		ahora := time.Now()
		items := make([]gin.H, 0, len(envios))
		for _, e := range envios {
			item := gin.H{"id": e.ID, "puntos": e.Puntos, "estado": e.Estado, "intentos": e.Intentos, "motivo": e.Motivo, "creado_en": e.CreadoEn, "actualizado_en": e.ActualizadoEn}
			if e.Estado == domain.EnvioPendiente {
				item["proximo_intento"] = e.ProximoIntento
			}
			if e.Estado == domain.EnvioPendienteRegistro && e.RegistroURL != "" && ahora.Before(e.RegistroVence) {
				item["registro_url"] = e.RegistroURL
				item["registro_vence"] = e.RegistroVence
			}
			items = append(items, item)
		}
		c.JSON(200, gin.H{"items": items, "puntos_por_asistencia": domain.PuntosMeltPorAsistencia})
	})
}
