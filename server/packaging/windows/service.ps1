param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("install","uninstall","start","stop","restart","stop-tray")]
    [string]$Action,
    [string]$ServiceName = "MyGamesAnywhere",
    [string]$DisplayName = "MyGamesAnywhere",
    [string]$AppDir,
    [string]$DataDir,
    [string]$ConfigPath,
    [string]$LogPath
)

$ErrorActionPreference = "Stop"

$transcriptStarted = $false

function Write-InstallLog {
    param([string]$Message)

    if (-not $LogPath) {
        return
    }

    $timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ss.fffK"
    if ($script:transcriptStarted) {
        Write-Host "[$timestamp] $Message"
        return
    }

    Add-Content -Path $LogPath -Value "[$timestamp] $Message" -Encoding UTF8
}

if ($LogPath) {
    $logDir = Split-Path -Parent $LogPath
    if ($logDir) {
        New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    }
    Write-InstallLog "Starting service.ps1 Action=$Action ServiceName=$ServiceName AppDir=$AppDir DataDir=$DataDir ConfigPath=$ConfigPath"
    Start-Transcript -Path $LogPath -Append | Out-Null
    $transcriptStarted = $true
}

function Invoke-Native {
    param(
        [Parameter(Mandatory=$true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments
    )

    $output = & $FilePath @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($Arguments -join ' ') failed with exit code $LASTEXITCODE. Output: $($output -join [Environment]::NewLine)"
    }
    return $output
}

function Wait-ServiceOrNull {
    param(
        [string]$Name,
        [int]$Attempts = 10
    )

    for ($i = 0; $i -lt $Attempts; $i++) {
        $service = Get-ServiceOrNull -Name $Name
        if ($service) {
            return $service
        }
        Start-Sleep -Milliseconds 500
    }

    return $null
}

function Get-ServiceCimOrNull {
    param([string]$Name)

    try {
        return Get-CimInstance Win32_Service -Filter "Name='$Name'" -ErrorAction Stop
    } catch {
        return $null
    }
}

function Wait-ServiceStopped {
    param(
        [string]$Name,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $service = Get-ServiceOrNull -Name $Name
        if (-not $service) {
            Write-InstallLog "Service $Name no longer exists."
            return $true
        }

        $cim = Get-ServiceCimOrNull -Name $Name
        $processId = 0
        if ($cim -and $cim.ProcessId) {
            $processId = [int]$cim.ProcessId
        }

        if ($service.Status -eq 'Stopped') {
            if ($processId -le 0) {
                Write-InstallLog "Service $Name is stopped."
                return $true
            }
            $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
            if (-not $process) {
                Write-InstallLog "Service $Name is stopped and process $processId has exited."
                return $true
            }
        }

        Start-Sleep -Milliseconds 500
    }

    Write-ServiceDiagnostics -Name $Name
    return $false
}

function Write-FileTailToInstallLog {
    param(
        [string]$Path,
        [string]$Label,
        [int]$LineCount = 80
    )

    if (-not $Path) {
        return
    }
    if (-not (Test-Path -LiteralPath $Path)) {
        Write-InstallLog "$Label not found at $Path"
        return
    }

    Write-InstallLog "$Label tail from ${Path}:"
    Get-Content -LiteralPath $Path -Tail $LineCount -ErrorAction SilentlyContinue | ForEach-Object {
        Write-InstallLog "$Label> $_"
    }
}

function Write-ServiceDiagnostics {
    param([string]$Name)

    try {
        $svc = Get-CimInstance Win32_Service -Filter "Name='$Name'" -ErrorAction Stop
        if ($svc) {
            Write-InstallLog "SERVICE_DIAG Name=$($svc.Name) State=$($svc.State) Status=$($svc.Status) Started=$($svc.Started) ExitCode=$($svc.ExitCode) ServiceSpecificExitCode=$($svc.ServiceSpecificExitCode) ProcessId=$($svc.ProcessId)"
            Write-InstallLog "SERVICE_DIAG PathName=$($svc.PathName)"
            Write-InstallLog "SERVICE_DIAG StartName=$($svc.StartName)"
        }
    } catch {
        Write-InstallLog "SERVICE_DIAG failed: $($_.Exception.Message)"
    }

    try {
        $events = Get-WinEvent -FilterHashtable @{ LogName = 'System'; ProviderName = 'Service Control Manager'; StartTime = (Get-Date).AddMinutes(-10) } -MaxEvents 8 -ErrorAction Stop
        foreach ($event in $events) {
            Write-InstallLog "SCM_EVENT Id=$($event.Id) Time=$($event.TimeCreated) Message=$($event.Message)"
        }
    } catch {
        Write-InstallLog "SCM_EVENT read failed: $($_.Exception.Message)"
    }

    if ($DataDir) {
        Write-FileTailToInstallLog -Path (Join-Path $DataDir "mga_server_bootstrap.log") -Label "MGA_BOOTSTRAP_LOG"
        Write-FileTailToInstallLog -Path (Join-Path $DataDir "logs\mga_server.log") -Label "MGA_SERVER_LOG"
    }
}

function Get-ServiceOrNull {
    param([string]$Name)
    return Get-Service -Name $Name -ErrorAction SilentlyContinue
}

function Get-TargetTrayProcesses {
    if (-not $AppDir) {
        throw "AppDir is required to stop the tray companion."
    }

    $trayExe = (Join-Path $AppDir "mga_tray.exe").ToLowerInvariant()
    $targets = @()
    foreach ($process in Get-Process -Name "mga_tray" -ErrorAction SilentlyContinue) {
        try {
            $path = $process.MainModule.FileName
            if ($path -and $path.ToLowerInvariant() -eq $trayExe) {
                $targets += $process
            }
        } catch {
            Write-InstallLog "Skipping tray process PID $($process.Id); could not verify executable path: $($_.Exception.Message)"
        }
    }
    return $targets
}

function Stop-TrayProcesses {
    $targets = Get-TargetTrayProcesses
    foreach ($process in $targets) {
        Write-InstallLog "Stopping MGA tray companion PID $($process.Id)."
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }

    for ($i = 0; $i -lt 20; $i++) {
        if ((Get-TargetTrayProcesses).Count -eq 0) {
            Write-InstallLog "MGA tray companion is stopped."
            return
        }
        Start-Sleep -Milliseconds 500
    }

    throw "Timed out waiting for MGA tray companion to stop."
}

try {
    switch ($Action) {
        "stop-tray" {
            Stop-TrayProcesses
        }
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
                Write-InstallLog "Existing service found. Removing it before reinstall."
                try { Invoke-Native sc.exe stop $ServiceName | Out-Null } catch {}
                if (-not (Wait-ServiceStopped -Name $ServiceName -TimeoutSeconds 60)) {
                    throw "Timed out waiting for existing service $ServiceName to stop before reinstall."
                }
                Invoke-Native sc.exe delete $ServiceName | Out-Null
                Start-Sleep -Seconds 2
            }
            $binPath = "`"$serverExe`" --service --no-tray --app-dir `"$AppDir`" --data-dir `"$DataDir`" --config `"$ConfigPath`""
            Write-InstallLog "Creating service with BinaryPathName=$binPath"
            New-Service -Name $ServiceName -BinaryPathName $binPath -DisplayName $DisplayName -StartupType Automatic -ErrorAction Stop | Out-Null
            Invoke-Native sc.exe description $ServiceName "MyGamesAnywhere local game library server" | Out-Null
            if (-not (Wait-ServiceOrNull -Name $ServiceName)) {
                throw "Service $ServiceName was not created."
            }
            Write-InstallLog "Service $ServiceName created."
        }
        "uninstall" {
            $existing = Get-ServiceOrNull -Name $ServiceName
            if ($existing) {
                Write-InstallLog "Removing service $ServiceName."
                try { Invoke-Native sc.exe stop $ServiceName | Out-Null } catch {}
                if (-not (Wait-ServiceStopped -Name $ServiceName -TimeoutSeconds 60)) {
                    throw "Timed out waiting for service $ServiceName to stop before uninstall."
                }
                Invoke-Native sc.exe delete $ServiceName | Out-Null
            }
        }
        "start" {
            Write-InstallLog "Starting service $ServiceName."
            try {
                Start-Service -Name $ServiceName -ErrorAction Stop
            } catch {
                Write-ServiceDiagnostics -Name $ServiceName
                throw
            }
        }
        "stop" {
            Write-InstallLog "Stopping service $ServiceName."
            Stop-Service -Name $ServiceName -ErrorAction SilentlyContinue
            if (-not (Wait-ServiceStopped -Name $ServiceName -TimeoutSeconds 60)) {
                throw "Timed out waiting for service $ServiceName to stop."
            }
        }
        "restart" {
            Write-InstallLog "Restarting service $ServiceName."
            Restart-Service -Name $ServiceName -Force -ErrorAction Stop
        }
    }
} catch {
    Write-InstallLog "ERROR: $($_.Exception.Message)"
    Write-InstallLog "ERROR_DETAILS: $($_ | Out-String)"
    throw
} finally {
    if ($transcriptStarted) {
        Stop-Transcript | Out-Null
    }
}
