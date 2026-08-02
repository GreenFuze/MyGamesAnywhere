param(
    [int]$FirstPort = 18931,
    [int]$SecondPort = 18932
)

$ErrorActionPreference = "Stop"
$repo = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$client = Join-Path $repo "client"
$consoleSource = Join-Path $client "bin\mga-client.exe"
$agentSource = Join-Path $client "bin\mga-client-agent.exe"
if (-not (Test-Path -LiteralPath $consoleSource) -or -not (Test-Path -LiteralPath $agentSource)) {
    throw "Build the MGA Client before running the IPC proof."
}

$proofID = [Guid]::NewGuid().ToString("N")
$proofRoot = Join-Path $env:TEMP "mga-client-ipc-proof-$proofID"
$dataDir = Join-Path $proofRoot "data"
$consolePath = Join-Path $proofRoot "mga-ipc-console-$proofID.exe"
$agentPath = Join-Path $proofRoot "mga-ipc-agent-$proofID.exe"
$agentName = [IO.Path]::GetFileNameWithoutExtension($agentPath)
$server = $null

function Wait-Until([scriptblock]$Condition, [string]$Failure, [int]$Seconds = 15) {
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw $Failure
}

try {
    New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
    Copy-Item -LiteralPath $consoleSource -Destination $consolePath
    Copy-Item -LiteralPath $agentSource -Destination $agentPath

    $server = Start-Process -FilePath "node.exe" `
        -ArgumentList @((Join-Path $PSScriptRoot "proof-server.mjs"), $FirstPort, $SecondPort) `
        -PassThru -WindowStyle Hidden
    Wait-Until {
        try {
            (Invoke-WebRequest -UseBasicParsing -TimeoutSec 1 "http://127.0.0.1:$FirstPort/health").StatusCode -eq 200
        } catch { $false }
    } "The IPC proof server did not start."

    $environment = @{ MGA_CLIENT_DATA_DIR = $dataDir }
    $firstServer = "http://127.0.0.1:$FirstPort"
    $secondServer = "http://127.0.0.1:$SecondPort"

    $initial = Start-Process -FilePath $consolePath `
        -ArgumentList @("pair", "--server", $firstServer, "--code", "proof-first") `
        -Environment $environment -PassThru -Wait -NoNewWindow
    if ($initial.ExitCode -ne 0) { throw "Initial proof pairing failed with exit code $($initial.ExitCode)." }

    Start-Process -FilePath $agentPath -ArgumentList @("agent") `
        -Environment $environment -WindowStyle Hidden | Out-Null
    $logPath = Join-Path $dataDir "mga-client.log"
    Wait-Until {
        (Test-Path -LiteralPath $logPath) -and
        ((Get-Content -LiteralPath $logPath -Raw) -match "agent host starting in standard mode with 1 server binding")
    } "The first proof tray agent did not start."

    $pairURI = "mga://pair?server=$([Uri]::EscapeDataString($secondServer))&code=proof-second"
    $secondary = Start-Process -FilePath $consolePath `
        -ArgumentList @("protocol", $pairURI) `
        -Environment $environment -PassThru -Wait -NoNewWindow
    if ($secondary.ExitCode -ne 0) { throw "Forwarded protocol process failed with exit code $($secondary.ExitCode)." }

    Wait-Until {
        $text = Get-Content -LiteralPath $logPath -Raw
        $text -match "forwarded browser protocol request" -and
        $text -match "same-mode agent takeover launched after forwarded pair request" -and
        $text -match "agent takeover acknowledged" -and
        $text -match "agent host starting in standard mode with 2 server binding"
    } "The running client did not complete the forwarded pair and takeover."

    $config = Get-Content -LiteralPath (Join-Path $dataDir "config.json") -Raw | ConvertFrom-Json
    if ($config.bindings.Count -ne 2) {
        throw "Proof config contains $($config.bindings.Count) bindings, expected 2."
    }
    Wait-Until {
        @((Get-Process -Name $agentName -ErrorAction SilentlyContinue)).Count -eq 1
    } "The proof did not converge to exactly one tray agent."

    [pscustomobject]@{
        Result = "PASS"
        ForwardedPair = $true
        BindingCount = $config.bindings.Count
        TrayProcessCount = @((Get-Process -Name $agentName -ErrorAction SilentlyContinue)).Count
        ExecutionModePreserved = "standard"
        LogPath = $logPath
    } | Format-List
} finally {
    Get-Process -Name $agentName -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    if ($server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $proofRoot) {
        Remove-Item -LiteralPath $proofRoot -Recurse -Force
    }
}
