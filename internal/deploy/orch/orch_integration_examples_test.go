package orch_test

import "testing"

func TestIntegrationPlacementExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/integration/placement.orch")
	requireMetadata(t, app, "placement-smoke", "default")
	requireWorkloadCount(t, app, 1)
	if len(app.Volumes) != 1 {
		t.Fatalf("volumes = %+v", app.Volumes)
	}
	requireMount(t, workloadByName(t, app, "whoami"), "whoamiData", "/tmp/orch-data")
	requireIngressRoute(t, app, 0, "/", "whoami", "http")
	requireValidApp(t, app)
}
func TestIntegrationNextcloudExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/integration/nextcloud.orch")
	requireMetadata(t, app, "nextcloud-smoke", "default")
	if len(app.Volumes) != 3 {
		t.Fatalf("volumes = %+v", app.Volumes)
	}
	requireDependsOn(t, workloadByName(t, app, "nextcloud"), "postgres", "redis")
	requireMount(t, workloadByName(t, app, "postgres"), "postgresData", "/var/lib/postgresql/data")
	requireMount(t, workloadByName(t, app, "redis"), "redisData", "/data")
	requireMount(t, workloadByName(t, app, "nextcloud"), "nextcloudData", "/var/www/html")
	requireIngressRoute(t, app, 0, "/", "nextcloud", "http")
	requireValidApp(t, app)
}

func TestIntegrationSeaweedExample(t *testing.T) {
	app := loadAppFile(t, "../../../examples/integration/seaweed.orch")
	requireMetadata(t, app, "seaweed-smoke", "default")
	requireWorkloadCount(t, app, 13)
	if len(app.Volumes) != 3 {
		t.Fatalf("volumes = %+v", app.Volumes)
	}
	requireMount(t, workloadByName(t, app, "seaweedvolumea"), "seaweedVolumeAData", "/data")
	requireMount(t, workloadByName(t, app, "seaweedvolumeb"), "seaweedVolumeBData", "/data")
	requireMount(t, workloadByName(t, app, "seaweedvolumec"), "seaweedVolumeCData", "/data")
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
