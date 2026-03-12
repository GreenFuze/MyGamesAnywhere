$ErrorActionPreference = "Stop"
$RootDir = $PSScriptRoot
$BinDir  = Join-Path $RootDir "bin"

# --- Server ---
Write-Host "Building server..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

$ext = if ($IsLinux -or $IsMacOS) { "" } else { ".exe" }
$serverBin = Join-Path $BinDir "mga_server$ext"

Push-Location $RootDir
try {
    go build -o $serverBin ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "server build failed" }
} finally { Pop-Location }

# --- Plugins ---
# Auto-discover: every subdirectory under server/plugins that contains a main.go
$pluginsSrc = Join-Path $RootDir "plugins"
$pluginsOut = Join-Path $BinDir  "plugins"

Get-ChildItem -Path $pluginsSrc -Directory | ForEach-Object {
    $name    = $_.Name
    $srcDir  = $_.FullName
    $mainGo  = Join-Path $srcDir "main.go"

    if (-not (Test-Path $mainGo)) {
        Write-Host "  Skipping $name (no main.go)" -ForegroundColor DarkGray
        return
    }

    $outDir = Join-Path $pluginsOut $name
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null

    Write-Host "Building plugin: $name..." -ForegroundColor Cyan
    Push-Location $RootDir
    try {
        go build -o (Join-Path $outDir "$name$ext") "./plugins/$name"
        if ($LASTEXITCODE -ne 0) { throw "plugin build failed: $name" }
    } finally { Pop-Location }

    Copy-Item (Join-Path $srcDir "*.plugin.json") -Destination $outDir -Force
}

# --- OpenAPI spec ---
Write-Host "Generating openapi.yaml..." -ForegroundColor Cyan
Push-Location $RootDir
try {
    go run ./cmd/openapi-gen
    if ($LASTEXITCODE -ne 0) { throw "openapi generation failed" }
} finally { Pop-Location }

# --- Config ---
$configDst = Join-Path $BinDir "config.json"
$configSrc = Join-Path $RootDir "config.json.example"
if (-not (Test-Path $configDst) -and (Test-Path $configSrc)) {
    Write-Host "Seeding config.json from example" -ForegroundColor Yellow
    Copy-Item $configSrc -Destination $configDst
}

Write-Host "`nBuild complete -> $BinDir" -ForegroundColor Green
