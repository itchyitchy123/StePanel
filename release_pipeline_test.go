package main

import "testing"

func TestPipelineResourceLimitsUseSiteProfile(t *testing.T) {
	app := &App{Resources: &ResourceStore{values: map[string]ResourceProfile{
		"demo": {Site: "demo", CPUPercent: 200, CPUWeight: 100, MemoryHighMB: 900, MemoryMB: 1024, IOWeight: 100, TasksMax: 256, PHPWorkers: 16},
	}}}
	cpu, memory, tasks := app.pipelineResourceLimits("demo")
	if cpu != 200 || memory != 1024 || tasks != 256 {
		t.Fatalf("limits = %d%%, %d MB, %d tasks", cpu, memory, tasks)
	}
}

func TestPipelineResourceLimitsBoundLegacySite(t *testing.T) {
	app := &App{Resources: &ResourceStore{values: map[string]ResourceProfile{}}}
	cpu, memory, tasks := app.pipelineResourceLimits("legacy")
	if cpu != 100 || memory != 512 || tasks != 128 {
		t.Fatalf("legacy limits = %d%%, %d MB, %d tasks", cpu, memory, tasks)
	}
}
