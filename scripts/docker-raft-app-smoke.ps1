[CmdletBinding()]
param(
    [ValidateSet("placement", "nextcloud", "seaweed")]
    [string]$Scenario = "nextcloud",
    [string]$Manifest = "",
    [string]$ImageTag = "",
    [string]$NetworkName = "",
    [string]$NetworkSubnet = "",
    [string]$WorkDir = "",
    [string]$Dockerfile = "scripts/docker-raft-ingress/Dockerfile",
    [int]$TimeoutSeconds = 600,
    [switch]$SkipBuild,
    [switch]$SkipImageBuild,
    [switch]$KeepCluster,
    [switch]$KeepWorkload
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repoRoot

function New-ScenarioConfig {
    param([Parameter(Mandatory = $true)][string]$Name)
    switch ($Name.ToLowerInvariant()) {
        "placement" {
            return [pscustomobject]@{
                Slug = "placement"
                AppName = "placement-smoke"
                DefaultSubnet = "172.31.242.0/24"
                DefaultNetwork = "orch-raft-placement-smoke"
                DefaultManifest = "examples/integration/placement.orch"
                APIBase = 18401
                IngressBase = 18380
                Workloads = @("whoami")
                WorkloadNodes = @{ whoami = "node-b" }
                DescribeWorkload = "whoami"
            }
        }
        "nextcloud" {
            return [pscustomobject]@{
                Slug = "nextcloud"
                AppName = "nextcloud-smoke"
                DefaultSubnet = "172.31.240.0/24"
                DefaultNetwork = "orch-raft-nextcloud-smoke"
                DefaultManifest = "examples/integration/nextcloud.orch"
                APIBase = 18201
                IngressBase = 18180
                Workloads = @("postgres", "redis", "nextcloud")
                WorkloadNodes = @{ postgres = "node-b"; redis = "node-b"; nextcloud = "node-b" }
                DescribeWorkload = "nextcloud"
            }
        }
        "seaweed" {
            return [pscustomobject]@{
                Slug = "seaweed"
                AppName = "seaweed-smoke"
                DefaultSubnet = "172.31.241.0/24"
                DefaultNetwork = "orch-raft-seaweed-smoke"
                DefaultManifest = "examples/integration/seaweed.orch"
                APIBase = 18301
                IngressBase = 18280
                Workloads = @(
                    "seaweedmastera", "seaweedmasterb", "seaweedmasterc",
                    "seaweedvolumea", "seaweedvolumeb", "seaweedvolumec",
                    "seaweedfilera", "seaweedfilerb", "seaweedfilerc",
                    "seaweeds3", "seaweedwebdav", "seaweedadmin", "seaweedworker"
                )
                WorkloadNodes = @{
                    seaweedmastera = "node-a"; seaweedmasterb = "node-b"; seaweedmasterc = "node-c"
                    seaweedvolumea = "node-a"; seaweedvolumeb = "node-b"; seaweedvolumec = "node-c"
                    seaweedfilera = "node-a"; seaweedfilerb = "node-b"; seaweedfilerc = "node-c"
                    seaweeds3 = "node-b"; seaweedwebdav = "node-c"; seaweedadmin = "node-a"; seaweedworker = "node-b"
                }
                DescribeWorkload = "seaweedfilera"
            }
        }
        default {
            throw "Unsupported scenario: $Name"
        }
    }
}

function Get-SubnetPrefix {
    param([Parameter(Mandatory = $true)][string]$Subnet)
    if ($Subnet -notmatch '^(\d{1,3}\.\d{1,3}\.\d{1,3})\.0/24$') {
        throw "NetworkSubnet must be an IPv4 /24 subnet ending in .0/24, got '$Subnet'"
    }
    return $Matches[1]
}

$scenarioCfg = New-ScenarioConfig -Name $Scenario
if ([string]::IsNullOrWhiteSpace($ImageTag)) {
    $ImageTag = "orch-docker-raft-$($scenarioCfg.Slug):local"
}
$networkOverridden = $PSBoundParameters.ContainsKey("NetworkName")
if ([string]::IsNullOrWhiteSpace($NetworkName)) {
    $NetworkName = $scenarioCfg.DefaultNetwork
}
if ($networkOverridden -and -not $PSBoundParameters.ContainsKey("Manifest")) {
    throw "Custom -NetworkName requires a matching -Manifest because integration .orch files pin docker.network."
}
if ([string]::IsNullOrWhiteSpace($NetworkSubnet)) {
    $NetworkSubnet = $scenarioCfg.DefaultSubnet
}
if ([string]::IsNullOrWhiteSpace($WorkDir)) {
    $WorkDir = ".orch-docker-raft-$($scenarioCfg.Slug)"
}
if ([string]::IsNullOrWhiteSpace($Manifest)) {
    $Manifest = $scenarioCfg.DefaultManifest
}
$manifestPath = (Resolve-Path (Join-Path $repoRoot $Manifest)).Path

$networkPrefix = Get-SubnetPrefix -Subnet $NetworkSubnet
$nodes = @(
    @{ ID = "node-a"; Name = "orch-$($scenarioCfg.Slug)-node-a"; IP = "$networkPrefix.11"; APIHostPort = [int]$scenarioCfg.APIBase; IngressHostPort = [int]$scenarioCfg.IngressBase },
    @{ ID = "node-b"; Name = "orch-$($scenarioCfg.Slug)-node-b"; IP = "$networkPrefix.12"; APIHostPort = [int]$scenarioCfg.APIBase + 1; IngressHostPort = [int]$scenarioCfg.IngressBase + 1 },
    @{ ID = "node-c"; Name = "orch-$($scenarioCfg.Slug)-node-c"; IP = "$networkPrefix.13"; APIHostPort = [int]$scenarioCfg.APIBase + 2; IngressHostPort = [int]$scenarioCfg.IngressBase + 2 }
)

$apiPort = 17443
$raftPort = 7444
$ingressPort = 18080
$dnsListen = "0.0.0.0:53"
$namespace = "default"
$targetNodeID = "node-b"
$targetNode = $nodes | Where-Object { $_.ID -eq $targetNodeID } | Select-Object -First 1
$appName = [string]$scenarioCfg.AppName
$workloadNames = @($scenarioCfg.Workloads)
$containerNames = $workloadNames | ForEach-Object { "orch-$namespace-$_" }
$workRoot = Join-Path $repoRoot $WorkDir
$hostBinDir = Join-Path $workRoot "bin"
$linuxBinDir = Join-Path $workRoot "linux-bin"
$logDir = Join-Path $workRoot "logs"

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
    $found = $nodes | Where-Object { $_.ID -eq $NodeID } | Select-Object -First 1
    if ($null -eq $found) {
        throw "Node not found: $NodeID"
    }
    return $found
}

function Select-NodeExcept {
    param([string[]]$Excluded = @())
    $excludedSet = @{}
    foreach ($id in $Excluded) {
        if (-not [string]::IsNullOrWhiteSpace($id)) {
            $excludedSet[$id] = $true
        }
    }
    $found = $nodes | Where-Object { -not $excludedSet.ContainsKey($_.ID) } | Select-Object -First 1
    if ($null -eq $found) {
        throw "No node available after exclusions: $($Excluded -join ',')"
    }
    return $found
}

function Select-NonLeaderNode {
    param([Parameter(Mandatory = $true)][string]$LeaderID)
    return Select-NodeExcept -Excluded @($LeaderID)
}

function Wait-ControlNodeStopped {
    param([Parameter(Mandatory = $true)]$Node)
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        $running = & docker inspect -f "{{.State.Running}}" $Node.Name 2>$null
        if ($LASTEXITCODE -eq 0 -and (($running | Out-String).Trim()) -eq "false") {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for control-plane container $($Node.Name) to stop"
}

function Wait-RaftQuorumAfterNodeStop {
    param([Parameter(Mandatory = $true)]$StoppedNode)
    $aliveNodes = @($nodes | Where-Object { $_.ID -ne $StoppedNode.ID })
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        foreach ($node in $aliveNodes) {
            try {
                $status = Get-RaftStatus -Node $node
                $leaderID = Get-RaftLeaderID -Status $status
                if ([bool](Get-JSONProperty -Object $status -Name "ready" -Default $false) -and $leaderID -ne "") {
                    $leader = $aliveNodes | Where-Object { $_.ID -eq $leaderID } | Select-Object -First 1
                    if ($null -ne $leader) {
                        return $leader
                    }
                    return $node
                }
            }
            catch {
            }
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for Raft quorum after stopping $($StoppedNode.ID)"
}
function Expected-WorkloadNode {
    param([Parameter(Mandatory = $true)][string]$WorkloadName)
    $nodesByWorkload = $scenarioCfg.WorkloadNodes
    if ($null -ne $nodesByWorkload -and $nodesByWorkload.ContainsKey($WorkloadName)) {
        return [string]$nodesByWorkload[$WorkloadName]
    }
    return $targetNodeID
}
function Wait-AssignmentsRunning {
    param([Parameter(Mandatory = $true)]$LeaderNode)
    $leaderURL = Node-URL $LeaderNode
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $assignments = Invoke-CLIJson -ServerURL $leaderURL -Arguments @("get", "assignments", "--json")
        $running = 0
        foreach ($name in $workloadNames) {
            $key = "$namespace/$appName/$name"
            $expectedNode = Expected-WorkloadNode -WorkloadName $name
            $found = $assignments | Where-Object {
                $_.key -eq $key -and $_.node -eq $expectedNode -and $_.status -eq "running"
            } | Select-Object -First 1
            if ($null -ne $found) {
                $running++
            }
        }
        if ($running -eq $workloadNames.Count) {
            return
        }
        Start-Sleep -Seconds 1
    }
    throw "Timed out waiting for $($scenarioCfg.Slug) assignments to reach expected nodes"
}

function Wait-AssignmentRunningOn {
    param(
        [Parameter(Mandatory = $true)]$LeaderNode,
        [Parameter(Mandatory = $true)][string]$WorkloadName,
        [Parameter(Mandatory = $true)][string]$ExpectedNodeID
    )
    $leaderURL = Node-URL $LeaderNode
    $key = "$namespace/$appName/$WorkloadName"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $assignments = Invoke-CLIJson -ServerURL $leaderURL -Arguments @("get", "assignments", "--json")
        $found = $assignments | Where-Object {
            $_.key -eq $key -and $_.node -eq $ExpectedNodeID -and $_.status -eq "running"
        } | Select-Object -First 1
        if ($null -ne $found) {
            $address = [string](Get-JSONProperty -Object $found -Name "address" -Default "")
            if ($address -ne "") {
                return
            }
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for $key to run on $ExpectedNodeID"
}

function Wait-WhoamiIngress {
    param([Parameter(Mandatory = $true)]$IngressNode)
    $url = "http://127.0.0.1:$($IngressNode.IngressHostPort)/"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 5
            $body = [string]$resp.Content
            if ($resp.StatusCode -eq 200 -and $body.Contains("Hostname:")) {
                Write-Host "Whoami response from $url"
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

function Wait-PlacementIngress {
    param($IngressNodes = $nodes)
    foreach ($node in @($IngressNodes)) {
        Wait-WhoamiIngress -IngressNode $node
    }
}
function Wait-NextcloudIngress {
    param([Parameter(Mandatory = $true)]$IngressNode)
    $url = "http://127.0.0.1:$($IngressNode.IngressHostPort)/"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 10 -MaximumRedirection 5
            $body = [string]$resp.Content
            if ($resp.StatusCode -eq 200 -and $body.ToLowerInvariant().Contains("nextcloud")) {
                $preview = $body -replace "\s+", " "
                Write-Host "Nextcloud response from $url"
                Write-Host $preview.Substring(0, [Math]::Min(240, $preview.Length))
                return
            }
        }
        catch {
        }
        Start-Sleep -Seconds 2
    }
    throw "Timed out waiting for Nextcloud response through ingress $url"
}

function Wait-SeaweedRoundTrip {
    $payload = "orch seaweed smoke $([DateTime]::UtcNow.ToString('O'))"
    $path = "/orch-smoke.txt"
    $putURL = "http://127.0.0.1:$($nodes[0].IngressHostPort)$path"
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri $putURL -Method Put -Body $payload -ContentType "text/plain" -TimeoutSec 10 | Out-Null
            $ok = 0
            foreach ($node in $nodes) {
                $url = "http://127.0.0.1:$($node.IngressHostPort)$path"
                $resp = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 10
                if ($resp.StatusCode -eq 200 -and ([string]$resp.Content).Trim() -eq $payload) {
                    Write-Host "SeaweedFS response from $url"
                    Write-Host ([string]$resp.Content).Trim()
                    $ok++
                }
            }
            if ($ok -eq $nodes.Count) {
                return
            }
        }
        catch {
        }
        Start-Sleep -Seconds 2
    }
    throw "Timed out waiting for SeaweedFS filer round-trip through ingress"
}

function Wait-ScenarioIngress {
    switch ($scenarioCfg.Slug) {
        "placement" { Wait-PlacementIngress }
        "nextcloud" {
            foreach ($node in $nodes) {
                Wait-NextcloudIngress -IngressNode $node
            }
        }
        "seaweed" { Wait-SeaweedRoundTrip }
        default { throw "Unsupported scenario: $($scenarioCfg.Slug)" }
    }
}

function Invoke-PlacementOperations {
    param([Parameter(Mandatory = $true)]$LeaderNode)
    if ($scenarioCfg.Slug -ne "placement") {
        return
    }

    $workloadName = "whoami"
    $preferredNodeID = "node-b"
    $leaderURL = Node-URL $LeaderNode
    $migrateTarget = Select-NodeExcept -Excluded @($preferredNodeID)

    Write-Host ""
    Write-Host "Migrating $workloadName to $($migrateTarget.ID)..."
    Invoke-Checked $cliBin @("--server", $leaderURL, "migrate", "app", $appName, "--to", $migrateTarget.ID, "--workload", $workloadName, "-n", $namespace)
    Wait-AssignmentRunningOn -LeaderNode $LeaderNode -WorkloadName $workloadName -ExpectedNodeID $migrateTarget.ID
    Wait-PlacementIngress

    Write-Host ""
    Write-Host "Rebalancing $workloadName back to preferred node $preferredNodeID..."
    Invoke-Checked $cliBin @("--server", $leaderURL, "rebalance", "app", $appName, "--workload", $workloadName, "-n", $namespace)
    Wait-AssignmentRunningOn -LeaderNode $LeaderNode -WorkloadName $workloadName -ExpectedNodeID $preferredNodeID
    Wait-PlacementIngress

    $currentLeader = Find-LeaderNode
    $victim = Select-NonLeaderNode -LeaderID ([string]$currentLeader.ID)
    if ($victim.ID -ne $preferredNodeID) {
        $leaderURL = Node-URL $currentLeader
        Write-Host ""
        Write-Host "Migrating $workloadName to non-leader victim $($victim.ID) before failure simulation..."
        Invoke-Checked $cliBin @("--server", $leaderURL, "migrate", "app", $appName, "--to", $victim.ID, "--workload", $workloadName, "-n", $namespace)
        Wait-AssignmentRunningOn -LeaderNode $currentLeader -WorkloadName $workloadName -ExpectedNodeID $victim.ID
        Wait-PlacementIngress
    }

    Write-Host ""
    Write-Host "Stopping control-plane node $($victim.ID) to simulate node failure..."
    Invoke-Docker @("stop", $victim.Name)
    Wait-ControlNodeStopped -Node $victim
    $leaderAfterStop = Wait-RaftQuorumAfterNodeStop -StoppedNode $victim
    $survivor = $leaderAfterStop
    $survivorURL = Node-URL $survivor

    Write-Host "Raft leader after node stop: $($leaderAfterStop.ID)"
    Write-Host "Failing over $workloadName from $($victim.ID) to $($survivor.ID)..."
    Invoke-Checked $cliBin @("--server", $survivorURL, "failover", "app", $appName, "--to", $survivor.ID, "--workload", $workloadName, "-n", $namespace)
    Wait-AssignmentRunningOn -LeaderNode $survivor -WorkloadName $workloadName -ExpectedNodeID $survivor.ID
    $aliveNodes = @($nodes | Where-Object { $_.ID -ne $victim.ID })
    Wait-PlacementIngress -IngressNodes $aliveNodes
}
function Wait-AppDeleted {
    param([Parameter(Mandatory = $true)]$LeaderNode)
    $leaderURL = Node-URL $LeaderNode
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $apps = Invoke-CLIJson -ServerURL $leaderURL -Arguments @("get", "apps", "--json")
        $found = $apps | Where-Object { $_.name -eq $appName -and $_.namespace -eq $namespace } | Select-Object -First 1
        if ($null -eq $found) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for app $namespace/$appName to be deleted"
}

function Wait-WorkloadContainersRemoved {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $remaining = 0
        foreach ($name in $containerNames) {
            $ids = & docker ps -a --filter "name=^/$name$" --format "{{.ID}}"
            if ($LASTEXITCODE -ne 0) {
                throw "docker ps failed while checking $name"
            }
            if ((($ids | Out-String).Trim()) -ne "") {
                $remaining++
            }
        }
        if ($remaining -eq 0) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for workload containers to be removed"
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
        "--ip", $Node.IP,
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
        "--dns-listen", $dnsListen,
        "--dns-zone", "orch.local",
        "--dns-workload-nameserver", $targetNode.IP,
        "--dns-workload-upstream", "1.1.1.1",
        "--observability-prometheus-enabled=false",
        "--observability-otlp-enabled=false",
        "--log-level", "info"
    )
}

function Save-DockerLogs {
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
    foreach ($name in $containerNames) {
        try {
            $logPath = Join-Path $logDir "$name.docker.log"
            & docker logs $name *> $logPath
            if ($LASTEXITCODE -eq 0) {
                Write-Host "Saved logs for ${name}: $logPath"
            }
        }
        catch {
            Write-Warning "failed to collect logs for ${name}: $_"
        }
    }
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
            foreach ($name in $containerNames) {
                Remove-ContainerIfExists -Name $name
            }
        }
        Remove-NetworkIfExists
    }

    Invoke-Docker @("network", "create", "--subnet", $NetworkSubnet, $NetworkName)
    foreach ($node in $nodes) {
        Start-OrchNode -Node $node
    }
    foreach ($node in $nodes) {
        Wait-CLIReady -Node $node
    }
    Wait-RaftMembers

    $leader = Find-LeaderNode
    $leaderID = [string]$leader.ID
    $leaderURL = Node-URL $leader

    Write-Host "Scenario:             $($scenarioCfg.Slug)"
    Write-Host "Raft leader:          $leaderID"
    Write-Host "Workload DNS target:  $targetNodeID"
    Write-Host "Leader API:           $leaderURL"
    Write-Host "Target DNS:           $($targetNode.IP):53"
    Write-Host "Target ingress:       http://127.0.0.1:$($targetNode.IngressHostPort)/"
    Write-Host "Docker network:       $NetworkName ($NetworkSubnet)"
    Write-Host "Manifest:             $manifestPath"
    Write-Host ""

    Invoke-Checked $cliBin @("--server", $leaderURL, "apply", "--file", $manifestPath, "--watch", "--timeout", "$($TimeoutSeconds)s")
    Wait-AssignmentsRunning -LeaderNode $leader
    Wait-ScenarioIngress
    Invoke-PlacementOperations -LeaderNode $leader

    Invoke-Checked $cliBin @("--server", $leaderURL, "get", "apps")
    Invoke-Checked $cliBin @("--server", $leaderURL, "get", "assignments")
    Invoke-Checked $cliBin @("--server", $leaderURL, "describe", "workload", $scenarioCfg.DescribeWorkload, "--app", $appName, "-n", $namespace)

    if (-not $KeepWorkload) {
        Write-Host ""
        Write-Host "Deleting $($scenarioCfg.Slug) app..."
        Invoke-Checked $cliBin @("--server", $leaderURL, "delete", "app", $appName, "-n", $namespace)
        Wait-AppDeleted -LeaderNode $leader
        Wait-WorkloadContainersRemoved
    }

    Write-Host "Docker Raft $($scenarioCfg.Slug) smoke completed."
}
catch {
    Write-Warning $_
    Save-DockerLogs
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
        foreach ($name in $containerNames) {
            try {
                Remove-ContainerIfExists -Name $name
            }
            catch {
                Write-Warning $_
            }
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