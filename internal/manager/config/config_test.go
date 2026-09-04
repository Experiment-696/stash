package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_GetAllPluginConfiguration(t *testing.T) {
	i := InitializeEmpty()

	assert.Equal(t, i.GetAllPluginConfiguration(), map[string]map[string]interface{}{})

	i.SetPluginConfiguration("plugin1", map[string]interface{}{"key1": "value1"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
	}, i.GetAllPluginConfiguration())

	i.SetPluginConfiguration("plugin2", map[string]interface{}{"key2": "value2"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
		"plugin2": {"key2": "value2"},
	}, i.GetAllPluginConfiguration())

	// ensure SetPluginConfiguration overwrites existing configuration
	i.SetPluginConfiguration("plugin2", map[string]interface{}{"key3": "value3"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
		"plugin2": {"key3": "value3"},
	}, i.GetAllPluginConfiguration())
}

func TestCamGirlFinderConfigDefaultsAndRoundTrip(t *testing.T) {
	i := InitializeEmpty()
	want := DefaultCamGirlFinderConfig()
	if want.Enabled || want.RequestIntervalMS != 1000 || want.TimeoutSeconds != 15 || want.ResultLimit != 25 {
		t.Fatalf("unsafe defaults: %#v", want)
	}
	if got := i.GetCamGirlFinderConfig(); got != want {
		t.Fatalf("defaults=%#v want=%#v", got, want)
	}
	want = CamGirlFinderConfig{Enabled: true, RequestIntervalMS: 2500, TimeoutSeconds: 20, ResultLimit: 10}
	i.SetCamGirlFinderConfig(want)
	if got := i.main.Int(CamGirlFinder + ".request_interval_ms"); got != want.RequestIntervalMS {
		t.Fatalf("canonical persisted request interval=%d want=%d", got, want.RequestIntervalMS)
	}
	if i.main.Exists(CamGirlFinder + ".requestintervalms") {
		t.Fatal("collapsed requestintervalms key persisted")
	}
	if got := i.GetCamGirlFinderConfig(); got != want {
		t.Fatalf("round trip=%#v want=%#v", got, want)
	}
}

func TestCompletedRecordingImportConfigDefaultsDisabledAndRoundTrips(t *testing.T) {
	i := InitializeEmpty()
	if got := i.GetCompletedRecordingImportConfig(); got != (CompletedRecordingImportConfig{}) {
		t.Fatalf("unsafe defaults: %#v", got)
	}
	want := CompletedRecordingImportConfig{Enabled: true, Root: "/explicit/library/root"}
	i.SetCompletedRecordingImportConfig(want)
	if got := i.GetCompletedRecordingImportConfig(); got != want {
		t.Fatalf("round trip=%#v want=%#v", got, want)
	}
	if !i.main.Exists(CompletedRecordingImport+".enabled") || !i.main.Exists(CompletedRecordingImport+".root") {
		t.Fatal("canonical completed-recording config keys were not persisted")
	}
}
