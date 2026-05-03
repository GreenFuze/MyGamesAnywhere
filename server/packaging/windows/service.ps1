param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("install","uninstall","start","stop","restart")]
    [string]$Action,
    [string]$ServiceName = "MyGamesAnywhere",
    [string]$DisplayName = "MyGamesAnywhere",
    [string]$AppDir,
    [string]$DataDir,
    [string]$ConfigPath
)

$ErrorActionPreference = "Stop"

function Get-ServiceOrNull {
    param([string]$Name)
    return Get-Service -Name $Name -ErrorAction SilentlyContinue
}

switch ($Action) {
    "install" {
        if (-not $AppDir -or -not $DataDir -or -not $ConfigPath) {
            throw "AppDir, DataDir, and ConfigPath are required for service install."
        }
        $serverExe = Join-Path $AppDir "mga_server.exe"
        if (-not (Test-Path $serverExe)) {
            throw "Missing server executable: $serverExe"
        }
        $existing = Get-ServiceOrNull -Name $ServiceName
        if ($existing) {
            & sc.exe stop $ServiceName | Out-Null
            & sc.exe delete $ServiceName | Out-Null
            Start-Sleep -Seconds 2
        }
        $binPath = "`"$serverExe`" --service --no-tray --app-dir `"$AppDir`" --data-dir `"$DataDir`" --config `"$ConfigPath`""
        & sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= $DisplayName | Out-Null
        & sc.exe description $ServiceName "MyGamesAnywhere local game library server" | Out-Null
    }
    "uninstall" {
        $existing = Get-ServiceOrNull -Name $ServiceName
        if ($existing) {
            & sc.exe stop $ServiceName | Out-Null
            Start-Sleep -Seconds 2
            & sc.exe delete $ServiceName | Out-Null
        }
    }
    "start" {
        Start-Service -Name $ServiceName -ErrorAction Stop
    }
    "stop" {
        Stop-Service -Name $ServiceName -ErrorAction SilentlyContinue
    }
    "restart" {
        Restart-Service -Name $ServiceName -Force -ErrorAction Stop
    }
}
