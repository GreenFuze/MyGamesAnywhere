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

function Get-PreservedPortableNames {
    return @{
        "config.json" = $true
        "data" = $true
        "media" = $true
        "source_cache" = $true
        "updates" = $true
        "logs" = $true
    }
}

function Copy-PortableImmutableFiles {
    param([string]$Source, [string]$Destination)
    $preserve = Get-PreservedPortableNames
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    Get-ChildItem -LiteralPath $Source -Force | ForEach-Object {
        if ($preserve.ContainsKey($_.Name)) {
            return
        }
        $target = Join-Path $Destination $_.Name
        if ($_.PSIsContainer) {
            Copy-Item -LiteralPath $_.FullName -Destination $target -Recurse -Force
        } else {
            Copy-Item -LiteralPath $_.FullName -Destination $target -Force
        }
    }
}

function Remove-PortableImmutableFiles {
    param([string]$AppDir)
    $preserve = Get-PreservedPortableNames
    Get-ChildItem -LiteralPath $AppDir -Force | ForEach-Object {
        if ($preserve.ContainsKey($_.Name)) {
            return
        }
        Remove-Item -LiteralPath $_.FullName -Recurse -Force
    }
}

function Get-DbPath {
    param([string]$ConfigPath, [string]$DataDir)
    if (Test-Path -LiteralPath $ConfigPath) {
        $config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
        if ($config.DB_PATH) {
            return [string]$config.DB_PATH
        }
    }
    return Join-Path $DataDir "data\db.sqlite"
}

function Backup-DbTriplet {
    param([string]$ConfigPath, [string]$DataDir, [string]$Destination)
    $dbPath = Get-DbPath -ConfigPath $ConfigPath -DataDir $DataDir
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    foreach ($path in @($dbPath, "$dbPath-wal", "$dbPath-shm")) {
        if (Test-Path -LiteralPath $path) {
            Copy-Item -LiteralPath $path -Destination (Join-Path $Destination (Split-Path -Leaf $path)) -Force
            Write-UpdateLog "Backed up DB file $path."
        }
    }
}

function Restore-DbTriplet {
    param([string]$ConfigPath, [string]$DataDir, [string]$Source)
    $dbPath = Get-DbPath -ConfigPath $ConfigPath -DataDir $DataDir
    foreach ($path in @($dbPath, "$dbPath-wal", "$dbPath-shm")) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }
    foreach ($file in Get-ChildItem -LiteralPath $Source -File -ErrorAction SilentlyContinue) {
        $target = Join-Path (Split-Path -Parent $dbPath) $file.Name
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force
        Write-UpdateLog "Restored DB file $target."
    }
}

function Backup-PortableUpdate {
    param([string]$AppDir, [string]$DataDir, [string]$ConfigPath)
    $backupRoot = Join-Path $AppDir "updates\backups\portable"
    $backupDir = Join-Path $backupRoot (Get-Date -Format "yyyyMMdd-HHmmss")
    $appBackup = Join-Path $backupDir "app"
    $dbBackup = Join-Path $backupDir "db"
    Copy-PortableImmutableFiles -Source $AppDir -Destination $appBackup
    Backup-DbTriplet -ConfigPath $ConfigPath -DataDir $DataDir -Destination $dbBackup
    $manifest = @{
        app_backup = $appBackup
        db_backup = $dbBackup
        created_at = (Get-Date).ToString("o")
    }
    $manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $backupRoot "latest.json") -Encoding UTF8
    Get-ChildItem -LiteralPath $backupRoot -Directory |
        Sort-Object Name -Descending |
        Select-Object -Skip 3 |
        ForEach-Object { Remove-Item -LiteralPath $_.FullName -Recurse -Force }
    Write-UpdateLog "Created portable update backup at $backupDir."
    return $manifest
}

function Restore-PortableUpdate {
    param([string]$AppDir, [string]$DataDir, [string]$ConfigPath, $Manifest)
    Remove-PortableImmutableFiles -AppDir $AppDir
    Copy-PortableImmutableFiles -Source ([string]$Manifest.app_backup) -Destination $AppDir
    Restore-DbTriplet -ConfigPath $ConfigPath -DataDir $DataDir -Source ([string]$Manifest.db_backup)
    Write-UpdateLog "Restored portable update backup."
}

function Run-MigrateOnly {
    param([string]$AppDir, [string]$DataDir, [string]$ConfigPath)
    $serverExe = Join-Path $AppDir "mga_server.exe"
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "Missing server executable for migration: $serverExe"
    }
    $backupDir = Join-Path $AppDir "updates\backups\migration"
    $arguments = @(
        "--migrate-only",
        "--app-dir", (Quote-Arg $AppDir),
        "--data-dir", (Quote-Arg $DataDir),
        "--config", (Quote-Arg $ConfigPath),
        "--runtime-mode", "portable",
        "--migration-backup-dir", (Quote-Arg $backupDir)
    ) -join " "
    Write-UpdateLog "Running migrate-only."
    $process = Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $AppDir -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "mga_server --migrate-only failed with exit code $($process.ExitCode)"
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
    $backupManifest = Backup-PortableUpdate -AppDir $appDir -DataDir $dataDir -ConfigPath $configPath

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
    try {
        Run-MigrateOnly -AppDir $appDir -DataDir $dataDir -ConfigPath $configPath
    } catch {
        Write-UpdateLog "Migration failed; restoring previous portable app and database. Error: $($_.Exception.Message)"
        Restore-PortableUpdate -AppDir $appDir -DataDir $dataDir -ConfigPath $configPath -Manifest $backupManifest
        $serverExe = Join-Path $appDir "mga_server.exe"
        $arguments = @(
            "--app-dir", (Quote-Arg $appDir),
            "--data-dir", (Quote-Arg $dataDir),
            "--config", (Quote-Arg $configPath),
            "--runtime-mode", "portable"
        ) -join " "
        Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $appDir -WindowStyle Hidden
        throw "Portable update migration failed and the previous version was restored. $($_.Exception.Message)"
    }

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
