package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixtureDigestDeterministic(t *testing.T) {
	size := fixtureSizes["S"]
	a := fixtureDigest("S", 42, size)
	b := fixtureDigest("S", 42, size)
	if a != b {
		t.Fatalf("digest changed: %q != %q", a, b)
	}
	if a == fixtureDigest("S", 43, size) {
		t.Fatal("digest did not include seed")
	}
}

func TestRedactMountOptions(t *testing.T) {
	got := redactMountOptions("ro,vers=3.1.1,username=alice,password=hunter2,uid=1000")
	if strings.Contains(got, "alice") || strings.Contains(got, "hunter2") {
		t.Fatalf("secret-bearing option leaked: %q", got)
	}
	if got != "ro,vers=3.1.1,uid=1000" {
		t.Fatalf("unexpected redaction result: %q", got)
	}
}

func TestRunWritesBoundedManifest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "run")
	err := run([]string{"--suite", "fixture", "--fixture", "S", "--seed", "42", "--output", out, "--max-disk-bytes", "1048576", "--max-duration", "1m", "--share-mount-options", "ro,password=nope"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".stash-benchmark-run", "manifest.json", "fixture-manifest.json", "file-records.jsonl"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "nope") {
		t.Fatal("manifest contains redacted credential")
	}
}

func TestDeterministicRecordCoversEdgeCases(t *testing.T) {
	a := deterministicRecord("S", 42, 97, 1000, 1024)
	b := deterministicRecord("S", 42, 97, 1000, 1024)
	if a != b {
		t.Fatalf("record is not deterministic: %#v != %#v", a, b)
	}
	if !strings.Contains(a.Path, "測試") {
		t.Fatalf("deep Unicode edge case missing: %q", a.Path)
	}
	duplicate := deterministicRecord("S", 42, 53, 1000, 1024)
	if duplicate.DuplicateOf != 43 {
		t.Fatalf("duplicate relationship missing: %#v", duplicate)
	}
}

func TestRunRejectsApprovedLimitOverrun(t *testing.T) {
	err := run([]string{"--output", filepath.Join(t.TempDir(), "run"), "--max-disk-bytes", "26843545601"})
	if err == nil {
		t.Fatal("expected disk ceiling error")
	}
}

func TestUnrelatedHashLimitDoesNotBlockOtherSuites(t *testing.T) {
	out := filepath.Join(t.TempDir(), "run")
	err := run([]string{"--suite", "scan", "--fixture", "S", "--output", out, "--max-disk-bytes", "200000", "--repetitions", "5"})
	if err != nil {
		t.Fatalf("scan was blocked by an unrelated hashing flag: %v", err)
	}
}

func TestComparabilityReasons(t *testing.T) {
	a := manifest{SchemaVersion: "1", Suite: "fixture", Fixture: "S", FixtureDigest: "same", Environment: environment{OS: "linux", Architecture: "amd64", StorageProfile: "nvme"}}
	b := a
	if got := comparabilityReasons(a, b); len(got) != 0 {
		t.Fatalf("identical manifests not comparable: %v", got)
	}
	b.Environment.StorageProfile = "smb"
	if got := comparabilityReasons(a, b); len(got) != 1 || !strings.Contains(got[0], "storage_profile") {
		t.Fatalf("storage mismatch not detected: %v", got)
	}
}

func TestCompareDetectsRegression(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	candidate := filepath.Join(root, "candidate")
	if err := run([]string{"--suite", "fixture", "--fixture", "S", "--seed", "42", "--output", base, "--max-disk-bytes", "1048576", "--max-duration", "1m"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--suite", "fixture", "--fixture", "S", "--seed", "42", "--output", candidate, "--max-disk-bytes", "1048576", "--max-duration", "1m"}); err != nil {
		t.Fatal(err)
	}
	var s summary
	if err := readJSON(filepath.Join(candidate, "summary.json"), &s); err != nil {
		t.Fatal(err)
	}
	s.Metrics["record_bytes"] = metric{Value: s.Metrics["record_bytes"].Value * 1.2, Unit: "bytes", LowerIsBetter: true}
	if err := os.Remove(filepath.Join(candidate, "summary.json")); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(candidate, "summary.json"), s); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "comparison")
	if err := run([]string{"compare", "--baseline", base, "--candidate", candidate, "--output", out}); err != nil {
		t.Fatal(err)
	}
	var result comparison
	if err := readJSON(filepath.Join(out, "comparison.json"), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "regression" || !result.Metrics["record_bytes"].Regression {
		t.Fatalf("regression not detected: %#v", result)
	}
}

func TestCompareDetectsNewNonzeroRegression(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	candidate := filepath.Join(root, "candidate")
	for _, out := range []string{base, candidate} {
		if err := run([]string{"--suite", "fixture", "--fixture", "S", "--seed", "42", "--output", out, "--max-disk-bytes", "1048576", "--max-duration", "1m"}); err != nil {
			t.Fatal(err)
		}
	}
	for path, value := range map[string]float64{base: 0, candidate: 5} {
		var s summary
		summaryPath := filepath.Join(path, "summary.json")
		if err := readJSON(summaryPath, &s); err != nil {
			t.Fatal(err)
		}
		s.Metrics["error_count"] = metric{Value: value, Unit: "count", LowerIsBetter: true}
		if err := os.Remove(summaryPath); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(summaryPath, s); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(root, "comparison")
	if err := run([]string{"compare", "--baseline", base, "--candidate", candidate, "--output", out}); err != nil {
		t.Fatal(err)
	}
	var result comparison
	if err := readJSON(filepath.Join(out, "comparison.json"), &result); err != nil {
		t.Fatal(err)
	}
	got, ok := result.Metrics["error_count"]
	if !ok || !got.Regression || got.ChangeKind != "new_nonzero" || result.Status != "regression" {
		t.Fatalf("zero-baseline regression not detected: %#v", result)
	}
}

func TestMismatchWritesNonComparableArtifact(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	candidate := filepath.Join(root, "candidate")
	if err := run([]string{"--suite", "fixture", "--fixture", "S", "--output", base, "--max-disk-bytes", "1048576", "--storage-profile", "nvme"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--suite", "fixture", "--fixture", "S", "--output", candidate, "--max-disk-bytes", "1048576", "--storage-profile", "smb"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "comparison")
	if err := run([]string{"compare", "--baseline", base, "--candidate", candidate, "--output", out}); err != nil {
		t.Fatal(err)
	}
	var result comparison
	if err := readJSON(filepath.Join(out, "comparison.json"), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "non_comparable" || result.Comparable || len(result.Reasons) == 0 {
		t.Fatalf("mismatch was not labeled: %#v", result)
	}
}

func TestHashingSuiteDeterministicAndCorrect(t *testing.T) {
	samples, metrics, err := runHashingSuite(4096, 5, 42, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 10 {
		t.Fatalf("got %d samples, want 10", len(samples))
	}
	for i := 1; i < 5; i++ {
		if samples[i].Digest != samples[0].Digest {
			t.Fatal("MD5 digest changed between repetitions")
		}
		if samples[i+5].Digest != samples[5].Digest {
			t.Fatal("SHA-256 digest changed between repetitions")
		}
	}
	if metrics["error_count"].Value != 0 || metrics["sha256_throughput_median"].Value <= 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestMedian(t *testing.T) {
	if got := median([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("odd median=%v", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Fatalf("even median=%v", got)
	}
}

func TestCalculateStats(t *testing.T) {
	got := calculateStats([]float64{1, 2, 3, 4, 5})
	if got.Min != 1 || got.Max != 5 || got.Median != 3 || got.P95 != 5 {
		t.Fatalf("unexpected stats: %#v", got)
	}
	if got.CV <= 0 {
		t.Fatalf("expected positive CV: %#v", got)
	}
}

func TestVarianceGate(t *testing.T) {
	result := summary{Status: "pass", Metrics: map[string]metric{
		"scan_wall_cv": {Value: 0.11, Unit: "ratio", LowerIsBetter: true},
	}}
	if !metricsAboveVariance(result.Metrics) {
		t.Fatal("high variance was not detected")
	}
	applyVarianceGate(&result)
	if result.Status != "inconclusive" || len(result.Notes) != 1 {
		t.Fatalf("variance gate did not mark result inconclusive: %#v", result)
	}
}
