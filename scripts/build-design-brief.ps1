param([Parameter(Mandatory=$true)][string]$SourceDocument, [switch]$Check)
$ErrorActionPreference='Stop'
$RepoRoot=(Get-Location).Path
Add-Type -AssemblyName System.IO.Compression.FileSystem
$wNamespace='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
$utf8=[Text.UTF8Encoding]::new($false)
# Reuse only helper declarations; never execute the synchronization script body.
$tokens=$null; $parseErrors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $RepoRoot 'scripts/sync-club-docs.ps1'),[ref]$tokens,[ref]$parseErrors)
if($parseErrors.Count) { throw 'Helper script parse error' }
$definitions=$ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst]},$false)
. ([scriptblock]::Create((($definitions | ForEach-Object { $_.Extent.Text }) -join "`n")))
$target=Join-Path $RepoRoot 'docs/diseno/Brief_Diseno_Web_Metal_Fitness_Codex_v2.docx'
if([IO.Path]::GetFullPath($SourceDocument) -eq [IO.Path]::GetFullPath($target)) { throw 'Source must differ from output' }
$sourceHash=(Get-FileHash -LiteralPath $SourceDocument -Algorithm SHA256).Hash
$archive=[IO.Compression.ZipFile]::OpenRead($SourceDocument)
try { $reader=[IO.StreamReader]::new($archive.GetEntry('word/document.xml').Open()); try { $original=$reader.ReadToEnd() } finally { $reader.Dispose() } } finally { $archive.Dispose() }
$wordXml=[xml]::new(); $wordXml.PreserveWhitespace=$true; $wordXml.LoadXml($original)
$ns=[Xml.XmlNamespaceManager]::new($wordXml.NameTable); $ns.AddNamespace('w',$wNamespace)
$body=$wordXml.SelectSingleNode('//w:body',$ns)
$section=$body.SelectSingleNode('w:sectPr',$ns)
foreach($child in @($body.ChildNodes)) { [void]$body.RemoveChild($child) }
$markdown=Read-RepoText 'docs/diseno/BRIEF_WEB_METAL_FITNESS_V2.md'
# Keep individual list items readable in Word without requiring numbering styles.
$markdown=[regex]::Replace($markdown,'(?m)^(?=(?:\d+\. |[-*] ))',"`r`n")
Append-Markdown $body $markdown
if($section) { [void]$body.AppendChild($section) }
$expected=Rendered-Text $wordXml $ns
if($Check) {
    $archive=[IO.Compression.ZipFile]::OpenRead($target)
    try { $reader=[IO.StreamReader]::new($archive.GetEntry('word/document.xml').Open()); try { $actual=[xml]$reader.ReadToEnd() } finally { $reader.Dispose() } } finally { $archive.Dispose() }
    $manager=[Xml.XmlNamespaceManager]::new($actual.NameTable); $manager.AddNamespace('w',$wNamespace)
    if((Rendered-Text $actual $manager) -cne $expected) { throw 'Design Word is outdated' }
    Write-Output 'Design Word synchronized'; return
}
$temporary=Join-Path ([IO.Path]::GetTempPath()) ('metal-design-'+[guid]::NewGuid().ToString('N')+'.docx')
try {
    [IO.File]::Copy($SourceDocument,$temporary,$false)
    $archive=[IO.Compression.ZipFile]::Open($temporary,[IO.Compression.ZipArchiveMode]::Update)
    try {
        $archive.GetEntry('word/document.xml').Delete()
        $entry=$archive.CreateEntry('word/document.xml')
        $settings=[Xml.XmlWriterSettings]::new(); $settings.Encoding=$utf8
        $writer=[Xml.XmlWriter]::Create($entry.Open(),$settings)
        try { $wordXml.Save($writer) } finally { $writer.Dispose() }
    } finally { $archive.Dispose() }
    $verify=[IO.Compression.ZipFile]::OpenRead($temporary)
    try { $reader=[IO.StreamReader]::new($verify.GetEntry('word/document.xml').Open()); try { $validated=[xml]$reader.ReadToEnd() } finally { $reader.Dispose() }; if(-not $verify.GetEntry('word/styles.xml')) { throw 'Missing Word styles' } } finally { $verify.Dispose() }
    if((Get-FileHash -LiteralPath $SourceDocument -Algorithm SHA256).Hash -ne $sourceHash) { throw 'Source changed during generation' }
    if(Test-Path -LiteralPath $target) { [IO.File]::Copy($target,(Join-Path ([IO.Path]::GetTempPath()) ('metal-design-before-'+[guid]::NewGuid().ToString('N')+'.docx')),$false) }
    [IO.File]::Copy($temporary,$target,$true)
    Write-Output "Created: $target"
} finally { if([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) } }
