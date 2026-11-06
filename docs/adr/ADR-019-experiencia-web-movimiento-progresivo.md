# ADR-019 — Experiencia web y movimiento progresivo

| Campo | Valor |
|---|---|
| Estado | Propuesto; documentación preparada, implementación y revisión pendientes |
| Fecha | 2026-11-06 |
| Autoría | Borrador para revisión del equipo METALFITNESS |
| Relacionado con | D-71 (pedido confirmado), D-72 (propuesta), ADR-006/017; D-70 sigue abierto |
| Etapas | FD-01–05; implementación futura en etapa 20A/B/C del plan Word |

## Contexto

El equipo solicita completar su brief con el teórico y definir movimiento de
entrada antes de llegar a la etapa de frontend. Existe una aplicación React,
TypeScript, Vite y CSS y flujos privados que deben conservarse. Las instrucciones
de implementación del documento aportado son material a revisar: este pedido
autoriza preparar el diseño, no iniciar ahora la implementación del frontend.

## Propuesta

Mantener la aplicación existente, sus componentes reutilizables y su cliente
HTTP hacia el gateway. Separar navegación pública y vistas privadas por audiencia;
los permisos y las reglas del Club se verifican en backend. No agregar BFF,
microfrontends, motor de física ni migración de framework para este efecto.

Prototipar una escena localizada en SVG/CSS: discos metálicos se alinean en una
barra y terminan estáticos. El texto y las acciones están disponibles desde el
inicio. Reservar espacio, usar transformaciones y opacidad y ofrecer alternativa
estática por movimiento reducido o fallo. Duración sugerida de 1,8 s y presupuestos
de assets son parámetros propios sujetos a prueba, no decisiones ratificadas.
El detalle visual, fuentes y colores pertenecen al brief, no son invariantes de arquitectura.

Las operaciones privadas deben representar procesando, confirmado, rechazado y
verificando resultado. Un timeout no implica rechazo ni habilita repetir con una
intención nueva. Conservar referencia y recuperar el resultado por contrato;
deshabilitar un botón no sustituye idempotencia en servidor.

## Alternativas

- Video real de entrenamiento: alternativa válida con autorización de imagen,
  compresión, poster, silencio y fallback; no combinar ambos efectos de entrada.
- WebGL o simulación física: complejidad y costo sin necesidad demostrada.
- Imagen estática: alternativa funcional obligatoria; puede ser el resultado
  elegido si el prototipo no cumple accesibilidad o rendimiento.

## Consecuencias y validación

No se agregan dependencias ni se cambia producción en esta etapa. FD-02 valida
wireframes y assets; FD-03 mide un prototipo antes de integrar; FD-04 conserva
flujos y contratos; FD-05 registra pruebas visuales, teclado, movimiento reducido,
recuperación de operaciones y rendimiento. CB-03 y su conciliación de seguridad
D-70 preceden a habilitar acceso real; las funciones pendientes no se simulan
como resultados confirmados. No se declara conformidad WCAG por redactar el brief.

## Fundamento

Teórico de 149 páginas: p. 23 (propiedad/API), pp. 90 y 97 (gateway y capas
justificadas), pp. 104 y 110 (intención estable y estados de frontend), p. 116
(fallback honesto), p. 118 (dependencias). El apunte no prescribe estética ni
duración de animaciones. Fuentes complementarias y condiciones verificables en
[brief](../diseno/BRIEF_WEB_METAL_FITNESS_V2.md), sección 17.
