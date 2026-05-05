param(
    [Parameter(Mandatory=$true)][string]$AppDir,
    [Parameter(Mandatory=$true)][string]$DataDir,
    [ValidateSet("local","lan")][string]$ListenMode = "local",
    [ValidateSet("user","machine","service")][string]$InstallType = "user",
    [switch]$PreserveExistingNetwork
)

$ErrorActionPreference = "Stop"

$listenIP = if ($ListenMode -eq "lan") { "0.0.0.0" } else { "127.0.0.1" }
$configPath = Join-Path $DataDir "config.json"

function Set-JsonProperty {
    param(
        [Parameter(Mandatory=$true)][psobject]$Object,
        [Parameter(Mandatory=$true)][string]$Name,
        [Parameter(Mandatory=$true)][AllowEmptyString()][string]$Value
    )

    if ($Object.PSObject.Properties[$Name]) {
        $Object.$Name = $Value
    } else {
        $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $Value
    }
}

function Write-JsonFile {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [Parameter(Mandatory=$true)][psobject]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 8
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $json + [Environment]::NewLine, $encoding)
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $DataDir "data") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $DataDir "media") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $DataDir "source_cache") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $DataDir "updates") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $DataDir "logs") | Out-Null

if (Test-Path $configPath) {
    $existing = Get-Content $configPath -Raw | ConvertFrom-Json
    if (-not $PreserveExistingNetwork) {
        Set-JsonProperty -Object $existing -Name "LISTEN_IP" -Value $listenIP
    } elseif (-not $existing.PSObject.Properties["LISTEN_IP"]) {
        Set-JsonProperty -Object $existing -Name "LISTEN_IP" -Value $listenIP
    }
    Set-JsonProperty -Object $existing -Name "PORT" -Value ($(if ($existing.PSObject.Properties["PORT"]) { [string]$existing.PORT } else { "8900" }))
    Set-JsonProperty -Object $existing -Name "DB_PATH" -Value ($(if ($existing.PSObject.Properties["DB_PATH"]) { [string]$existing.DB_PATH } else { Join-Path $DataDir "data\db.sqlite" }))
    Set-JsonProperty -Object $existing -Name "MEDIA_ROOT" -Value ($(if ($existing.PSObject.Properties["MEDIA_ROOT"]) { [string]$existing.MEDIA_ROOT } else { Join-Path $DataDir "media" }))
    Set-JsonProperty -Object $existing -Name "SOURCE_CACHE_ROOT" -Value ($(if ($existing.PSObject.Properties["SOURCE_CACHE_ROOT"]) { [string]$existing.SOURCE_CACHE_ROOT } else { Join-Path $DataDir "source_cache" }))
    Set-JsonProperty -Object $existing -Name "UPDATES_DIR" -Value ($(if ($existing.PSObject.Properties["UPDATES_DIR"]) { [string]$existing.UPDATES_DIR } else { Join-Path $DataDir "updates" }))
    Set-JsonProperty -Object $existing -Name "LOG_FILE" -Value ($(if ($existing.PSObject.Properties["LOG_FILE"]) { [string]$existing.LOG_FILE } else { Join-Path $DataDir "logs\mga_server.log" }))
    Set-JsonProperty -Object $existing -Name "LOG_MAX_SIZE_MB" -Value ($(if ($existing.PSObject.Properties["LOG_MAX_SIZE_MB"]) { [string]$existing.LOG_MAX_SIZE_MB } else { "50" }))
    Set-JsonProperty -Object $existing -Name "LOG_MAX_BACKUPS" -Value ($(if ($existing.PSObject.Properties["LOG_MAX_BACKUPS"]) { [string]$existing.LOG_MAX_BACKUPS } else { "5" }))
    Set-JsonProperty -Object $existing -Name "APP_INSTALL_TYPE" -Value $InstallType
    Set-JsonProperty -Object $existing -Name "PLUGINS_DIR" -Value (Join-Path $AppDir "plugins")
    Set-JsonProperty -Object $existing -Name "FRONTEND_DIST" -Value (Join-Path $AppDir "frontend\dist")
    Set-JsonProperty -Object $existing -Name "UPDATE_MANIFEST_URL" -Value ($(if ($existing.PSObject.Properties["UPDATE_MANIFEST_URL"]) { [string]$existing.UPDATE_MANIFEST_URL } else { "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-update.json" }))
    Write-JsonFile -Path $configPath -Value $existing
    exit 0
}

$config = [ordered]@{
    PORT = "8900"
    LISTEN_IP = $listenIP
    DB_PATH = Join-Path $DataDir "data\db.sqlite"
    PLUGINS_DIR = Join-Path $AppDir "plugins"
    FRONTEND_DIST = Join-Path $AppDir "frontend\dist"
    MEDIA_ROOT = Join-Path $DataDir "media"
    SOURCE_CACHE_ROOT = Join-Path $DataDir "source_cache"
    UPDATES_DIR = Join-Path $DataDir "updates"
    LOG_FILE = Join-Path $DataDir "logs\mga_server.log"
    LOG_MAX_SIZE_MB = "50"
    LOG_MAX_BACKUPS = "5"
    APP_INSTALL_TYPE = $InstallType
    UPDATE_MANIFEST_URL = "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download/mga-update.json"
}

Write-JsonFile -Path $configPath -Value $config
