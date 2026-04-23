$ErrorActionPreference = "Stop"

$rootDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$verifyScript = Join-Path $rootDir "verify-mga.ps1"
$server = Join-Path $rootDir "mga_server.exe"

if (-not (Test-Path $verifyScript)) {
    Write-Error "Missing verify script: $verifyScript"
    exit 1
}

if (-not (Test-Path $server)) {
    Write-Error "Missing MGA server binary: $server"
    exit 1
}

& $verifyScript
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Push-Location $rootDir
try {
    Write-Host "Starting MGA..." -ForegroundColor Cyan
    Write-Host "Open http://127.0.0.1:8080 in your browser once the server is up." -ForegroundColor DarkCyan
    & $server @args
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
