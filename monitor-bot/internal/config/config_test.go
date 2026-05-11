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
