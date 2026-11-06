# Revisión y trazabilidad del brief

Revisión local del 06/11/2026. Original conservado en Downloads; la versión Word
revisada se entrega en esta carpeta. El DOCX original no contiene imágenes
embebidas: no se verificaron visualmente el logo, Instagram ni las fotografías.

| Fuente | Verificación |
|---|---|
| Brief_Diseno_Web_Metal_Fitness_Codex (1).docx | SHA256: 119037506A1F810F19A55BE95544558EEE4D586EB293DFBBE5518FE1A70EE6B4 |
| Teórico EN PDF.pdf | 149 páginas; SHA256: 26C26AE19A8AA3AC1181BFD1D389E3357B6F667C60976635EC69953AE95F8828 |

Se extrajo texto del PDF para revisar los conceptos y sus páginas, sin incorporar
su texto completo al repositorio. La teoría fundamenta contratos, responsabilidades,
idempotencia, estados de resultado desconocido y recuperación. La estética y el
storyboard son propuestas propias. Las fuentes complementarias de accesibilidad y
rendimiento se enumeran en la sección 17 del brief.

Contrastes calculados mediante luminancia relativa sRGB: negro #080A0A con lima
#A7DA00 = 11,98:1; blanco con ese lima = 1,66:1; gris #BFC4C3 sobre #141719 =
10,20:1. Por eso se propone texto oscuro sobre CTA lima. Estos pares no prueban
conformidad de la interfaz completa: faltan estados, foco, tamaños y pruebas reales.

El storyboard SVG es una lámina estática conceptual; no es una animación ya
implementada. El Word se genera desde Markdown con validación estructural del
paquete XML/ZIP, conservando las otras partes del documento fuente; su paginación
y aspecto final en Microsoft Word requieren inspección visual.

Verificación realizada: el Word revisado coincide con su fuente Markdown y las
17 partes del paquete distintas de word/document.xml permanecen idénticas al
original. Los SHA256 de ambos documentos de Downloads siguen coincidiendo con
los registrados arriba. SVG validado como XML; no se ejecutó una animación.

Regeneración desde la raíz, sin modificar el documento original:

```powershell
& ([scriptblock]::Create((Get-Content scripts/build-design-brief.ps1 -Raw -Encoding UTF8))) -SourceDocument 'C:\Users\marti\Downloads\Brief_Diseno_Web_Metal_Fitness_Codex (1).docx'
& ([scriptblock]::Create((Get-Content scripts/build-design-brief.ps1 -Raw -Encoding UTF8))) -SourceDocument 'C:\Users\marti\Downloads\Brief_Diseno_Web_Metal_Fitness_Codex (1).docx' -Check
```
