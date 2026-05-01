$ErrorActionPreference = "Stop"

$rootDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$verifyScript = Join-Path $rootDir "verify-mga.ps1"
$server = Join-Path $rootDir "mga_server.exe"
$configPath = Join-Path $rootDir "config.json"

function Get-LocalAccessUrl {
    param([string]$ConfigPath)

    try {
        $config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
    } catch {
        return "http://127.0.0.1:8900"
    }

    $port = if ($config.PORT) { [string]$config.PORT } else { "8900" }
    $hostName = if ($config.LISTEN_IP) { ([string]$config.LISTEN_IP).Trim() } else { "127.0.0.1" }
    if ($hostName -eq "" -or $hostName -eq "0.0.0.0" -or $hostName -eq "::" -or $hostName -ieq "localhost") {
        $hostName = "127.0.0.1"
    }
    if ($hostName.Contains(":") -and -not $hostName.StartsWith("[")) {
        $hostName = "[$hostName]"
    }
    return "http://${hostName}:$port"
}

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
    Write-Host "Open $(Get-LocalAccessUrl -ConfigPath $configPath) in your browser once the server is up." -ForegroundColor DarkCyan
    & $server @args
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
