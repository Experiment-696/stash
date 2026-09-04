// Command benchmark provides the reproducible Phase 1A benchmark harness.
package main

import (
	"bufio"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const schemaVersion = "1"

type fixtureSize struct {
	Objects int   `json:"objects"`
	Files   int   `json:"files"`
	Logical int64 `json:"logical_bytes"`
}

var fixtureSizes = map[string]fixtureSize{
	"S": {Objects: 1_000, Files: 100, Logical: 10 << 30},
	"M": {Objects: 100_000, Files: 10_000, Logical: 100 << 30},
	"L": {Objects: 1_000_000, Files: 100_000, Logical: 25 << 30},
}

type manifest struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	CreatedUTC    string            `json:"created_utc"`
	Suite         string            `json:"suite"`
	Fixture       string            `json:"fixture"`
	Seed          int64             `json:"seed"`
	FixtureDigest string            `json:"fixture_digest"`
	Environment   environment       `json:"environment"`
	Limits        limits            `json:"limits"`
	Labels        map[string]string `json:"labels,omitempty"`
}

type environment struct {
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	LogicalCPUs      int    `json:"logical_cpus"`
	GoVersion        string `json:"go_version"`
	StorageProfile   string `json:"storage_profile"`
	Filesystem       string `json:"filesystem,omitempty"`
	ShareProtocol    string `json:"share_protocol,omitempty"`
	ShareMountOption string `json:"share_mount_options,omitempty"`
}

type limits struct {
	MaxPhysicalBytes int64  `json:"max_physical_bytes"`
	MaxWallTime      string `json:"max_wall_time"`
}

type fixtureManifest struct {
	SchemaVersion string      `json:"schema_version"`
	Name          string      `json:"name"`
	Seed          int64       `json:"seed"`
	Size          fixtureSize `json:"size"`
	Digest        string      `json:"digest"`
	RecordsDigest string      `json:"records_digest"`
	RecordBytes   int64       `json:"record_bytes"`
}

type fixtureRecord struct {
	ID           int    `json:"id"`
	ObjectID     int    `json:"object_id"`
	Path         string `json:"path"`
	LogicalBytes int64  `json:"logical_bytes"`
	Kind         string `json:"kind"`
	Missing      bool   `json:"missing,omitempty"`
	DuplicateOf  int    `json:"duplicate_of,omitempty"`
}

type metric struct {
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`
	LowerIsBetter bool    `json:"lower_is_better"`
}

type summary struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Status        string            `json:"status"`
	Correct       bool              `json:"correct"`
	Metrics       map[string]metric `json:"metrics"`
	Notes         []string          `json:"notes,omitempty"`
}

type comparisonMetric struct {
	BaselineValue  float64  `json:"baseline_value"`
	CandidateValue float64  `json:"candidate_value"`
	Unit           string   `json:"unit"`
	ChangeKind     string   `json:"change_kind"`
	ChangePercent  *float64 `json:"change_percent,omitempty"`
	Regression     bool     `json:"regression"`
}

type comparison struct {
	SchemaVersion  string                      `json:"schema_version"`
	BaselineRunID  string                      `json:"baseline_run_id"`
	CandidateRunID string                      `json:"candidate_run_id"`
	Comparable     bool                        `json:"comparable"`
	Forced         bool                        `json:"forced"`
	Status         string                      `json:"status"`
	Reasons        []string                    `json:"reasons,omitempty"`
	Metrics        map[string]comparisonMetric `json:"metrics"`
}

type hashSample struct {
	Algorithm string  `json:"algorithm"`
	Bytes     int64   `json:"bytes"`
	Seconds   float64 `json:"seconds"`
	Digest    string  `json:"digest"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "compare" {
		return runCompare(args[1:])
	}
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	suite := fs.String("suite", "manifest", "suite to prepare (manifest or fixture)")
	fixture := fs.String("fixture", "S", "fixture size: S, M, or L")
	output := fs.String("output", "", "new run output directory (required)")
	seed := fs.Int64("seed", 20260715, "deterministic fixture seed")
	maxDisk := fs.Int64("max-disk-bytes", 25<<30, "maximum physical bytes for this run")
	maxTime := fs.Duration("max-duration", 2*time.Hour, "maximum wall time for this run")
	storage := fs.String("storage-profile", "unspecified", "nvme, hdd, smb, nfs, or another operator label")
	filesystem := fs.String("filesystem", "", "operator-observed filesystem type")
	shareProtocol := fs.String("share-protocol", "", "operator-observed share protocol")
	shareOptions := fs.String("share-mount-options", "", "redacted share mount options")
	hashBytes := fs.Int64("hash-bytes", 1<<20, "synthetic bytes per hashing sample")
	imageSize := fs.Int("image-size", 512, "synthetic square image dimension for pHash")
	baseURL := fs.String("base-url", "", "localhost URL for browser suite")
	chromePath := fs.String("chrome-path", "", "optional Chromium/Chrome executable path")
	repetitions := fs.Int("repetitions", 5, "measured repetitions after one warm-up")
	if err := fs.Parse(args); err != nil {
		return err
	}

	name := strings.ToUpper(*fixture)
	size, ok := fixtureSizes[name]
	if !ok {
		return fmt.Errorf("unknown fixture %q (want S, M, or L)", *fixture)
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	if *maxDisk <= 0 || *maxDisk > 25<<30 {
		return errors.New("--max-disk-bytes must be between 1 and the approved 25 GiB ceiling")
	}
	if *maxTime <= 0 || *maxTime > 2*time.Hour {
		return errors.New("--max-duration must be between 1ns and the approved 2h ceiling")
	}
	if *suite == "hashing" && (*hashBytes <= 0 || *hashBytes > *maxDisk) {
		return errors.New("--hash-bytes must be positive and no larger than --max-disk-bytes")
	}
	if *repetitions < 5 || *repetitions > 100 {
		return errors.New("--repetitions must be between 5 and 100")
	}

	out, err := createRunDirectory(*output)
	if err != nil {
		return err
	}
	digest := fixtureDigest(name, *seed, size)
	runID := filepath.Base(out)
	m := manifest{
		SchemaVersion: schemaVersion,
		RunID:         runID,
		CreatedUTC:    time.Now().UTC().Format(time.RFC3339Nano),
		Suite:         *suite,
		Fixture:       name,
		Seed:          *seed,
		FixtureDigest: digest,
		Environment: environment{
			OS: runtime.GOOS, Architecture: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(),
			StorageProfile: *storage, Filesystem: *filesystem, ShareProtocol: *shareProtocol, ShareMountOption: redactMountOptions(*shareOptions),
		},
		Limits: limits{MaxPhysicalBytes: *maxDisk, MaxWallTime: maxTime.String()},
	}
	if err := writeJSON(filepath.Join(out, "manifest.json"), m); err != nil {
		return err
	}
	started := time.Now()
	result := summary{SchemaVersion: schemaVersion, RunID: runID, Status: "prepared", Correct: true, Metrics: map[string]metric{}}
	if *suite == "fixture" {
		recordsDigest, recordBytes, err := writeFixtureRecords(filepath.Join(out, "file-records.jsonl"), name, *seed, size, *maxDisk, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		fm := fixtureManifest{SchemaVersion: schemaVersion, Name: name, Seed: *seed, Size: size, Digest: digest, RecordsDigest: recordsDigest, RecordBytes: recordBytes}
		if err := writeJSON(filepath.Join(out, "fixture-manifest.json"), fm); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics["wall_time"] = metric{Value: time.Since(started).Seconds(), Unit: "seconds", LowerIsBetter: true}
		result.Metrics["record_bytes"] = metric{Value: float64(recordBytes), Unit: "bytes", LowerIsBetter: true}
		result.Metrics["records"] = metric{Value: float64(size.Files), Unit: "count", LowerIsBetter: false}
	} else if *suite == "hashing" {
		samples, metrics, err := runHashingSuite(*hashBytes, *repetitions, *seed, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			samples, metrics, err = runHashingSuite(*hashBytes, *repetitions, *seed, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "raw-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Synthetic deterministic byte stream; no real media read", "BLAKE3 is deferred until the approved implementation dependency is available"}
	} else if *suite == "sqlite" {
		dbPath := filepath.Join(out, "benchmark.sqlite")
		samples, metrics, err := runSQLiteSuite(dbPath, size.Files, *repetitions, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			if err := removeSQLiteRunFiles(dbPath); err != nil {
				return err
			}
			samples, metrics, err = runSQLiteSuite(dbPath, size.Files, *repetitions, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "sqlite-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Synthetic isolated SQLite database; no user database opened"}
	} else if *suite == "scan" {
		libraryPath := filepath.Join(out, "synthetic-library")
		samples, metrics, err := runScanSuite(libraryPath, size.Files, *repetitions, *seed, *maxDisk, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			samples, metrics, err = runScanSuite(libraryPath, size.Files, *repetitions, *seed, *maxDisk, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "scan-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Run-owned synthetic filesystem only; no configured media library accessed"}
	} else if *suite == "phash" {
		samples, metrics, err := runPHashSuite(*imageSize, *repetitions, *seed, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			samples, metrics, err = runPHashSuite(*imageSize, *repetitions, *seed, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "phash-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Generated non-explicit gradient image only; no real media decoded", "Video pHash/FFmpeg sampling remains pending"}
	} else if *suite == "generated-storage" {
		storageRoot := filepath.Join(out, "generated-artifacts")
		samples, metrics, err := runGeneratedStorageSuite(storageRoot, size.Files, *repetitions, *seed, *maxDisk, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			samples, metrics, err = runGeneratedStorageSuite(storageRoot, size.Files, *repetitions, *seed, *maxDisk, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "generated-storage-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Run-owned synthetic PNG artifacts only; no source media decoded or deleted"}
	} else if *suite == "browser" {
		if *baseURL == "" {
			return errors.New("browser suite requires --base-url")
		}
		samples, metrics, err := runBrowserSuite(*baseURL, *chromePath, *repetitions, time.Now().Add(*maxTime))
		if err != nil {
			return err
		}
		if metricsAboveVariance(metrics) {
			time.Sleep(100 * time.Millisecond)
			samples, metrics, err = runBrowserSuite(*baseURL, *chromePath, *repetitions, time.Now().Add(*maxTime))
			if err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(out, "browser-samples.json"), samples); err != nil {
			return err
		}
		result.Status = "pass"
		result.Metrics = metrics
		result.Notes = []string{"Fresh temporary browser profile per repetition", "Localhost-only single-identity baseline; role/account-switch scenarios begin after P1A-11"}
	} else if *suite != "manifest" {
		return fmt.Errorf("suite %q is not implemented yet", *suite)
	}
	applyVarianceGate(&result)
	return writeJSON(filepath.Join(out, "summary.json"), result)
}

func metricsAboveVariance(metrics map[string]metric) bool {
	for name, measured := range metrics {
		if strings.HasSuffix(name, "_cv") && measured.Value > 0.10 {
			return true
		}
	}
	return false
}

func applyVarianceGate(result *summary) {
	for name, measured := range result.Metrics {
		if strings.HasSuffix(name, "_cv") && measured.Value > 0.10 {
			result.Status = "inconclusive"
			result.Notes = append(result.Notes, name+" remained above the approved 10% variance threshold after retry")
		}
	}
}

func runHashingSuite(byteCount int64, repetitions int, seed int64, deadline time.Time) ([]hashSample, map[string]metric, error) {
	algorithms := []struct {
		name string
		new  func() hash.Hash
	}{{name: "md5", new: md5.New}, {name: "sha256", new: sha256.New}}
	var samples []hashSample
	metrics := map[string]metric{}
	for _, algorithm := range algorithms {
		measured, durations, err := measureHashAlgorithm(algorithm.name, algorithm.new, byteCount, repetitions, seed, deadline)
		if err != nil {
			return nil, nil, err
		}
		stats := calculateStats(durations)
		samples = append(samples, measured...)
		prefix := algorithm.name + "_wall_"
		metrics[prefix+"min"] = metric{Value: stats.Min, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"max"] = metric{Value: stats.Max, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"median"] = metric{Value: stats.Median, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"p95"] = metric{Value: stats.P95, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"cv"] = metric{Value: stats.CV, Unit: "ratio", LowerIsBetter: true}
		metrics[algorithm.name+"_throughput_median"] = metric{Value: (float64(byteCount) / (1024 * 1024)) / stats.Median, Unit: "MiB/s", LowerIsBetter: false}
	}
	metrics["error_count"] = metric{Value: 0, Unit: "count", LowerIsBetter: true}
	return samples, metrics, nil
}

func measureHashAlgorithm(name string, newHash func() hash.Hash, byteCount int64, repetitions int, seed int64, deadline time.Time) ([]hashSample, []float64, error) {
	var expected string
	var samples []hashSample
	durations := make([]float64, 0, repetitions)
	for repetition := -1; repetition < repetitions; repetition++ {
		if time.Now().After(deadline) {
			return nil, nil, errors.New("hashing suite exceeded the configured wall-time ceiling")
		}
		h := newHash()
		started := time.Now()
		written, err := io.CopyN(h, newDeterministicReader(seed), byteCount)
		elapsed := time.Since(started).Seconds()
		if err != nil {
			return nil, nil, err
		}
		if written != byteCount {
			return nil, nil, fmt.Errorf("%s hashed %d bytes, want %d", name, written, byteCount)
		}
		digest := hex.EncodeToString(h.Sum(nil))
		if expected == "" {
			expected = digest
		} else if digest != expected {
			return nil, nil, fmt.Errorf("%s correctness gate failed: digest changed between repetitions", name)
		}
		if repetition >= 0 {
			samples = append(samples, hashSample{Algorithm: name, Bytes: written, Seconds: elapsed, Digest: digest})
			durations = append(durations, elapsed)
		}
	}
	return samples, durations, nil
}

type sampleStats struct{ Min, Max, Median, P95, CV float64 }

func calculateStats(values []float64) sampleStats {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	medianValue := median(ordered)
	mean := 0.0
	for _, value := range ordered {
		mean += value
	}
	mean /= float64(len(ordered))
	variance := 0.0
	for _, value := range ordered {
		delta := value - mean
		variance += delta * delta
	}
	variance /= float64(len(ordered))
	cv := 0.0
	if mean != 0 {
		cv = math.Sqrt(variance) / mean
	}
	p95Index := int(math.Ceil(float64(len(ordered))*0.95)) - 1
	return sampleStats{Min: ordered[0], Max: ordered[len(ordered)-1], Median: medianValue, P95: ordered[p95Index], CV: cv}
}

type deterministicReader struct {
	state uint64
}

func newDeterministicReader(seed int64) *deterministicReader {
	return &deterministicReader{state: uint64(seed) ^ 0x9e3779b97f4a7c15}
}

func (r *deterministicReader) Read(p []byte) (int, error) {
	for i := range p {
		r.state ^= r.state << 13
		r.state ^= r.state >> 7
		r.state ^= r.state << 17
		p[i] = byte(r.state)
	}
	return len(p), nil
}

func median(values []float64) float64 {
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func runCompare(args []string) error {
	fs := flag.NewFlagSet("benchmark compare", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "baseline run directory")
	candidatePath := fs.String("candidate", "", "candidate run directory")
	output := fs.String("output", "", "new comparison output directory")
	forceCompare := fs.Bool("force-compare", false, "compare mismatched manifests while preserving mismatch reasons")
	threshold := fs.Float64("threshold-percent", 10, "approved regression alert threshold")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" || *candidatePath == "" || *output == "" {
		return errors.New("compare requires --baseline, --candidate, and --output")
	}
	if *threshold <= 0 || *threshold > 100 {
		return errors.New("--threshold-percent must be greater than 0 and at most 100")
	}
	baseManifest, baseSummary, err := readRun(*baselinePath)
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	candidateManifest, candidateSummary, err := readRun(*candidatePath)
	if err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	reasons := comparabilityReasons(baseManifest, candidateManifest)
	out, err := createRunDirectory(*output)
	if err != nil {
		return err
	}
	result := comparison{
		SchemaVersion: schemaVersion, BaselineRunID: baseManifest.RunID, CandidateRunID: candidateManifest.RunID,
		Comparable: len(reasons) == 0, Forced: *forceCompare && len(reasons) > 0, Status: "pass", Reasons: reasons, Metrics: map[string]comparisonMetric{},
	}
	if !result.Comparable && !result.Forced {
		result.Status = "non_comparable"
	}
	for name, baseMetric := range baseSummary.Metrics {
		candidateMetric, ok := candidateSummary.Metrics[name]
		if !ok || candidateMetric.Unit != baseMetric.Unit {
			continue
		}
		changeKind := "percent"
		var changePercent *float64
		regression := false
		comparisonEnabled := result.Comparable || result.Forced
		if baseMetric.Value == 0 {
			changeKind = "unchanged_zero"
			if candidateMetric.Value != 0 {
				changeKind = "new_nonzero"
				regression = comparisonEnabled && ((baseMetric.LowerIsBetter && candidateMetric.Value > 0) || (!baseMetric.LowerIsBetter && candidateMetric.Value < 0))
			}
		} else {
			change := ((candidateMetric.Value - baseMetric.Value) / baseMetric.Value) * 100
			changePercent = &change
			regression = comparisonEnabled && ((baseMetric.LowerIsBetter && change > *threshold) || (!baseMetric.LowerIsBetter && change < -*threshold))
		}
		result.Metrics[name] = comparisonMetric{BaselineValue: baseMetric.Value, CandidateValue: candidateMetric.Value, Unit: baseMetric.Unit, ChangeKind: changeKind, ChangePercent: changePercent, Regression: regression}
		if regression {
			result.Status = "regression"
		}
	}
	return writeJSON(filepath.Join(out, "comparison.json"), result)
}

func readRun(path string) (manifest, summary, error) {
	var m manifest
	var s summary
	if err := readJSON(filepath.Join(path, "manifest.json"), &m); err != nil {
		return m, s, err
	}
	if err := readJSON(filepath.Join(path, "summary.json"), &s); err != nil {
		return m, s, err
	}
	if !s.Correct || s.Status == "fail" {
		return m, s, errors.New("run failed its correctness gate")
	}
	return m, s, nil
}

func comparabilityReasons(a, b manifest) []string {
	var reasons []string
	checks := []struct{ name, a, b string }{
		{"schema_version", a.SchemaVersion, b.SchemaVersion}, {"suite", a.Suite, b.Suite}, {"fixture", a.Fixture, b.Fixture},
		{"fixture_digest", a.FixtureDigest, b.FixtureDigest}, {"os", a.Environment.OS, b.Environment.OS},
		{"architecture", a.Environment.Architecture, b.Environment.Architecture}, {"storage_profile", a.Environment.StorageProfile, b.Environment.StorageProfile},
	}
	for _, check := range checks {
		if check.a != check.b {
			reasons = append(reasons, fmt.Sprintf("%s differs (%q vs %q)", check.name, check.a, check.b))
		}
	}
	return reasons
}

func writeFixtureRecords(path, name string, seed int64, size fixtureSize, maxBytes int64, deadline time.Time) (string, int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	w := bufio.NewWriterSize(f, 64*1024)
	logicalPerFile := size.Logical / int64(size.Files)
	var written int64
	for i := 1; i <= size.Files; i++ {
		if i%1000 == 0 && time.Now().After(deadline) {
			return "", written, errors.New("fixture generation exceeded the approved wall-time ceiling")
		}
		record := deterministicRecord(name, seed, i, size.Objects, logicalPerFile)
		line, err := json.Marshal(record)
		if err != nil {
			return "", written, err
		}
		line = append(line, '\n')
		if written+int64(len(line)) > maxBytes {
			return "", written, errors.New("fixture records exceeded the configured physical-byte ceiling")
		}
		if _, err := w.Write(line); err != nil {
			return "", written, err
		}
		if _, err := h.Write(line); err != nil {
			return "", written, err
		}
		written += int64(len(line))
	}
	if err := w.Flush(); err != nil {
		return "", written, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), written, nil
}

func deterministicRecord(name string, seed int64, index, objects int, logicalBytes int64) fixtureRecord {
	kinds := [...]string{"video", "image", "gallery", "caption", "sidecar"}
	segments := []string{name, fmt.Sprintf("seed-%d", seed), fmt.Sprintf("batch-%05d", index/1000)}
	if index%97 == 0 {
		segments = append(segments, "deep", "nested", "path", "測試")
	}
	segments = append(segments, fmt.Sprintf("synthetic-%06d.dat", index))
	record := fixtureRecord{
		ID: index, ObjectID: ((index-1)*7919)%objects + 1, Path: filepath.ToSlash(filepath.Join(segments...)),
		LogicalBytes: logicalBytes + int64((index+int(seed))%4096), Kind: kinds[(index+int(seed))%len(kinds)], Missing: index%101 == 0,
	}
	if index > 10 && index%53 == 0 {
		record.DuplicateOf = index - 10
	}
	return record
}

func createRunDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if filepath.Clean(abs) == filepath.Clean(filepath.Dir(abs)) {
		return "", errors.New("output must name a run directory, not a filesystem root")
	}
	if _, err := os.Stat(abs); err == nil {
		return "", fmt.Errorf("output already exists: %s", abs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(abs, ".stash-benchmark-run"), []byte(schemaVersion+"\n"), 0o600); err != nil {
		return "", err
	}
	return abs, nil
}

func fixtureDigest(name string, seed int64, size fixtureSize) string {
	canonical := strings.Join([]string{name, strconv.FormatInt(seed, 10), strconv.Itoa(size.Objects), strconv.Itoa(size.Files), strconv.FormatInt(size.Logical, 10)}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func redactMountOptions(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	kept := parts[:0]
	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(strings.SplitN(part, "=", 2)[0]))
		switch key {
		case "password", "passwd", "pass", "username", "user", "credential", "credentials", "secret", "token":
			continue
		default:
			kept = append(kept, strings.TrimSpace(part))
		}
	}
	return strings.Join(kept, ",")
}

func writeJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func readJSON(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	return dec.Decode(value)
}
