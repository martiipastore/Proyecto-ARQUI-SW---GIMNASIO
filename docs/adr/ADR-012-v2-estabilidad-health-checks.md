# ADR-012 v2 — Evitar cambios de salud por verificaciones aisladas

| Campo | Valor |
|---|---|
| **Estado** | Aceptado como diseño inicial el 16/10/2026; implementación y evidencia pendientes |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D12 |
| **Reemplaza** | [ADR-012](ADR-012-balanceo-booking.md) |
| **Autores** | Equipo del TPI |
| **Relacionado con** | D-46 y D-51 del registro; ADR-007; BASE_TEORICA.md |

## Contexto y fundamento teórico

La página 87 de `Teórico EN PDF.pdf` explica que retirar y reincorporar una réplica por una única verificación puede producir flapping: cambios repetidos de estado ante demoras pasajeras. Recomienda varias verificaciones consecutivas para cambiar su elegibilidad. ADR-012 no precisaba ese criterio.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Retirar y reincorporar con un solo resultado | Reacción rápida | Sensible a una falla o recuperación aislada |
| Exigir resultados consecutivos | Evita cambios por un resultado aislado | Demora la detección y recuperación |
| Intervalos o umbrales altos sin medición | Reduce cambios frecuentes | Puede seguir enviando tráfico a una instancia caída durante demasiado tiempo |

## Decisión

Conservar dos instancias de Booking detrás de Traefik, con igual peso, sin afinidad y sin reintentos automáticos de escrituras. Mantener GET /health/ready cada 5 s con timeout de 2 s. Retirar una instancia tras **3 fallos consecutivos** y reincorporarla tras **2 éxitos consecutivos**. Un resultado contrario interrumpe la secuencia; una respuesta 200 degraded conserva el tratamiento de dependencias parciales de ADR-007.

Los valores 3/2 son parámetros iniciales aprobados por el equipo, no cifras prescritas por el teórico. Tres fallos evitan reaccionar a una demora aislada; dos éxitos aportan una confirmación adicional de recuperación. El costo es continuar intentando tráfico durante parte de la detección y demorar la reincorporación. El tiempo real depende de la programación y duración de los checks, no solo de multiplicar el intervalo.

Traefik sigue siendo una elección práctica adicional, justificada por la necesidad de ejecutar el balanceo exigido por el TP. Antes de implementarlo se debe verificar que la versión y configuración elegidas permitan expresar esta política. Este ADR no afirma soporte nativo ya comprobado. Si no lo permiten, presentar al equipo una alternativa basada en el mecanismo de clase; no incorporar componentes adicionales ni reducir la política sin aprobación.

## Consecuencias

- Positiva: mayor estabilidad frente a demoras pasajeras y recuperaciones aisladas.
- Negativa: detección y recuperación menos rápidas que con un solo resultado.
- Riesgo: una instancia puede fallar solicitudes antes de ser retirada. Mantener timeouts e idempotencia; medir el comportamiento antes de ajustar los umbrales.

## Cómo se verificará

En la etapa de balanceo: comprobar soporte de la herramienta, provocar uno y dos fallos sin retiro, comprobar retiro tras el tercero, interrumpir una secuencia con éxito y verificar reinicio del conteo. Para recuperación, un éxito no reincorpora y dos consecutivos sí; un fallo intermedio reinicia la secuencia. Registrar tiempos y distribución entre instancias. No se considera implementado ni probado al aprobar el diseño.
