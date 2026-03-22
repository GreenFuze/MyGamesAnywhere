# Requires: mga_server running (e.g. from server\bin after build.ps1), Xbox integration configured.
# Scans only the Xbox source integration, then prints games with xcloud_available / xcloud_url.
$ErrorActionPreference = "Stop"
$BaseUrl = if ($env:MGA_BASE_URL) { $env:MGA_BASE_URL.TrimEnd("/") } else { "http://127.0.0.1:8900" }

Write-Host "MGA base: $BaseUrl" -ForegroundColor Cyan
$null = Invoke-RestMethod -Uri "$BaseUrl/health" -Method Get -TimeoutSec 10

$integrations = Invoke-RestMethod -Uri "$BaseUrl/api/integrations" -Method Get -TimeoutSec 30
$xbox = @($integrations | Where-Object { $_.plugin_id -eq "game-source-xbox" })
if ($xbox.Count -eq 0) {
    Write-Host "No integration with plugin_id game-source-xbox. Add one in the app, then re-run." -ForegroundColor Yellow
    exit 1
}

$xid = $xbox[0].id
Write-Host "Scanning Xbox integration: $xid ..." -ForegroundColor Cyan
$body = @{ game_sources = @($xid) } | ConvertTo-Json
# Scan can take many minutes (metadata resolvers).
$scan = Invoke-RestMethod -Uri "$BaseUrl/api/scan" -Method Post -Body $body -ContentType "application/json" -TimeoutSec (30 * 60)

$withX = @($scan.games | Where-Object { $_.xcloud_available -eq $true })
$withGP = @($scan.games | Where-Object { $_.is_game_pass -eq $true })
Write-Host ("Scan completed: {0} canonical games in response." -f $scan.games.Count) -ForegroundColor Green
Write-Host ("  xcloud_available=true: {0}" -f $withX.Count) -ForegroundColor Green
Write-Host ("  is_game_pass=true: {0}" -f $withGP.Count) -ForegroundColor Green

$sample = $withX | Select-Object -First 5 title, xcloud_available, is_game_pass, store_product_id, xcloud_url
if ($sample.Count -gt 0) {
    Write-Host "Sample (streamable):" -ForegroundColor Cyan
    $sample | Format-Table -AutoSize
} else {
    Write-Host "No games with xcloud_available in scan response (auth or Title Hub may return no streamable flags)." -ForegroundColor Yellow
}

exit 0
