param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("stop-user","start-user")]
    [string]$Action,
    [Parameter(Mandatory=$true)]
    [string]$AppDir,
    [Parameter(Mandatory=$true)]
    [string]$DataDir,
    [Parameter(Mandatory=$true)]
    [string]$ConfigPath,
    [string]$LogPath,
    [int]$Pid = 0
)

$ErrorActionPreference = "Stop"

function Write-InstallLog {
    param([string]$Message)
    if (-not $LogPath) {
        return
    }
    $timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ss.fffK"
    Add-Content -Path $LogPath -Value "[$timestamp] $Message" -Encoding UTF8
}

function Quote-Arg {
    param([string]$Value)
    return '"' + ($Value -replace '"', '\"') + '"'
}

function Get-TargetServerProcesses {
    $serverExe = (Join-Path $AppDir "mga_server.exe").ToLowerInvariant()
    $candidates = @()
    if ($Pid -gt 0) {
        $process = Get-Process -Id $Pid -ErrorAction SilentlyContinue
        if ($process) {
            $candidates += $process
        }
    }
    $candidates += Get-Process -Name "mga_server" -ErrorAction SilentlyContinue
    $unique = @{}
    foreach ($process in $candidates) {
        if ($unique.ContainsKey($process.Id)) {
            continue
        }
        try {
            $path = $process.MainModule.FileName
            if ($path -and $path.ToLowerInvariant() -eq $serverExe) {
                $unique[$process.Id] = $process
            }
        } catch {
            if ($Pid -gt 0 -and $process.Id -eq $Pid) {
                $unique[$process.Id] = $process
            }
        }
    }
    return $unique.Values
}

switch ($Action) {
    "stop-user" {
        Write-InstallLog "Stopping user-mode MGA server. AppDir=$AppDir Pid=$Pid"
        foreach ($process in Get-TargetServerProcesses) {
            Write-InstallLog "Stopping MGA server PID $($process.Id)."
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        }
        for ($i = 0; $i -lt 30; $i++) {
            if ((Get-TargetServerProcesses).Count -eq 0) {
                exit 0
            }
            Start-Sleep -Seconds 1
        }
        throw "Timed out waiting for user-mode MGA server to stop."
    }
    "start-user" {
        $serverExe = Join-Path $AppDir "mga_server.exe"
        if (-not (Test-Path -LiteralPath $serverExe)) {
            throw "Missing server executable: $serverExe"
        }
        New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
        $arguments = @(
            "--app-dir", (Quote-Arg $AppDir),
            "--data-dir", (Quote-Arg $DataDir),
            "--config", (Quote-Arg $ConfigPath),
            "--runtime-mode", "user"
        ) -join " "
        Write-InstallLog "Starting user-mode MGA server."
        Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $AppDir -WindowStyle Hidden
    }
}
