package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedStorageSuiteCorrectness(t *testing.T) {
	samples, metrics, err := runGeneratedStorageSuite(filepath.Join(t.TempDir(), "artifacts"), 8, 5, 42, 10<<20, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 10 {
		t.Fatalf("got %d samples, want 10", len(samples))
	}
	if metrics["generated_eager_artifacts"].Value != 24 || metrics["generated_lazy_artifacts"].Value != 8 {
		t.Fatalf("unexpected artifact metrics: %#v", metrics)
	}
	if metrics["generated_eager_dedup_potential_bytes"].Value <= 0 || metrics["error_count"].Value != 0 {
		t.Fatalf("unexpected dedup/error metrics: %#v", metrics)
	}
}

func TestGeneratedStorageHonorsByteCeiling(t *testing.T) {
	_, _, err := runGeneratedStorageSuite(filepath.Join(t.TempDir(), "artifacts"), 2, 5, 42, 1, time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected physical-byte ceiling failure")
	}
}
