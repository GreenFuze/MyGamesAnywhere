param(
    [Parameter(Mandatory=$true)]
    [string]$PlanPath
)

$ErrorActionPreference = "Stop"

function Write-UpdateLog {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ss.fffK"
    $line = "[$timestamp] $Message"
    Write-Host $line
    if ($script:LogPath) {
        Add-Content -Path $script:LogPath -Value $line -Encoding UTF8
    }
}

function Quote-Arg {
    param([string]$Value)
    return '"' + ($Value -replace '"', '\"') + '"'
}

function Wait-ServerExit {
    param([int]$ServerPID)
    if ($ServerPID -le 0) {
        return
    }
    $process = Get-Process -Id $ServerPID -ErrorAction SilentlyContinue
    if (-not $process) {
        return
    }
    Write-UpdateLog "Waiting for MGA server PID $ServerPID to exit."
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        $process = Get-Process -Id $ServerPID -ErrorAction SilentlyContinue
        if (-not $process) {
            Write-UpdateLog "MGA server exited."
            return
        }
    }
    Write-UpdateLog "MGA server did not exit in time; stopping PID $ServerPID."
    Stop-Process -Id $ServerPID -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

function Find-PackageRoot {
    param([string]$ExtractDir)
    $servers = Get-ChildItem -LiteralPath $ExtractDir -Recurse -Filter "mga_server.exe" -File
    foreach ($server in $servers) {
        $candidate = $server.Directory.FullName
        if ((Test-Path -LiteralPath (Join-Path $candidate "plugins")) -and
            (Test-Path -LiteralPath (Join-Path $candidate "frontend\dist\index.html"))) {
            return $candidate
        }
    }
    throw "Portable package is missing mga_server.exe, plugins, or frontend\dist\index.html."
}

function Copy-PortableAppFiles {
    param(
        [string]$PackageRoot,
        [string]$AppDir
    )
    $preserve = @{
        "config.json" = $true
        "data" = $true
        "media" = $true
        "source_cache" = $true
        "updates" = $true
        "logs" = $true
    }
    Get-ChildItem -LiteralPath $PackageRoot -Force | ForEach-Object {
        if ($preserve.ContainsKey($_.Name)) {
            Write-UpdateLog "Preserving mutable path $($_.Name)."
            return
        }
        $dest = Join-Path $AppDir $_.Name
        if ($_.PSIsContainer) {
            if (Test-Path -LiteralPath $dest) {
                Remove-Item -LiteralPath $dest -Recurse -Force
            }
            Copy-Item -LiteralPath $_.FullName -Destination $dest -Recurse -Force
        } else {
            Copy-Item -LiteralPath $_.FullName -Destination $dest -Force
        }
        Write-UpdateLog "Updated $($_.Name)."
    }
}

try {
    if (-not (Test-Path -LiteralPath $PlanPath)) {
        throw "Update plan not found: $PlanPath"
    }
    $plan = Get-Content -LiteralPath $PlanPath -Raw | ConvertFrom-Json
    $appDir = [string]$plan.app_dir
    $dataDir = [string]$plan.data_dir
    $configPath = [string]$plan.config_path
    $assetPath = [string]$plan.asset_path
    if (-not $appDir -or -not $dataDir -or -not $configPath -or -not $assetPath) {
        throw "Update plan is missing app_dir, data_dir, config_path, or asset_path."
    }

    $updatesDir = Join-Path $appDir "updates"
    New-Item -ItemType Directory -Force -Path $updatesDir | Out-Null
    $script:LogPath = Join-Path $updatesDir "mga_update.log"
    Write-UpdateLog "Starting portable update. AppDir=$appDir Asset=$assetPath"

    if (-not (Test-Path -LiteralPath $assetPath)) {
        throw "Portable update ZIP not found: $assetPath"
    }

    Wait-ServerExit -ServerPID ([int]$plan.server_pid)

    $extractDir = Join-Path $updatesDir "portable-extract"
    if (Test-Path -LiteralPath $extractDir) {
        Remove-Item -LiteralPath $extractDir -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    Write-UpdateLog "Extracting portable ZIP."
    Expand-Archive -LiteralPath $assetPath -DestinationPath $extractDir -Force
    $packageRoot = Find-PackageRoot -ExtractDir $extractDir
    Write-UpdateLog "Package root: $packageRoot"

    Copy-PortableAppFiles -PackageRoot $packageRoot -AppDir $appDir

    $serverExe = Join-Path $appDir "mga_server.exe"
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "Updated server executable is missing: $serverExe"
    }
    $arguments = @(
        "--app-dir", (Quote-Arg $appDir),
        "--data-dir", (Quote-Arg $dataDir),
        "--config", (Quote-Arg $configPath),
        "--runtime-mode", "portable"
    ) -join " "
    Write-UpdateLog "Restarting MGA."
    Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $appDir -WindowStyle Hidden
    Write-UpdateLog "Portable update completed."
} catch {
    if (-not $script:LogPath) {
        $script:LogPath = Join-Path (Split-Path -Parent $PlanPath) "mga_update.log"
    }
    Write-UpdateLog "ERROR: $($_.Exception.Message)"
    Write-UpdateLog "ERROR_DETAILS: $($_ | Out-String)"
    throw
}
