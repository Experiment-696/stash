package manager

import (
	"fmt"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel/camgirlfinder"
	"time"
)

const CamGirlFinderBaseURL = "https://api.camgirlfinder.net"

func ValidateCamGirlFinderConfig(value config.CamGirlFinderConfig) error {
	if value.RequestIntervalMS < 100 || value.RequestIntervalMS > 60000 {
		return fmt.Errorf("request interval must be 100-60000 ms")
	}
	if value.TimeoutSeconds < 1 || value.TimeoutSeconds > 120 {
		return fmt.Errorf("timeout must be 1-120 seconds")
	}
	if value.ResultLimit < 1 || value.ResultLimit > 100 {
		return fmt.Errorf("result limit must be 1-100")
	}
	_, err := camgirlfinder.New(AdapterCamGirlFinderConfig(value), nil)
	if !value.Enabled && err == camgirlfinder.ErrDisabled {
		return nil
	}
	return err
}
func AdapterCamGirlFinderConfig(value config.CamGirlFinderConfig) camgirlfinder.Config {
	return camgirlfinder.Config{Enabled: value.Enabled, BaseURL: CamGirlFinderBaseURL, UserAgent: "Stash CS-04A CamGirlFinder/1.0", RequestsPerSecond: 1000 / float64(value.RequestIntervalMS), Timeout: time.Duration(value.TimeoutSeconds) * time.Second, MaxResults: value.ResultLimit}
}
