[CmdletBinding()]
param(
    [string]$ImageTag = "orch-systemd-smoke:debian12",
    [string]$ContainerName = "orch-systemd-smoke",
    [string]$Dockerfile = "scripts/systemd/Dockerfile.debian",
    [string]$PackagePath = "",
    [string]$PackageArch = "amd64",
    [string]$DockerPlatform = "linux/amd64",
    [string]$WorkloadUnitName = "orch-systemd-smoke-workload.service",
    [string]$WorkDir = ".orch-systemd-smoke",
    [int]$TimeoutSeconds = 180,
    [string]$GoreleaserVersion = "v2.12.2",
    [switch]$SkipSnapshot,
    [switch]$KeepContainer,
    [switch]$SkipImageBuild
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repoRoot

$workRoot = Join-Path $repoRoot $WorkDir
New-Item -ItemType Directory -Force $workRoot | Out-Null

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $FilePath $($Arguments -join ' ')"
    }
}

function Invoke-Docker {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    Invoke-Checked docker $Arguments
}

function Remove-SmokeContainer {
    $id = & docker ps -a --filter "name=^/$ContainerName$" --format "{{.ID}}"
    if ($LASTEXITCODE -ne 0) {
        throw "docker ps failed while checking $ContainerName"
    }
    if (($id | Out-String).Trim() -ne "") {
        Invoke-Docker @("rm", "-f", $ContainerName)
    }
}

function Resolve-PackagePath {
    if (-not [string]::IsNullOrWhiteSpace($PackagePath)) {
        return (Resolve-Path $PackagePath).Path
    }

    if (-not $SkipSnapshot) {
        Invoke-Checked go @("run", "github.com/goreleaser/goreleaser/v2@$GoreleaserVersion", "release", "--snapshot", "--clean")
    }

    $dist = Join-Path $repoRoot "dist"
    if (-not (Test-Path $dist)) {
        throw "dist directory not found; run without -SkipSnapshot or pass -PackagePath"
    }

    $packages = Get-ChildItem -Path $dist -Recurse -Filter "*.deb" |
        Where-Object { $_.Name -match "_$PackageArch\.deb$" -or $_.Name -match "linux_$PackageArch\.deb$" } |
        Sort-Object LastWriteTime -Descending
    $pkg = $packages | Select-Object -First 1
    if ($null -eq $pkg) {
        throw "No .deb package for arch '$PackageArch' found under $dist"
    }
    return $pkg.FullName
}

function Wait-SystemdReady {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $state = & docker exec $ContainerName systemctl is-system-running 2>$null
        $text = ($state | Out-String).Trim()
        if ($LASTEXITCODE -eq 0 -or $text -eq "running" -or $text -eq "degraded") {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for systemd in $ContainerName"
}

function Wait-OrchServiceActive {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        & docker exec $ContainerName systemctl is-active --quiet orch-server.service
        if ($LASTEXITCODE -eq 0) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for orch-server.service to become active"
}

function Wait-AppDeleted {
    param(
        [Parameter(Mandatory = $true)][string]$Namespace,
        [Parameter(Mandatory = $true)][string]$Name
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $raw = & docker exec $ContainerName orch --server "http://127.0.0.1:17443" get apps --json
        if ($LASTEXITCODE -eq 0) {
            $text = ($raw | Out-String).Trim()
            if ($text -eq "") {
                return
            }

            $parsed = $text | ConvertFrom-Json
            if ($null -eq $parsed) {
                return
            }

            $found = $false
            foreach ($app in @($parsed)) {
                if ($null -eq $app) {
                    continue
                }
                $props = $app.PSObject.Properties
                if ($null -eq $props["namespace"] -or $null -eq $props["name"]) {
                    continue
                }
                if ($app.namespace -eq $Namespace -and $app.name -eq $Name) {
                    $found = $true
                    break
                }
            }
            if (-not $found) {
                return
            }
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for app $Namespace/$Name to be deleted"
}
function Write-SmokeManifest {
    $manifest = Join-Path $workRoot "systemd-smoke.yaml"
    @"
apiVersion: warden.arcgolabs.io/v1alpha1
kind: App
metadata:
  name: systemd-smoke
  namespace: default
workloads:
  - name: smoke-systemd
    kind: worker
    runtime: systemd
    run:
      exec:
        command: ["sh", "-c"]
        args:
          - echo orch-systemd-smoke-started; while true; do sleep 3600; done
      runtimeOptions:
        systemd:
          unitName: $WorkloadUnitName
          restart: on-failure
          restartSec: 1s
"@ | Set-Content -Path $manifest -Encoding utf8
    return $manifest
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker CLI was not found on PATH"
}
Invoke-Checked docker @("version")

if (-not $SkipImageBuild) {
    Invoke-Docker @("build", "--platform", $DockerPlatform, "-t", $ImageTag, "-f", $Dockerfile, ".")
}

$package = Resolve-PackagePath
$manifestPath = Write-SmokeManifest

Remove-SmokeContainer

try {
    Invoke-Docker @(
        "run", "--detach",
        "--name", $ContainerName,
        "--privileged",
        "--cgroupns=host",
        "--tmpfs", "/run",
        "--tmpfs", "/run/lock",
        "--volume", "/sys/fs/cgroup:/sys/fs/cgroup:rw",
        $ImageTag
    )
    Wait-SystemdReady

    Invoke-Docker @("cp", $package, "$ContainerName`:/tmp/orch.deb")
    Invoke-Docker @("cp", $manifestPath, "$ContainerName`:/tmp/systemd-smoke.yaml")

    Invoke-Docker @("exec", $ContainerName, "apt-get", "install", "-y", "/tmp/orch.deb")
    Invoke-Docker @("exec", $ContainerName, "test", "-f", "/usr/lib/systemd/system/orch-server.service")
    Invoke-Docker @("exec", $ContainerName, "test", "-f", "/etc/orch/env")
    Invoke-Docker @("exec", $ContainerName, "systemctl", "daemon-reload")
    Invoke-Docker @("exec", $ContainerName, "systemctl", "enable", "--now", "orch-server.service")
    Wait-OrchServiceActive

    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "ready", "--wait", "--timeout", "$($TimeoutSeconds)s")
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "apply", "--file", "/tmp/systemd-smoke.yaml", "--watch", "--timeout", "$($TimeoutSeconds)s")
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "wait", "app", "systemd-smoke", "-n", "default", "--for", "running", "--timeout", "$($TimeoutSeconds)s")
    Invoke-Docker @("exec", $ContainerName, "systemctl", "is-active", "--quiet", $WorkloadUnitName)
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "describe", "app", "systemd-smoke", "-n", "default")
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "describe", "workload", "smoke-systemd", "--app", "systemd-smoke", "-n", "default")
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "logs", "smoke-systemd", "--app", "systemd-smoke", "-n", "default", "--tail", "20")
    Invoke-Docker @("exec", $ContainerName, "orch", "--server", "http://127.0.0.1:17443", "delete", "app", "systemd-smoke", "-n", "default")
    Wait-AppDeleted -Namespace "default" -Name "systemd-smoke"
    Invoke-Docker @("exec", $ContainerName, "bash", "-lc", "test ! -e /etc/systemd/system/$WorkloadUnitName")
    Invoke-Docker @("exec", $ContainerName, "systemctl", "stop", "orch-server.service")
    Invoke-Docker @("exec", $ContainerName, "apt-get", "remove", "-y", "orch")
    Invoke-Docker @("exec", $ContainerName, "bash", "-lc", "test ! -e /usr/lib/systemd/system/orch-server.service")

    Write-Host "Docker systemd smoke completed."
}
catch {
    Write-Warning $_
    try {
        Write-Host "--- orch-server.service status ---"
        & docker exec $ContainerName systemctl status --no-pager orch-server.service
        Write-Host "--- orch-server.service journal ---"
        & docker exec $ContainerName journalctl -u orch-server.service --no-pager -n 100
    }
    catch {
        Write-Warning "failed to collect container systemd logs: $_"
    }
    throw
}
finally {
    if (-not $KeepContainer) {
        try {
            Remove-SmokeContainer
        }
        catch {
            Write-Warning $_
        }
    }
}



