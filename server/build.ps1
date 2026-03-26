param(
    # Skip npm/Vite when you only need Go/plugins (e.g. CI without Node).
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$RootDir = $PSScriptRoot
$BinDir  = Join-Path $RootDir "bin"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# --- Frontend → server/bin/frontend/dist (served by mga_server from cwd=bin, default FRONTEND_DIST) ---
$feDir = Join-Path $RootDir "frontend"
$fePkg = Join-Path $feDir "package.json"
if ((Test-Path $fePkg) -and -not $SkipFrontend) {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        throw "Node.js/npm is required to build server/frontend. Install Node or pass -SkipFrontend to build.ps1."
    }
    Write-Host "Building frontend (output will be copied to $BinDir\frontend\dist)..." -ForegroundColor Cyan
    Push-Location $feDir
    try {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "frontend build failed" }
    } finally { Pop-Location }
    $feOut = Join-Path $BinDir "frontend\dist"
    if (-not (Test-Path (Join-Path $feDir "dist\index.html"))) {
        throw "frontend build did not produce dist/index.html"
    }
    New-Item -ItemType Directory -Force -Path $feOut | Out-Null
    Copy-Item -Path (Join-Path $feDir "dist\*") -Destination $feOut -Recurse -Force
} elseif ((Test-Path $fePkg) -and $SkipFrontend) {
    Write-Host "Skipping frontend build (-SkipFrontend)." -ForegroundColor Yellow
}

# --- Server ---
Write-Host "Building server..." -ForegroundColor Cyan

$ext = if ($IsLinux -or $IsMacOS) { "" } else { ".exe" }
$serverBin = Join-Path $BinDir "mga_server$ext"

# Windows: Explorer .exe icon (separate from systray //go:embed — needs COFF .syso)
if ($env:OS -eq "Windows_NT") {
    $serverPkg = Join-Path $RootDir "cmd\server"
    $goarch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    $sysoName = "rsrc_windows_$goarch.syso"
    Write-Host "Embedding .exe icon ($goarch) -> cmd\server\$sysoName..." -ForegroundColor Cyan
    Push-Location $serverPkg
    try {
        go run github.com/akavel/rsrc@v0.10.2 -ico mga.ico -arch $goarch -o $sysoName
        if ($LASTEXITCODE -ne 0) { throw "rsrc failed (Windows .exe icon)" }
    } finally { Pop-Location }
}

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

    # Load build-time secrets from .secrets.env if present (e.g. client_id/secret via ldflags).
    $ldflags = ""
    $secretsFile = Join-Path $srcDir ".secrets.env"
    if (Test-Path $secretsFile) {
        $secretVars = @{}
        Get-Content $secretsFile | ForEach-Object {
            $line = $_.Trim()
            if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
                $eqIdx = $line.IndexOf("=")
                $key   = $line.Substring(0, $eqIdx).Trim()
                $val   = $line.Substring($eqIdx + 1).Trim()
                $secretVars[$key] = $val
            }
        }
        if ($secretVars["CLIENT_ID"] -and $secretVars["CLIENT_SECRET"]) {
            $cid = $secretVars["CLIENT_ID"]
            $csec = $secretVars["CLIENT_SECRET"]
            $ldflags = "-X main.builtinClientID=$cid -X main.builtinClientSecret=$csec"
            Write-Host "  Injecting build-time credentials from .secrets.env" -ForegroundColor DarkCyan
        }
    }

    $pluginGoMod = Join-Path $srcDir "go.mod"
    if (Test-Path $pluginGoMod) {
        # Standalone module: build from the plugin's own directory.
        Push-Location $srcDir
        try {
            if ($ldflags) {
                go build -ldflags $ldflags -o (Join-Path $outDir "$name$ext") .
            } else {
                go build -o (Join-Path $outDir "$name$ext") .
            }
            if ($LASTEXITCODE -ne 0) { throw "plugin build failed: $name" }
        } finally { Pop-Location }
    } else {
        # Sub-package of the server module: build from the server root.
        Push-Location $RootDir
        try {
            if ($ldflags) {
                go build -ldflags $ldflags -o (Join-Path $outDir "$name$ext") "./plugins/$name"
            } else {
                go build -o (Join-Path $outDir "$name$ext") "./plugins/$name"
            }
            if ($LASTEXITCODE -ne 0) { throw "plugin build failed: $name" }
        } finally { Pop-Location }
    }

    Copy-Item (Join-Path $srcDir "*.plugin.json") -Destination $outDir -Force

    # Copy config.json if present (plugin-local config, e.g. API keys).
    $configSrcFile = Join-Path $srcDir "config.json"
    if (Test-Path $configSrcFile) {
        $configDstFile = Join-Path $outDir "config.json"
        if (-not (Test-Path $configDstFile)) {
            Copy-Item $configSrcFile -Destination $configDstFile
        }
    }
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
