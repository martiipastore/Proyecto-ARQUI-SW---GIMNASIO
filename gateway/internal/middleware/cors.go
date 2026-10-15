// Package middleware contiene middlewares propios del gateway.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
)

const (
	allowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	allowedHeaders = "Authorization, Content-Type, Idempotency-Key, " + correlation.Header
	exposedHeaders = correlation.Header + ", Retry-After"
)

// CORS permite que el frontend (otro origen, ej. http://localhost:5173) llame al gateway.
// Solo responde con headers CORS a los orígenes de la lista.
func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || !allowed[origin] {
			if c.Request.Method == http.MethodOptions && origin != "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Add("Vary", "Origin")
		h.Set("Access-Control-Expose-Headers", exposedHeaders)
		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", allowedMethods)
			h.Set("Access-Control-Allow-Headers", allowedHeaders)
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
