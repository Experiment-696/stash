package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestScanSuiteCorrectness(t *testing.T) {
	samples, metrics, err := runScanSuite(filepath.Join(t.TempDir(), "library"), 202, 5, 42, 1<<20, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 5 {
		t.Fatalf("got %d samples, want 5", len(samples))
	}
	if samples[0].Operation != "initial_scan" || samples[1].Operation != "unchanged_rescan" {
		t.Fatalf("unexpected operations: %#v", samples)
	}
	if metrics["scan_files"].Value != 200 || metrics["error_count"].Value != 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
	for _, sample := range samples[1:] {
		if sample.Digest != samples[0].Digest {
			t.Fatal("unchanged scan digest changed")
		}
	}
}
