[CmdletBinding()]
param(
    [string]$ImageTag = "orch-docker-raft-ingress:local",
    [string]$NetworkName = "orch-raft-ingress-smoke",
    [string]$WorkDir = ".orch-docker-raft-ingress",
    [string]$Dockerfile = "scripts/docker-raft-ingress/Dockerfile",
    [string]$WhoamiImage = "traefik/whoami:v1.11",
    [string]$AppName = "whoami-smoke",
    [string]$WorkloadName = "whoami",
    [int]$TimeoutSeconds = 180,
    [switch]$SkipBuild,
    [switch]$SkipImageBuild,
    [switch]$KeepCluster,
    [switch]$KeepWorkload
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repoRoot

$nodes = @(
    @{ ID = "node-a"; Name = "orch-raft-ingress-node-a"; APIHostPort = 18101; IngressHostPort = 18080 },
    @{ ID = "node-b"; Name = "orch-raft-ingress-node-b"; APIHostPort = 18102; IngressHostPort = 18081 },
    @{ ID = "node-c"; Name = "orch-raft-ingress-node-c"; APIHostPort = 18103; IngressHostPort = 18082 }
)
$apiPort = 17443
$raftPort = 7444
$ingressPort = 18080
$namespace = "default"
$containerName = "orch-default-$WorkloadName"
$workRoot = Join-Path $repoRoot $WorkDir
$hostBinDir = Join-Path $workRoot "bin"
$linuxBinDir = Join-Path $workRoot "linux-bin"
$logDir = Join-Path $workRoot "logs"
$manifestPath = Join-Path $workRoot "whoami.yaml"

function Assert-UnderRepo {
    param([Parameter(Mandatory = $true)][string]$Path)
    $full = [System.IO.Path]::GetFullPath($Path)
    $repoPrefix = $repoRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
    if (-not $full.StartsWith($repoPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to use path outside repository: $full"
    }
    return $full
}

$workRoot = Assert-UnderRepo $workRoot
$hostBinDir = Assert-UnderRepo $hostBinDir
$linuxBinDir = Assert-UnderRepo $linuxBinDir
$logDir = Assert-UnderRepo $logDir
New-Item -ItemType Directory -Force $hostBinDir, $linuxBinDir, $logDir | Out-Null

function Test-IsWindows {
    return [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
        [System.Runtime.InteropServices.OSPlatform]::Windows
    )
}

$binExt = ""
if (Test-IsWindows) {
    $binExt = ".exe"
}
$cliBin = Join-Path $hostBinDir "orch$binExt"
$linuxServerBin = Join-Path $linuxBinDir "orch-server"

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

function Invoke-CLIJson {
    param(
        [Parameter(Mandatory = $true)][string]$ServerURL,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )
    $cmdArgs = @("--server", $ServerURL) + $Arguments
    $raw = & $cliBin @cmdArgs
    if ($LASTEXITCODE -ne 0) {
        throw "orch CLI failed ($LASTEXITCODE): --server $ServerURL $($Arguments -join ' ')"
    }
    $text = ($raw | Out-String).Trim()
    if ($text -eq "") {
        return @()
    }
    $parsed = $text | ConvertFrom-Json
    if ($null -eq $parsed) {
        return @()
    }
    if ($parsed -is [array]) {
        return $parsed
    }
    return @($parsed)
}

function Node-URL {
    param([Parameter(Mandatory = $true)]$Node)
    return "http://127.0.0.1:$($Node.APIHostPort)"
}

function Get-JSONProperty {
    param(
        [Parameter(Mandatory = $true)]$Object,
        [Parameter(Mandatory = $true)][string]$Name,
        $Default = $null
    )
    $prop = $Object.PSObject.Properties[$Name]
    if ($null -eq $prop) {
        return $Default
    }
    return $prop.Value
}

function Get-RaftStatus {
    param([Parameter(Mandatory = $true)]$Node)
    return (Invoke-CLIJson -ServerURL (Node-URL $Node) -Arguments @("raft", "status", "--json"))[0]
}

function Get-RaftLeaderID {
    param([Parameter(Mandatory = $true)]$Status)
    $leaderID = [string](Get-JSONProperty -Object $Status -Name "leaderId" -Default "")
    if ($leaderID -eq "" -and [bool](Get-JSONProperty -Object $Status -Name "isLeader" -Default $false)) {
        $leaderID = [string](Get-JSONProperty -Object $Status -Name "nodeId" -Default "")
    }
    return $leaderID
}

function Wait-CLIReady {
    param([Parameter(Mandatory = $true)]$Node)
    $url = Node-URL $Node
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        & $cliBin --server $url health *> $null
        if ($LASTEXITCODE -eq 0) {
            & $cliBin --server $url ready --wait --timeout "5s" *> $null
            if ($LASTEXITCODE -eq 0) {
                return
            }
        }
        $running = & docker inspect -f "{{.State.Running}}" $Node.Name 2>$null
        if ($LASTEXITCODE -eq 0 -and (($running | Out-String).Trim()) -ne "true") {
            throw "orch-server container $($Node.Name) exited early. See docker logs $($Node.Name)"
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for orch ready at $url"
}

function Wait-RaftMembers {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $allReady = $true
        foreach ($node in $nodes) {
            try {
                $status = Get-RaftStatus -Node $node
                $members = Get-JSONProperty -Object $status -Name "members" -Default @()
                $leaderID = Get-RaftLeaderID -Status $status
                if (-not [bool](Get-JSONProperty -Object $status -Name "ready" -Default $false) -or $members.Count -ne 3 -or $leaderID -eq "") {
                    $allReady = $false
                    break
                }
            }
            catch {
                $allReady = $false
                break
            }
        }
        if ($allReady) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for 3-node Raft membership"
}

function Find-LeaderNode {
    foreach ($node in $nodes) {
        $status = Get-RaftStatus -Node $node
        if ([bool](Get-JSONProperty -Object $status -Name "isLeader" -Default $false)) {
            return $node
        }
    }
    throw "Could not find Raft leader"
}

function Find-NodeByID {
    param([Parameter(Mandatory = $true)][string]$NodeID)
    return $nodes | Where-Object { $_.ID -eq $NodeID } | Select-Object -First 1
}

function Select-TargetNode {
    param([Parameter(Mandatory = $true)][string]$LeaderID)
    $target = $nodes | Where-Object { $_.ID -ne $LeaderID } | Select-Object -First 1
    if ($null -eq $target) {
        throw "Could not select non-leader target node"
    }
    return $target
}

function Write-WhoamiManifest {
    param([Parameter(Mandatory = $true)][string]$TargetNodeID)
    @"
apiVersion: warden.arcgolabs.io/v1alpha1
kind: App
metadata:
  name: $AppName
  namespace: $namespace
workloads:
  - name: $WorkloadName
    kind: service
    runtime: docker
    run:
      artifact:
        image: $WhoamiImage
      runtimeOptions:
        docker:
          networkMode: $NetworkName
    endpoints:
      - name: http
        port: 80
        protocol: http
    scheduling:
      preferredNodes:
        - $TargetNodeID
ingresses:
  - name: public
    routes:
      - path: /
        backend:
          workload: $WorkloadName
          endpoint: http
"@ | Set-Content -Path $manifestPath -Encoding utf8
}

function Wait-AssignmentRunning {
    param(
        [Parameter(Mandatory = $true)]$LeaderNode,
        [Parameter(Mandatory = $true)][string]$TargetNodeID
    )
    $leaderURL = Node-URL $LeaderNode
    $assignmentKey = "$namespace/$AppName/$WorkloadName"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $assignments = Invoke-CLIJson -ServerURL $leaderURL -Arguments @("get", "assignments", "--json")
        $found = $assignments | Where-Object {
            $_.key -eq $assignmentKey -and $_.node -eq $TargetNodeID -and $_.status -eq "running"
        } | Select-Object -First 1
        if ($null -ne $found) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for $assignmentKey to run on $TargetNodeID"
}

function Wait-AppDeleted {
    param([Parameter(Mandatory = $true)]$LeaderNode)
    $leaderURL = Node-URL $LeaderNode
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $apps = Invoke-CLIJson -ServerURL $leaderURL -Arguments @("get", "apps", "--json")
        $found = $apps | Where-Object { $_.name -eq $AppName -and $_.namespace -eq $namespace } | Select-Object -First 1
        if ($null -eq $found) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for app $namespace/$AppName to be deleted"
}

function Wait-WorkloadContainerRemoved {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $ids = & docker ps -a --filter "name=^/$containerName$" --format "{{.ID}}"
        if ($LASTEXITCODE -ne 0) {
            throw "docker ps failed while checking $containerName"
        }
        if ((($ids | Out-String).Trim()) -eq "") {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for workload container $containerName to be removed"
}

function Wait-IngressWhoami {
    param([Parameter(Mandatory = $true)]$IngressNode)
    $url = "http://127.0.0.1:$($IngressNode.IngressHostPort)/"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
            $body = [string]$resp.Content
            if ($resp.StatusCode -eq 200 -and $body.Contains("Hostname:")) {
                Write-Host "Ingress response from $url"
                Write-Host $body
                return
            }
        }
        catch {
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for whoami response through ingress $url"
}

function Remove-ContainerIfExists {
    param([Parameter(Mandatory = $true)][string]$Name)
    $id = & docker ps -a --filter "name=^/$Name$" --format "{{.ID}}"
    if ($LASTEXITCODE -ne 0) {
        throw "docker ps failed while checking $Name"
    }
    if ((($id | Out-String).Trim()) -ne "") {
        Invoke-Docker @("rm", "-f", $Name)
    }
}

function Remove-NetworkIfExists {
    $id = & docker network ls --filter "name=^$NetworkName$" --format "{{.ID}}"
    if ($LASTEXITCODE -ne 0) {
        throw "docker network ls failed while checking $NetworkName"
    }
    if ((($id | Out-String).Trim()) -ne "") {
        Invoke-Docker @("network", "rm", $NetworkName)
    }
}

function Build-Binaries {
    Invoke-Checked go @("build", "-o", $cliBin, "./cmd/orch-cli")

    $previousGOOS = [Environment]::GetEnvironmentVariable("GOOS", "Process")
    $previousGOARCH = [Environment]::GetEnvironmentVariable("GOARCH", "Process")
    $previousCGO = [Environment]::GetEnvironmentVariable("CGO_ENABLED", "Process")
    try {
        [Environment]::SetEnvironmentVariable("GOOS", "linux", "Process")
        [Environment]::SetEnvironmentVariable("GOARCH", "amd64", "Process")
        [Environment]::SetEnvironmentVariable("CGO_ENABLED", "0", "Process")
        Invoke-Checked go @("build", "-o", $linuxServerBin, "./cmd/orch-server")
    }
    finally {
        [Environment]::SetEnvironmentVariable("GOOS", $previousGOOS, "Process")
        [Environment]::SetEnvironmentVariable("GOARCH", $previousGOARCH, "Process")
        [Environment]::SetEnvironmentVariable("CGO_ENABLED", $previousCGO, "Process")
    }
}

function Start-OrchNode {
    param([Parameter(Mandatory = $true)]$Node)
    $peers = ($nodes | ForEach-Object { "$($_.ID)=$($_.Name):$raftPort" }) -join ","
    $clusterNodes = ($nodes | ForEach-Object { "$($_.ID)=http://$($_.Name):$apiPort" }) -join ","
    Invoke-Docker @(
        "run", "--detach",
        "--name", $Node.Name,
        "--network", $NetworkName,
        "--volume", "/var/run/docker.sock:/var/run/docker.sock",
        "--publish", "$($Node.APIHostPort):$apiPort",
        "--publish", "$($Node.IngressHostPort):$ingressPort",
        $ImageTag,
        "--http-addr", ":$apiPort",
        "--raft-node-id", $Node.ID,
        "--raft-bind", "0.0.0.0:$raftPort",
        "--raft-advertise", "$($Node.Name):$raftPort",
        "--raft-peers", $peers,
        "--raft-bootstrap=true",
        "--raft-data-dir", "/var/lib/orch/dragonboat",
        "--cluster-nodes", $clusterNodes,
        "--ingress-enabled=true",
        "--ingress-listen", ":$ingressPort",
        "--dns-enabled=true",
        "--dns-listen", "127.0.0.1:15353",
        "--observability-prometheus-enabled=false",
        "--observability-otlp-enabled=false",
        "--log-level", "info"
    )
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker CLI was not found on PATH"
}
Invoke-Checked docker @("version")

if (-not $SkipBuild) {
    Build-Binaries
}
if (-not (Test-Path $cliBin)) {
    throw "CLI binary not found: $cliBin"
}
if (-not (Test-Path $linuxServerBin)) {
    throw "Linux server binary not found: $linuxServerBin"
}
if (-not $SkipImageBuild) {
    Invoke-Docker @("build", "-t", $ImageTag, "-f", $Dockerfile, $linuxBinDir)
}

try {
    if (-not $KeepCluster) {
        foreach ($node in $nodes) {
            Remove-ContainerIfExists -Name $node.Name
        }
        if (-not $KeepWorkload) {
            Remove-ContainerIfExists -Name $containerName
        }
        Remove-NetworkIfExists
    }

    Invoke-Docker @("network", "create", $NetworkName)
    foreach ($node in $nodes) {
        Start-OrchNode -Node $node
    }
    foreach ($node in $nodes) {
        Wait-CLIReady -Node $node
    }
    Wait-RaftMembers

    $leader = Find-LeaderNode
    $leaderID = [string]$leader.ID
    $target = Select-TargetNode -LeaderID $leaderID
    Write-WhoamiManifest -TargetNodeID $target.ID

    $leaderURL = Node-URL $leader
    Write-Host "Raft leader:       $leaderID"
    Write-Host "Workload target:   $($target.ID)"
    Write-Host "Leader API:        $leaderURL"
    Write-Host "Target ingress:    http://127.0.0.1:$($target.IngressHostPort)/"
    Write-Host "Docker network:    $NetworkName"
    Write-Host "Manifest:          $manifestPath"
    Write-Host ""

    Invoke-Checked $cliBin @("--server", $leaderURL, "apply", "--file", $manifestPath, "--watch", "--timeout", "$($TimeoutSeconds)s")
    Wait-AssignmentRunning -LeaderNode $leader -TargetNodeID $target.ID
    foreach ($node in $nodes) {
        Wait-IngressWhoami -IngressNode $node
    }

    Invoke-Checked $cliBin @("--server", $leaderURL, "get", "apps")
    Invoke-Checked $cliBin @("--server", $leaderURL, "get", "assignments")
    Invoke-Checked $cliBin @("--server", (Node-URL $target), "describe", "workload", $WorkloadName, "--app", $AppName, "-n", $namespace)

    if (-not $KeepWorkload) {
        Write-Host ""
        Write-Host "Deleting whoami app..."
        Invoke-Checked $cliBin @("--server", $leaderURL, "delete", "app", $AppName, "-n", $namespace)
        Wait-AppDeleted -LeaderNode $leader
        Wait-WorkloadContainerRemoved
    }

    Write-Host "Docker Raft ingress smoke completed."
}
catch {
    Write-Warning $_
    foreach ($node in $nodes) {
        try {
            $logPath = Join-Path $logDir "$($node.ID).docker.log"
            & docker logs $node.Name *> $logPath
            Write-Host "Saved logs for $($node.Name): $logPath"
        }
        catch {
            Write-Warning "failed to collect logs for $($node.Name): $_"
        }
    }
    throw
}
finally {
    if (-not $KeepCluster) {
        foreach ($node in $nodes) {
            try {
                Remove-ContainerIfExists -Name $node.Name
            }
            catch {
                Write-Warning $_
            }
        }
    }
    if (-not $KeepWorkload) {
        try {
            Remove-ContainerIfExists -Name $containerName
        }
        catch {
            Write-Warning $_
        }
    }
    if (-not $KeepCluster) {
        try {
            Remove-NetworkIfExists
        }
        catch {
            Write-Warning $_
        }
    }
}

