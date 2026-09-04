package manager

import (
	"github.com/stashapp/stash/internal/manager/config"
	"testing"
)

func TestCamGirlFinderConfigValidationAndAdapter(t *testing.T) {
	valid := config.DefaultCamGirlFinderConfig()
	if err := ValidateCamGirlFinderConfig(valid); err != nil {
		t.Fatalf("disabled defaults invalid: %v", err)
	}
	valid.Enabled = true
	if err := ValidateCamGirlFinderConfig(valid); err != nil {
		t.Fatalf("enabled defaults invalid: %v", err)
	}
	adapter := AdapterCamGirlFinderConfig(valid)
	if adapter.BaseURL != CamGirlFinderBaseURL || adapter.Credential != "" {
		t.Fatalf("unexpected adapter endpoint or credential: %#v", adapter)
	}
	for _, mutate := range []func(*config.CamGirlFinderConfig){
		func(v *config.CamGirlFinderConfig) { v.RequestIntervalMS = 99 },
		func(v *config.CamGirlFinderConfig) { v.TimeoutSeconds = 121 },
		func(v *config.CamGirlFinderConfig) { v.ResultLimit = 101 },
	} {
		bad := valid
		mutate(&bad)
		if err := ValidateCamGirlFinderConfig(bad); err == nil {
			t.Fatalf("invalid config accepted: %#v", bad)
		}
	}
}
