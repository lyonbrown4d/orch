package api

// HTTP path constants (server base path is /api via httpx.WithBasePath).
const (
	BasePath = "/api"

	PathReady              = BasePath + "/ready"
	PathHealth             = BasePath + "/health"
	PathV1Diagnostics      = BasePath + "/v1/diagnostics"
	PathV1Hostinfo         = BasePath + "/v1/hostinfo"
	PathV1RuntimeProviders = BasePath + "/v1/system/runtimes"
	PathV1Apps             = BasePath + "/v1/apps"
	PathV1Workloads        = BasePath + "/v1/workloads"
	PathV1Assignments      = BasePath + "/v1/assignments"
	PathV1Volumes          = BasePath + "/v1/volumes"
	PathV1Deploy           = BasePath + "/v1/deploy"
	PathV1DeployDelete     = BasePath + "/v1/deploy"
	PathV1DeployFailover   = BasePath + "/v1/deploy"
	PathV1DeployMigrate    = BasePath + "/v1/deploy"
	PathV1DeployRebalance  = BasePath + "/v1/deploy"
	PathV1DeployRestart    = BasePath + "/v1/deploy"
	PathV1DeployRollback   = BasePath + "/v1/deploy"
	PathV1DeployStart      = BasePath + "/v1/deploy"
	PathV1DeployStop       = BasePath + "/v1/deploy"
	PathV1DeploySource     = BasePath + "/v1/deploy/source"
	PathV1WorkerDeploy     = BasePath + "/v1/worker/deploy"
	PathV1WorkerStop       = BasePath + "/v1/worker/stop"
	PathV1RaftStatus       = BasePath + "/v1/raft/status"
	PathV1RaftMembers      = BasePath + "/v1/raft/members"
	PathV1OrchVPNBootstrap = BasePath + "/v1/orch-vpn/bootstrap"
)
