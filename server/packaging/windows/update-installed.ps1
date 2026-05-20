param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("stop-user","start-user","backup","migrate-or-rollback")]
    [string]$Action,
    [Parameter(Mandatory=$true)]
    [string]$AppDir,
    [Parameter(Mandatory=$true)]
    [string]$DataDir,
    [Parameter(Mandatory=$true)]
    [string]$ConfigPath,
    [string]$LogPath,
    [ValidateSet("user","service","machine")]
    [string]$InstallType = "user",
    [int]$ServerPid = 0
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

function Get-UpdatesDir {
    return Join-Path $DataDir "updates"
}

function Get-BackupRoot {
    return Join-Path (Get-UpdatesDir) "backups\installed"
}

function Get-LatestBackupManifestPath {
    return Join-Path (Get-BackupRoot) "latest.json"
}

function Get-DbPath {
    if (-not (Test-Path -LiteralPath $ConfigPath)) {
        return Join-Path $DataDir "data\db.sqlite"
    }
    $config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
    if ($config.DB_PATH) {
        return [string]$config.DB_PATH
    }
    return Join-Path $DataDir "data\db.sqlite"
}

function Copy-DirectoryContents {
    param([string]$Source, [string]$Destination)
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    Get-ChildItem -LiteralPath $Source -Force | ForEach-Object {
        $target = Join-Path $Destination $_.Name
        if ($_.PSIsContainer) {
            Copy-Item -LiteralPath $_.FullName -Destination $target -Recurse -Force
        } else {
            Copy-Item -LiteralPath $_.FullName -Destination $target -Force
        }
    }
}

function Backup-DbTriplet {
    param([string]$Destination)
    $dbPath = Get-DbPath
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    foreach ($path in @($dbPath, "$dbPath-wal", "$dbPath-shm")) {
        if (Test-Path -LiteralPath $path) {
            Copy-Item -LiteralPath $path -Destination (Join-Path $Destination (Split-Path -Leaf $path)) -Force
            Write-InstallLog "Backed up DB file $path."
        }
    }
}

function Restore-DbTriplet {
    param([string]$Source)
    $dbPath = Get-DbPath
    foreach ($path in @($dbPath, "$dbPath-wal", "$dbPath-shm")) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }
    foreach ($file in Get-ChildItem -LiteralPath $Source -File -ErrorAction SilentlyContinue) {
        $target = Join-Path (Split-Path -Parent $dbPath) $file.Name
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force
        Write-InstallLog "Restored DB file $target."
    }
}

function Backup-InstalledUpdate {
    $root = Get-BackupRoot
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $backupDir = Join-Path $root $stamp
    $appBackup = Join-Path $backupDir "app"
    $dbBackup = Join-Path $backupDir "db"
    New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
    if (Test-Path -LiteralPath $AppDir) {
        Copy-DirectoryContents -Source $AppDir -Destination $appBackup
    }
    Backup-DbTriplet -Destination $dbBackup
    $manifest = @{
        app_dir = $AppDir
        data_dir = $DataDir
        config_path = $ConfigPath
        install_type = $InstallType
        app_backup = $appBackup
        db_backup = $dbBackup
        created_at = (Get-Date).ToString("o")
    }
    $manifestPath = Get-LatestBackupManifestPath
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $manifestPath) | Out-Null
    $manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    Write-InstallLog "Created update backup at $backupDir."
    Prune-Backups -Root $root -Keep 3
}

function Prune-Backups {
    param([string]$Root, [int]$Keep)
    if (-not (Test-Path -LiteralPath $Root)) {
        return
    }
    Get-ChildItem -LiteralPath $Root -Directory |
        Sort-Object Name -Descending |
        Select-Object -Skip $Keep |
        ForEach-Object {
            Write-InstallLog "Removing old update backup $($_.FullName)."
            Remove-Item -LiteralPath $_.FullName -Recurse -Force
        }
}

function Restore-InstalledUpdate {
    $manifestPath = Get-LatestBackupManifestPath
    if (-not (Test-Path -LiteralPath $manifestPath)) {
        throw "No update backup manifest found at $manifestPath"
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $appBackup = [string]$manifest.app_backup
    $dbBackup = [string]$manifest.db_backup
    if (-not (Test-Path -LiteralPath $appBackup)) {
        throw "App backup is missing: $appBackup"
    }
    Get-ChildItem -LiteralPath $AppDir -Force -ErrorAction SilentlyContinue | ForEach-Object {
        Remove-Item -LiteralPath $_.FullName -Recurse -Force
    }
    Copy-DirectoryContents -Source $appBackup -Destination $AppDir
    if (Test-Path -LiteralPath $dbBackup) {
        Restore-DbTriplet -Source $dbBackup
    }
    Write-InstallLog "Restored installed update backup."
}

function Run-MigrateOnly {
    $serverExe = Join-Path $AppDir "mga_server.exe"
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "Missing server executable for migration: $serverExe"
    }
    $backupDir = Join-Path (Get-UpdatesDir) "backups\migration"
    $arguments = @(
        "--migrate-only",
        "--app-dir", (Quote-Arg $AppDir),
        "--data-dir", (Quote-Arg $DataDir),
        "--config", (Quote-Arg $ConfigPath),
        "--runtime-mode", $(if ($InstallType -eq "service") { "machine" } else { "user" }),
        "--migration-backup-dir", (Quote-Arg $backupDir)
    ) -join " "
    Write-InstallLog "Running migrate-only: $serverExe $arguments"
    $process = Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $AppDir -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "mga_server --migrate-only failed with exit code $($process.ExitCode)"
    }
}

function Get-TargetServerProcesses {
    $serverExe = (Join-Path $AppDir "mga_server.exe").ToLowerInvariant()
    $candidates = @()
    if ($ServerPid -gt 0) {
        $process = Get-Process -Id $ServerPid -ErrorAction SilentlyContinue
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
            if ($ServerPid -gt 0 -and $process.Id -eq $ServerPid) {
                $unique[$process.Id] = $process
            }
        }
    }
    return $unique.Values
}

switch ($Action) {
    "backup" {
        Backup-InstalledUpdate
    }
    "migrate-or-rollback" {
        try {
            Run-MigrateOnly
            Write-InstallLog "Database migration succeeded."
        } catch {
            Write-InstallLog "Migration failed; restoring previous app and database. Error: $($_.Exception.Message)"
            Restore-InstalledUpdate
            if ($InstallType -eq "service") {
                Start-Service -Name "MyGamesAnywhere" -ErrorAction SilentlyContinue
            } else {
                $Action = "start-user"
                $serverExe = Join-Path $AppDir "mga_server.exe"
                $arguments = @(
                    "--app-dir", (Quote-Arg $AppDir),
                    "--data-dir", (Quote-Arg $DataDir),
                    "--config", (Quote-Arg $ConfigPath),
                    "--runtime-mode", "user"
                ) -join " "
                Start-Process -FilePath $serverExe -ArgumentList $arguments -WorkingDirectory $AppDir -WindowStyle Hidden
            }
            throw "MGA update migration failed and the previous version was restored. $($_.Exception.Message)"
        }
    }
    "stop-user" {
        Write-InstallLog "Stopping user-mode MGA server. AppDir=$AppDir Pid=$ServerPid"
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
