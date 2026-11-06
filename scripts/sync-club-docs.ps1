param([string]$RepoRoot = (Get-Location).Path, [switch]$Check)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$wNamespace = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
$utf8 = [System.Text.UTF8Encoding]::new($false)

function Read-RepoText([string]$RelativePath) {
    return [System.IO.File]::ReadAllText((Join-Path $RepoRoot $RelativePath), $utf8)
}
function Section([string]$Text, [string]$Start, [string]$End) {
    $startAt = $Text.IndexOf($Start, [StringComparison]::Ordinal)
    if ($startAt -lt 0) { throw "Falta seccion: $Start" }
    $endAt = $Text.Length
    if ($End) { $endAt = $Text.IndexOf($End, $startAt + $Start.Length, [StringComparison]::Ordinal) }
    if ($endAt -lt 0) { throw "Falta fin de seccion: $End" }
    return $Text.Substring($startAt, $endAt - $startAt)
}
function Plain([string]$Text) {
    return ($Text.Replace('**','').Replace([string][char]96,'')).Trim()
}
function Table-Rows([string]$Text) {
    foreach ($line in ($Text -split '\r?\n')) {
        if ($line -notmatch '^\|' -or $line -match '^\|\s*:?-+') { continue }
        $cells = @($line.Trim().Trim('|').Split('|') | ForEach-Object { Plain $_ })
        [pscustomobject]@{ Cells = $cells }
    }
}
function W-Node([string]$Name) { return ,$wordXml.CreateElement('w',$Name,$wNamespace) }
function W-Attribute($Node,[string]$Name,[string]$Value) {
    $attribute = $wordXml.CreateAttribute('w',$Name,$wNamespace)
    $attribute.Value = $Value
    [void]$Node.Attributes.Append($attribute)
}
function Paragraph([string]$Text,[bool]$Heading=$false) {
    $paragraph = W-Node 'p'
    if ($Heading) { $properties=W-Node 'pPr'; $style=W-Node 'pStyle'; W-Attribute $style 'val' 'Heading2'; [void]$properties.AppendChild($style); [void]$paragraph.AppendChild($properties) }
    $run=W-Node 'r'; $textNode=W-Node 't'; $textNode.InnerText=(Plain $Text)
    [void]$textNode.SetAttribute('space','http://www.w3.org/XML/1998/namespace','preserve')
    [void]$run.AppendChild($textNode); [void]$paragraph.AppendChild($run)
    return ,$paragraph
}
function New-Table($Rows) {
    $table=W-Node 'tbl'; $properties=W-Node 'tblPr'; $borders=W-Node 'tblBorders'
    foreach($edge in @('top','left','bottom','right','insideH','insideV')) { $border=W-Node $edge; W-Attribute $border 'val' 'single'; W-Attribute $border 'sz' '4'; W-Attribute $border 'color' 'CBD5E1'; [void]$borders.AppendChild($border) }
    [void]$properties.AppendChild($borders); [void]$table.AppendChild($properties)
    $grid=W-Node 'tblGrid'; $width=[string][int](9000/$Rows[0].Cells.Count)
    foreach($value in $Rows[0].Cells) { $column=W-Node 'gridCol'; W-Attribute $column 'w' $width; [void]$grid.AppendChild($column) }; [void]$table.AppendChild($grid)
    $rowIndex=0
    foreach($record in $Rows) {
        if($record.Cells.Count -ne $Rows[0].Cells.Count) { throw 'Tabla Markdown con columnas incompatibles' }
        $row=W-Node 'tr'
        if($rowIndex -eq 0) { $rowProperties=W-Node 'trPr'; [void]$rowProperties.AppendChild((W-Node 'tblHeader')); [void]$row.AppendChild($rowProperties) }
        foreach($value in $record.Cells) { $cell=W-Node 'tc'; $cellProperties=W-Node 'tcPr'; $cellWidth=W-Node 'tcW'; W-Attribute $cellWidth 'w' $width; W-Attribute $cellWidth 'type' 'dxa'; [void]$cellProperties.AppendChild($cellWidth); [void]$cell.AppendChild($cellProperties); [void]$cell.AppendChild((Paragraph $value)); [void]$row.AppendChild($cell) }
        [void]$table.AppendChild($row); $rowIndex++
    }
    return ,$table
}
function Replace-Table($Table,$Rows) {
    $replacement=New-Table $Rows
    [void]$Table.ParentNode.ReplaceChild($replacement,$Table)
}
function Append-Markdown($Parent,[string]$Text) {
    $lines=@($Text -split '\r?\n'); $position=0
    while($position -lt $lines.Count) {
        $line=$lines[$position]
        if($line -match '^\|') {
            $tableLines=[System.Collections.Generic.List[string]]::new()
            while($position -lt $lines.Count -and $lines[$position] -match '^\|') { $tableLines.Add($lines[$position]); $position++ }
            $rows=@(Table-Rows ($tableLines -join "`n")); if($rows.Count -gt 0) { [void]$Parent.AppendChild((New-Table $rows)) }; continue
        }
        if(-not $line.Trim()) { $position++; continue }
        if($line -match '^#') { [void]$Parent.AppendChild((Paragraph ($line -replace '^#+\s*','') $true)); $position++; continue }
        $paragraphLines=[System.Collections.Generic.List[string]]::new()
        while($position -lt $lines.Count -and $lines[$position].Trim() -and $lines[$position] -notmatch '^(#|\|)') { $paragraphLines.Add($lines[$position].Trim()); $position++ }
        [void]$Parent.AppendChild((Paragraph ($paragraphLines -join ' ')))
    }
}
function Rendered-Text($Document,$Manager) { return (@($Document.SelectNodes('//w:body//w:t',$Manager) | ForEach-Object { $_.InnerText }) -join [char]31) }

$decisions=Read-RepoText 'docs/DECISIONES.md'
$confirmed=@(Table-Rows (Section $decisions '## Decisiones confirmadas' '### Decisión técnica local pendiente'))
# El bloque confirmado contiene dos tablas: conservar un solo encabezado.
$confirmed=@($confirmed | Where-Object { $_.Cells[0] -match '^D-\d+$' })
$confirmed=@([pscustomobject]@{Cells=@('ID','Tema','Decisión')}) + $confirmed
$replaced=@(Table-Rows (Section $decisions '## Decisiones reemplazadas' '## Historial de cambios'))
$history=@(Table-Rows (Section $decisions '## Historial de cambios' ''))
$theory=@(Table-Rows (Section $decisions '## Justificación teórica' '## Decisiones reemplazadas'))
$pending=Section $decisions '### Decisión técnica local pendiente' '## Justificación teórica'
$confirmedIds=@($confirmed | Where-Object { $_.Cells[0] -match '^D-\d+$' } | ForEach-Object { $_.Cells[0] })
$replacedIds=@($replaced | Where-Object { $_.Cells[0] -match '^D-\d+$' } | ForEach-Object { $_.Cells[0] })
if(@($confirmedIds | Group-Object | Where-Object Count -gt 1).Count -gt 0) { throw 'IDs confirmados duplicados' }
if(@($replacedIds | Group-Object | Where-Object Count -gt 1).Count -gt 0) { throw 'IDs reemplazados duplicados' }
if(@($confirmedIds | Where-Object { $replacedIds -contains $_ }).Count -gt 0) { throw 'Una decision esta confirmada y reemplazada a la vez' }
$plans=@(
    @{Path='Registro_de_Decisiones_Gimnasio.docx'; Kind='register'; Markdown=$pending},
    @{Path='Claude outputs/Plan_de_Etapas_Gimnasio_v2.docx'; Kind='plan'; Markdown=(Read-RepoText 'docs/PLAN_CLUB_BENEFICIOS.md')},
    @{Path='Claude outputs/Prompts_por_Etapa_Gimnasio_v2.docx'; Kind='prompts'; Markdown=(Read-RepoText 'docs/PROMPTS_CLUB_BENEFICIOS.md')}
)

foreach($plan in $plans) {
    $target=Join-Path $RepoRoot $plan.Path
    $archive=[System.IO.Compression.ZipFile]::OpenRead($target)
    try { $reader=[System.IO.StreamReader]::new($archive.GetEntry('word/document.xml').Open()); try { $originalXml=$reader.ReadToEnd() } finally { $reader.Dispose() } } finally { $archive.Dispose() }
    $wordXml=[System.Xml.XmlDocument]::new(); $wordXml.PreserveWhitespace=$true; $wordXml.LoadXml($originalXml)
    $ns=[System.Xml.XmlNamespaceManager]::new($wordXml.NameTable); $ns.AddNamespace('w',$wNamespace)
    $before=Rendered-Text $wordXml $ns
    $body=$wordXml.SelectSingleNode('//w:body',$ns)
    foreach($previous in @($body.SelectNodes('w:sdt[w:sdtPr/w:tag[@w:val="club-amendment-20261106"]]',$ns))) { [void]$body.RemoveChild($previous) }
    foreach($previous in @($body.SelectNodes('w:sdt[w:sdtPr/w:tag[@w:val="club-notice-20261106"]]',$ns))) { [void]$body.RemoveChild($previous) }
    if($plan.Kind -eq 'register') {
        $tables=@($body.SelectNodes('w:tbl',$ns)); if($tables.Count -ne 4) { throw 'Registro Word: se esperaban las cuatro tablas originales' }
        Replace-Table $tables[0] $confirmed; Replace-Table $tables[1] $replaced
        Replace-Table $tables[2] $history; Replace-Table $tables[3] $theory
        foreach($paragraph in @($body.SelectNodes('w:p',$ns))) { if($paragraph.InnerText.StartsWith('Última actualización:')) { [void]$body.ReplaceChild((Paragraph 'Última actualización: 6 de noviembre de 2026'),$paragraph) } }
    }
    $notice=W-Node 'sdt'; $noticeProperties=W-Node 'sdtPr'; $noticeTag=W-Node 'tag'; W-Attribute $noticeTag 'val' 'club-notice-20261106'; [void]$noticeProperties.AppendChild($noticeTag); [void]$notice.AppendChild($noticeProperties); $noticeContent=W-Node 'sdtContent'; [void]$notice.AppendChild($noticeContent)
    [void]$noticeContent.AppendChild((Paragraph 'Actualización vigente del Club — 6 de noviembre de 2026' $true))
    [void]$noticeContent.AppendChild((Paragraph 'D-63 a D-69 amplían el alcance; D-70 registra seguridad corregida localmente en CB-03. D-71 solicita diseño futuro; D-72/73/74 documentan propuestas e implementación para revisión. CB-02/03 tienen validación local, sin cerrar el Club completo. La adenda final prevalece para el Club sobre las restricciones anteriores. No equivale a aprobación técnica ni despliegue. Las etapas académicas, las del Word v2, las CB y las FD conservan sus identificadores distintos.'))
    $paragraphs=@($body.SelectNodes('w:p',$ns)); [void]$body.InsertBefore($notice,$paragraphs[[Math]::Min(3,$paragraphs.Count-1)])
    $block=W-Node 'sdt'; $properties=W-Node 'sdtPr'; $tag=W-Node 'tag'; W-Attribute $tag 'val' 'club-amendment-20261106'; [void]$properties.AppendChild($tag); [void]$block.AppendChild($properties); $content=W-Node 'sdtContent'; [void]$block.AppendChild($content)
    [void]$content.AppendChild((Paragraph 'Adenda del Club — revisión local del 06/11/2026' $true))
    Append-Markdown $content $plan.Markdown
    [void]$content.AppendChild((Paragraph 'Referencias vigentes: docs/DECISIONES.md; docs/SPEC_CLUB_BENEFICIOS.md; docs/PLAN_CLUB_BENEFICIOS.md; docs/PROMPTS_CLUB_BENEFICIOS.md; docs/CB02_VALIDACION.md; docs/CB03_VALIDACION.md; docs/PLAN_DISENO_FRONTEND.md; ADR-016/017/018/019. CB-02 pasó PostgreSQL, vet y Spectral; CB-03 pasó JWT/roles/bcrypt, PostgreSQL/SMTP local, tests/vet y build. Siguen CB-04, revisión del equipo, configuración y validación productiva.'))
    $sectionProperties=$body.SelectSingleNode('w:sectPr',$ns)
    if($sectionProperties) { [void]$body.InsertBefore($block,$sectionProperties) } else { [void]$body.AppendChild($block) }
    $after=Rendered-Text $wordXml $ns
    if($Check) { if($before -cne $after) { throw "Word desactualizado: $($plan.Path)" }; Write-Output "Sincronizado: $($plan.Path)"; continue }
    if($before -ceq $after) { Write-Output "Sin cambios: $($plan.Path)"; continue }
    $backup=Join-Path ([System.IO.Path]::GetTempPath()) (([System.IO.Path]::GetFileNameWithoutExtension($target))+'-antes-club-'+[guid]::NewGuid().ToString('N')+'.docx')
    [System.IO.File]::Copy($target,$backup,$false)
    $temporary=$target+'.'+[guid]::NewGuid().ToString('N')+'.tmp'
    try {
        [System.IO.File]::Copy($target,$temporary,$false)
        $updated=[System.IO.Compression.ZipFile]::Open($temporary,[System.IO.Compression.ZipArchiveMode]::Update)
        try { $updated.GetEntry('word/document.xml').Delete(); $entry=$updated.CreateEntry('word/document.xml'); $settings=[System.Xml.XmlWriterSettings]::new(); $settings.Encoding=$utf8; $writer=[System.Xml.XmlWriter]::Create($entry.Open(),$settings); try { $wordXml.Save($writer) } finally { $writer.Dispose() } } finally { $updated.Dispose() }
        # Validar el paquete resultante antes de reemplazar el archivo original.
        $verify=[System.IO.Compression.ZipFile]::OpenRead($temporary)
        try { $reader=[System.IO.StreamReader]::new($verify.GetEntry('word/document.xml').Open()); try { $verifiedXml=[System.Xml.XmlDocument]::new(); $verifiedXml.LoadXml($reader.ReadToEnd()) } finally { $reader.Dispose() }; if(-not $verify.GetEntry('[Content_Types].xml') -or -not $verify.GetEntry('word/styles.xml')) { throw 'Paquete Word incompleto' } } finally { $verify.Dispose() }
        [System.IO.File]::Copy($temporary,$target,$true)
        Write-Output "Actualizado: $($plan.Path). Copia previa: $backup"
    } finally { if([System.IO.File]::Exists($temporary)) { [System.IO.File]::Delete($temporary) } }
}
