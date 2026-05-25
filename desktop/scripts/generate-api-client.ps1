#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Generates the NSwag API client from the server OpenAPI spec.

.DESCRIPTION
    Runs NSwag to produce src/MGA.Api/Generated/MgaApiClient.g.cs from
    ../server/openapi.yaml.

    The generated file is committed so CI never requires NSwag at build time.

.EXAMPLE
    .\scripts\generate-api-client.ps1
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

$RepoRoot   = (Resolve-Path "$PSScriptRoot\..\.." ).Path
$SpecPath   = Join-Path $RepoRoot "server\openapi.yaml"
$OutDir     = Join-Path $PSScriptRoot "..\src\MGA.Api\Generated"
$OutFile    = Join-Path $OutDir "MgaApiClient.g.cs"

# ---------------------------------------------------------------------------
# Ensure NSwag CLI is installed
# ---------------------------------------------------------------------------

if (-not (Get-Command nswag -ErrorAction SilentlyContinue)) {
    Write-Host "Installing NSwag CLI (dotnet tool install -g NSwag.MSBuild)…"
    dotnet tool install -g NSwag.MSBuild
    if ($LASTEXITCODE -ne 0) { throw "Failed to install NSwag.MSBuild." }
}

# ---------------------------------------------------------------------------
# Validate inputs
# ---------------------------------------------------------------------------

if (-not (Test-Path $SpecPath)) {
    throw "OpenAPI spec not found at: $SpecPath"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# ---------------------------------------------------------------------------
# Generate
# ---------------------------------------------------------------------------

Write-Host "Generating API client from $SpecPath …"

nswag openapi2csclient `
    /input:"$SpecPath" `
    /namespace:"MGA.Api.Generated" `
    /className:"MgaApiClient" `
    /output:"$OutFile" `
    /generateClientClasses:true `
    /generateClientInterfaces:true `
    /generateExceptionClasses:true `
    /exceptionClass:"MgaApiException" `
    /generateResponseClasses:true `
    /responseClass:"SwaggerResponse" `
    /jsonLibrary:"SystemTextJson" `
    /generateJsonMethods:false `
    /useBaseUrl:true `
    /injectHttpClient:true `
    /disposeHttpClient:false `
    /generateContractsOutput:false `
    /operationGenerationMode:"SingleClientFromPathSegments"

if ($LASTEXITCODE -ne 0) { throw "NSwag generation failed." }

Write-Host "Generated: $OutFile" -ForegroundColor Green
