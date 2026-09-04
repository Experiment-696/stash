package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type backgroundPolicyDocument struct {
	Surfaces []struct {
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Capability string `json:"capability"`
	} `json:"surfaces"`
}

func TestBackgroundExecutionEntryPointsMatchCapabilitiesAndSnapshotRules(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	policyBytes, err := os.ReadFile(filepath.Join(root, "internal", "authz", "graphql_policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy backgroundPolicyDocument
	if err := json.Unmarshal(policyBytes, &policy); err != nil {
		t.Fatal(err)
	}
	capabilities := map[string]string{}
	for _, surface := range policy.Surfaces {
		capabilities[surface.Name] = surface.Capability
	}
	required := map[string]string{
		"metadataScan":           "automation.run",
		"metadataGenerate":       "automation.run",
		"metadataClean":          "automation.run",
		"metadataCleanGenerated": "automation.run",
		"runPluginTask":          "extension.manage",
		"runPluginOperation":     "extension.manage",
		"reloadPlugins":          "extension.manage",
		"stopJob":                "job.manage",
		"jobQueue":               "job.read",
		"jobsSubscribe":          "job.read",
	}
	for operation, capability := range required {
		if capabilities[operation] != capability {
			t.Errorf("%s capability = %q, want %q", operation, capabilities[operation], capability)
		}
	}

	pluginTask, err := os.ReadFile(filepath.Join(root, "internal", "manager", "task_plugin.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(pluginTask)
	create := strings.Index(source, "s.PluginCache.CreateTask(ctx")
	queue := strings.Index(source, "s.JobManager.Add(ctx")
	if create < 0 || queue < 0 || create > queue {
		t.Fatal("plugin task must snapshot its authenticated connection before queueing")
	}
	if strings.Contains(source[queue:], "CreateTask(ctx") {
		t.Fatal("queued plugin execution retains the originating request context")
	}
}
