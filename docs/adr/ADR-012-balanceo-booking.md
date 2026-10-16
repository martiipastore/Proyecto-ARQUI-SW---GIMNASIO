# ADR-012 — Balancear dos instancias de Booking mediante Traefik

| Campo | Valor |
|---|---|
| **Estado** | Reemplazado por [ADR-012 v2](ADR-012-v2-estabilidad-health-checks.md) |
| **Fecha** | 2026-10-16 |
| **Decisión del TP** | D12 |
| **Autores** | Equipo del TPI |
| **Relacionado con** | ADR-001; ADR-003; ADR-005 v2; ADR-007; ARCHITECTURE §14–15 |

## Contexto

El TP requiere dos o más instancias de un servicio, detección de indisponibilidad y evidencia observable de distribución. El diseño existente elige Booking por la carga concurrente de reservas; hoy Compose tiene una sola instancia.

## Alternativas consideradas

| Alternativa | Ventaja | Desventaja |
|---|---|---|
| Réplicas sin balanceador | Menos infraestructura | No distribuye tráfico ni retira destinos caídos |
| Balancear Benefits | Flujo de partners simple de medir | Cambia el servicio seleccionado en el diseño vigente |
| Dos Booking detrás de Traefik | Conserva el diseño; permite demostrar concurrencia | La base continúa siendo compartida y puede limitar capacidad |

## Decisión

Formalizar dos instancias de booking-service detrás de Traefik, con pesos iguales, sin afinidad de sesión y sin reintentos automáticos de escrituras. El gateway seguirá siendo frontera de autenticación y routing; Traefik enviará tráfico a instancias saludables. Su función de borde HTTPS de producción se distingue de este balanceo interno.

Parámetros iniciales aprobados para health check activo: GET /health/ready cada 5 s, timeout 2 s; aceptar HTTP 200 (ready o degraded), retirar la instancia ante fallo y reincorporarla tras recuperación. La respuesta degraded no retira todas las capacidades por una dependencia parcial (ADR-007). No se promete detección instantánea ni reparto exactamente 50/50.

Ambas instancias usan booking_db con sus permisos propios. Las restricciones transaccionales siguen garantizando cupo y unicidad; el balanceador no protege esas invariantes. Jobs programados requieren exclusión entre instancias y consumidores idempotentes conforme al diseño vigente.

## Consecuencias

- Positiva: una instancia caída deja de recibir tráfico y se puede escalar el proceso de Booking.
- Negativa: Traefik y PostgreSQL siguen siendo puntos únicos; dos réplicas no garantizan el doble de capacidad.
- Riesgo: requests durante detección de caída pueden fallar. Mitigación: timeouts y reintento seguro por el cliente cuando corresponda.

## Cómo se verificará

En la etapa de balanceo: registrar instance_id técnico en logs/spans, emitir múltiples solicitudes y demostrar que ambas atendieron. Detener una instancia y comprobar retiro tras detección, continuidad en la otra y reincorporación. Ejecutar prueba concurrente del último cupo. No se agrega ni se declara balanceo real en la etapa 1.

## Fundamento de clase y elección práctica

Balanceo, round-robin, health checks y afinidad se explican en pp. 85–87. Traefik es la herramienta práctica adicional elegida para ejecutar ese diseño, no una herramienta explicada en el PDF. El equipo permite su uso por esa necesidad y utilidad, dejando esta justificación. La recomendación de verificaciones consecutivas de p. 87 queda pendiente de concretar con el equipo antes de implementar.
