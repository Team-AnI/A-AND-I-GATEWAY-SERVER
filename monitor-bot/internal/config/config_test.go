package config

import "testing"

func TestDefaultReportLogGroup(t *testing.T) {
	t.Setenv("LOG_GROUP_REPORT", "")

	cfg := Load()
	if cfg.LogGroups["report"] != "/a-and-i/prod/report" {
		t.Fatalf("unexpected report log group: %q", cfg.LogGroups["report"])
	}
}

func TestReportLogGroupOverride(t *testing.T) {
	t.Setenv("LOG_GROUP_REPORT", "/custom/report")

	cfg := Load()
	if cfg.LogGroups["report"] != "/custom/report" {
		t.Fatalf("unexpected overridden report log group: %q", cfg.LogGroups["report"])
	}
}

func TestOpsAdminConfigReusesExistingServiceURIAndToken(t *testing.T) {
	t.Setenv("AUTH_SERVICE_URI", "http://auth.internal:8080")
	t.Setenv("REPORT_SERVICE_URI", "http://report.internal:8080")
	t.Setenv("OPS_ADMIN_REFRESH_TOKEN", "refresh-token")

	cfg := Load()
	if cfg.AuthServiceURI != "http://auth.internal:8080" {
		t.Fatalf("AUTH_SERVICE_URI was not loaded: %q", cfg.AuthServiceURI)
	}
	if cfg.ReportServiceURI != "http://report.internal:8080" {
		t.Fatalf("REPORT_SERVICE_URI was not loaded: %q", cfg.ReportServiceURI)
	}
	if cfg.OpsAdminRefreshToken != "refresh-token" {
		t.Fatal("OPS_ADMIN_REFRESH_TOKEN was not loaded")
	}
}

func TestOpsAdminConfigUsesRefreshTokenOnly(t *testing.T) {
	t.Setenv("OPS_ADMIN_REFRESH_TOKEN", "refresh-token")

	cfg := Load()
	if cfg.OpsAdminRefreshToken != "refresh-token" {
		t.Fatal("OPS_ADMIN_REFRESH_TOKEN was not preferred")
	}
}

func TestServiceRegistryAlwaysContainsOperationalServices(t *testing.T) {
	registry := BuildServiceRegistry(
		map[string]string{"gateway": "/a-and-i/gateway", "report": "/a-and-i/prod/report"},
		map[string]string{"gateway": "http://gateway:9090/actuator/health/readiness"},
	)
	want := []string{"gateway", "auth", "report", "online-judge", "post"}
	if len(registry) != len(want) {
		t.Fatalf("unexpected registry length: %d", len(registry))
	}
	for i, service := range registry {
		if service.Name != want[i] {
			t.Fatalf("registry order mismatch at %d: got %s want %s", i, service.Name, want[i])
		}
	}
	if registry[1].Enabled {
		t.Fatal("auth should be disabled when both health URL and log group are missing")
	}
	if !registry[2].Enabled || registry[2].HealthURL != "" || registry[2].LogGroup != "/a-and-i/prod/report" {
		t.Fatalf("unexpected report registry entry: %#v", registry[2])
	}
}
