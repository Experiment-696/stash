package main

import (
	"testing"
	"time"
)

func TestPHashSuiteDeterministic(t *testing.T) {
	samples, metrics, err := runPHashSuite(64, 5, 42, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 5 {
		t.Fatalf("got %d samples, want 5", len(samples))
	}
	for _, sample := range samples[1:] {
		if sample.Hash != samples[0].Hash {
			t.Fatal("pHash changed between repetitions")
		}
	}
	if metrics["error_count"].Value != 0 || metrics["phash_pixels_per_second"].Value <= 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestSyntheticImageSeedChangesContent(t *testing.T) {
	a := syntheticImage(32, 1).At(10, 10)
	b := syntheticImage(32, 2).At(10, 10)
	if a == b {
		t.Fatal("seed did not alter synthetic image")
	}
}
