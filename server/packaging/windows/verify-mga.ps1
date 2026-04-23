$ErrorActionPreference = "Stop"

function Fail {
    param([string]$Message)
    Write-Error $Message
    exit 1
}

function Warn {
    param([string]$Message)
    Write-Warning $Message
}

function Require-Path {
    param([string]$Path, [string]$Description)
    if (-not (Test-Path $Path)) {
        Fail "Missing $Description at $Path"
    }
}

function Get-ConfiguredPort {
    param([string]$ConfigPath)

    try {
        $config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
    } catch {
        Fail "Failed to parse config.json: $($_.Exception.Message)"
    }

    if (-not $config.PORT) {
        Fail "config.json is missing PORT."
    }

    $port = 0
    if (-not [int]::TryParse([string]$config.PORT, [ref]$port)) {
        Fail "config.json PORT must be numeric. Got '$($config.PORT)'."
    }

    if ($port -lt 1 -or $port -gt 65535) {
        Fail "config.json PORT must be between 1 and 65535. Got '$port'."
    }

    return $port
}

function Test-PortAvailable {
    param([int]$Port)

    $listeners = [System.Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners()
    foreach ($listener in $listeners) {
        if ($listener.Port -eq $Port) {
            return $false
        }
    }
    return $true
}

$rootDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$configPath = Join-Path $rootDir "config.json"
$serverPath = Join-Path $rootDir "mga_server.exe"
$pluginsPath = Join-Path $rootDir "plugins"
$frontendIndex = Join-Path $rootDir "frontend\dist\index.html"

Require-Path $configPath "config.json"
Require-Path $serverPath "mga_server.exe"
Require-Path $pluginsPath "plugins directory"
Require-Path $frontendIndex "frontend dist index.html"

if ($rootDir -match '\\Program Files( \(x86\))?(\\|$)') {
    Warn "MGA is running from Program Files. Portable MGA expects a writable folder. Move it to a user-writable location if startup or data writes fail."
}

foreach ($dirName in @("data", "media")) {
    $dirPath = Join-Path $rootDir $dirName
    if (-not (Test-Path $dirPath)) {
        New-Item -ItemType Directory -Force -Path $dirPath | Out-Null
    }
}

$writeProbe = Join-Path $rootDir ".mga-write-test.tmp"
try {
    [System.IO.File]::WriteAllText($writeProbe, "ok")
    Remove-Item $writeProbe -Force -ErrorAction SilentlyContinue
} catch {
    Fail "MGA needs a writable runtime folder. Failed to write inside '$rootDir': $($_.Exception.Message)"
}

$port = Get-ConfiguredPort -ConfigPath $configPath
if (-not (Test-PortAvailable -Port $port)) {
    Fail "Port $port is already in use. Stop the process using it or change PORT in config.json before starting MGA."
}

Write-Host "MGA runtime verification passed." -ForegroundColor Green
Write-Host "Runtime folder: $rootDir"
Write-Host "Configured port: $port"
