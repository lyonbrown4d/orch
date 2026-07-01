[CmdletBinding()]
param(
    [switch]$Runtime,
    [switch]$E2E,
    [switch]$E2EDind,
    [switch]$SkipBaseline
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not $SkipBaseline) {
    Write-Host "[release-gate] Running baseline checks (release-gate:static)..."
    task release-gate:static
}
else {
    Write-Host "[release-gate] Skipping baseline checks by request."
}

if ($Runtime) {
    Write-Host "[release-gate] Running local container runtime smoke checks..."
    task smoke:local-container-runtimes
}

if ($E2E) {
    Write-Host "[release-gate] Running full-chain e2e gate..."
    if ($E2EDind) {
        task smoke:full-chain-full-dind
    }
    else {
        task release-gate:e2e
    }
}

Write-Host "[release-gate] Completed."
