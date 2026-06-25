package orch_test

import "testing"

func TestIntegrationNextcloudExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/integration/nextcloud.orch")
	requireMetadata(t, app, "nextcloud-smoke", "default")
	requireDependsOn(t, workloadByName(t, app, "nextcloud"), "postgres", "redis")
	requireIngressRoute(t, app, 0, "/", "nextcloud", "http")
	requireValidApp(t, app)
}

func TestIntegrationSeaweedExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/integration/seaweed.orch")
	requireMetadata(t, app, "seaweed-smoke", "default")
	requireWorkloadCount(t, app, 13)
	requireDependsOn(t, workloadByName(t, app, "seaweedmasterb"), "seaweedmastera")
	requireDependsOn(t, workloadByName(t, app, "seaweedmasterc"), "seaweedmastera", "seaweedmasterb")
	requireDependsOn(t, workloadByName(t, app, "seaweedvolumea"), "seaweedmastera", "seaweedmasterb", "seaweedmasterc")
	requireDependsOn(t, workloadByName(t, app, "seaweedfilera"), "seaweedmastera", "seaweedmasterb", "seaweedmasterc", "seaweedvolumea", "seaweedvolumeb", "seaweedvolumec")
	requireDependsOn(t, workloadByName(t, app, "seaweeds3"), "seaweedfilera", "seaweedfilerb", "seaweedfilerc")
	requireDependsOn(t, workloadByName(t, app, "seaweedadmin"), "seaweedmastera", "seaweedmasterb", "seaweedmasterc", "seaweedfilera")
	requireIngressRoute(t, app, 0, "/s3", "seaweeds3", "http")
	requireIngressRoute(t, app, 1, "/webdav", "seaweedwebdav", "http")
	requireIngressRoute(t, app, 2, "/", "seaweedfilera", "http")
	requireValidApp(t, app)
}
