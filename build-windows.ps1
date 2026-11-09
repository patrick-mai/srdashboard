param(
    [string]$ProjectRoot = $PSScriptRoot,
    [string]$DistDir = "dist\windows-amd64",
    [switch]$SkipTests,
    [switch]$Clean
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Fail {
    param([string]$Message)
    Write-Host ""
    Write-Host "FEHLER: $Message" -ForegroundColor Red
    exit 1
}

function Invoke-Checked {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        Fail "$FilePath $($Arguments -join ' ') ist mit Exit-Code $LASTEXITCODE fehlgeschlagen."
    }
}

function Test-GoVersion {
    param(
        [int]$Major,
        [int]$Minor,
        [int]$RequiredMajor = 1,
        [int]$RequiredMinor = 24
    )

    if ($Major -gt $RequiredMajor) {
        return $true
    }

    if ($Major -lt $RequiredMajor) {
        return $false
    }

    return ($Minor -ge $RequiredMinor)
}

function Copy-PluginTree {
    param(
        [string]$SourceRoot,
        [string]$DestinationRoot
    )

    if (-not (Test-Path $SourceRoot)) {
        Fail "Plugin-Verzeichnis wurde nicht gefunden: $SourceRoot"
    }

    New-Item -ItemType Directory -Force -Path $DestinationRoot | Out-Null

    Get-ChildItem -Path $SourceRoot -Directory | ForEach-Object {
        $srcPlugin = $_.FullName
        $dstPlugin = Join-Path $DestinationRoot $_.Name

        New-Item -ItemType Directory -Force -Path $dstPlugin | Out-Null

        Get-ChildItem -Path $srcPlugin -Recurse -File | ForEach-Object {
            $relative = $_.FullName.Substring($srcPlugin.Length).TrimStart('\', '/')

            # Vorhandene Laufzeit-Konfiguration im dist-Ordner nicht überschreiben.
            if ($relative -ieq "config.xml") {
                return
            }

            $target = Join-Path $dstPlugin $relative
            $targetDir = Split-Path -Parent $target

            if ($targetDir) {
                New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
            }

            Copy-Item -Force $_.FullName $target
        }
    }
}

try {
    $ProjectRoot = (Resolve-Path $ProjectRoot).Path
} catch {
    Fail "Projektverzeichnis existiert nicht: $ProjectRoot"
}

Set-Location $ProjectRoot

Write-Host "SRDashboard Windows Build" -ForegroundColor Green
Write-Host "Projekt: $ProjectRoot"

if (-not (Test-Path (Join-Path $ProjectRoot "go.mod"))) {
    Fail "Keine go.mod gefunden. Starte das Skript im SRDashboard-Hauptverzeichnis."
}

Write-Step "Go-Installation prüfen"

$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCommand) {
    Fail "Go wurde nicht gefunden. Installiere Go 1.24 oder neuer und stelle sicher, dass 'go' im PATH liegt."
}

$goVersionText = (& go version)
Write-Host $goVersionText

if ($goVersionText -notmatch 'go(\d+)\.(\d+)(?:\.(\d+))?') {
    Fail "Go-Version konnte nicht erkannt werden."
}

$goMajor = [int]$Matches[1]
$goMinor = [int]$Matches[2]

if (-not (Test-GoVersion -Major $goMajor -Minor $goMinor)) {
    Fail "Go 1.24 oder neuer ist erforderlich. Gefunden wurde Go $goMajor.$goMinor."
}

$distPath = Join-Path $ProjectRoot $DistDir
$pluginsSource = Join-Path $ProjectRoot "plugins"
$pluginsTarget = Join-Path $distPath "plugins"
$exePath = Join-Path $distPath "srdashboard.exe"
$configSource = Join-Path $ProjectRoot "config.xml"

if ($Clean -and (Test-Path $distPath)) {
    Write-Step "Altes Dist-Verzeichnis löschen"
    Remove-Item -Recurse -Force $distPath
}

New-Item -ItemType Directory -Force -Path $distPath | Out-Null

if (-not $SkipTests) {
    Write-Step "Komplette Testsuite ausführen"
    Invoke-Checked -FilePath "go" -Arguments @("test", "./...")
} else {
    Write-Step "Tests übersprungen"
}

Write-Step "Windows AMD64 EXE bauen"

$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
$oldCgo = $env:CGO_ENABLED

try {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    Invoke-Checked -FilePath "go" -Arguments @(
        "build",
        "-trimpath",
        "-o", $exePath,
        "."
    )
}
finally {
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
    $env:CGO_ENABLED = $oldCgo
}

if (-not (Test-Path $exePath)) {
    Fail "Build lief ohne gemeldeten Fehler, aber srdashboard.exe wurde nicht gefunden."
}

Write-Step "Plugins nach dist kopieren"
Copy-PluginTree -SourceRoot $pluginsSource -DestinationRoot $pluginsTarget

$dartsManifest = Join-Path $pluginsTarget "darts\manifest.xml"
if (-not (Test-Path $dartsManifest)) {
    Fail "Das Dart-Plugin wurde nicht nach '$pluginsTarget\darts' kopiert."
}

Write-Step "config.xml bereitstellen"

$configTarget = Join-Path $distPath "config.xml"

if (-not (Test-Path $configTarget)) {
    if (Test-Path $configSource) {
        Copy-Item -Force $configSource $configTarget
        Write-Host "config.xml wurde aus dem Projektverzeichnis kopiert."
    } else {
        Write-Host "WARNUNG: Keine config.xml gefunden." -ForegroundColor Yellow
    }
} else {
    Write-Host "Vorhandene dist\windows-amd64\config.xml bleibt unverändert."
}

Write-Step "Installations-ZIP erstellen"

$zipName = "srdashboard-windows-amd64-darts.zip"
$zipPath = Join-Path $ProjectRoot "dist\$zipName"

if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}

$zipItems = Get-ChildItem -Path $distPath -Force
if (-not $zipItems) {
    Fail "Dist-Verzeichnis ist leer."
}

Compress-Archive -Path $zipItems.FullName -DestinationPath $zipPath -CompressionLevel Optimal -Force

Write-Step "Build abgeschlossen"

$exeInfo = Get-Item $exePath
$zipInfo = Get-Item $zipPath

Write-Host ""
Write-Host "EXE:" -ForegroundColor Green
Write-Host "  $($exeInfo.FullName)"
Write-Host "  $([math]::Round($exeInfo.Length / 1MB, 2)) MB"

Write-Host ""
Write-Host "Dart-Plugin:" -ForegroundColor Green
Write-Host "  $(Join-Path $pluginsTarget 'darts')"

Write-Host ""
Write-Host "Installations-ZIP:" -ForegroundColor Green
Write-Host "  $($zipInfo.FullName)"
Write-Host "  $([math]::Round($zipInfo.Length / 1MB, 2)) MB"

Write-Host ""
Write-Host "Fertig." -ForegroundColor Green
