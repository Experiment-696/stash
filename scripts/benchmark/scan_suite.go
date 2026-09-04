package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type scanSample struct {
	Operation string  `json:"operation"`
	Files     int     `json:"files"`
	Bytes     int64   `json:"bytes"`
	Seconds   float64 `json:"seconds"`
	Digest    string  `json:"digest"`
}

func runScanSuite(root string, records, repetitions int, seed int64, maxBytes int64, deadline time.Time) ([]scanSample, map[string]metric, error) {
	if records <= 0 {
		return nil, nil, errors.New("scan record count must be positive")
	}
	physicalBytes, expectedFiles, err := createScanFixture(root, records, seed, maxBytes, deadline)
	if err != nil {
		return nil, nil, err
	}
	var samples []scanSample
	var expectedDigest string
	durations := make([]float64, 0, repetitions)
	for repetition := 0; repetition < repetitions; repetition++ {
		if time.Now().After(deadline) {
			return nil, nil, errors.New("scan suite exceeded the configured wall-time ceiling")
		}
		started := time.Now()
		files, bytes, digest, err := scanFixture(root)
		seconds := time.Since(started).Seconds()
		if err != nil {
			return nil, nil, err
		}
		if files != expectedFiles || bytes != physicalBytes {
			return nil, nil, fmt.Errorf("scan correctness gate: files=%d/%d bytes=%d/%d", files, expectedFiles, bytes, physicalBytes)
		}
		if expectedDigest == "" {
			expectedDigest = digest
		} else if digest != expectedDigest {
			return nil, nil, errors.New("scan correctness gate: path/size digest changed during unchanged rescan")
		}
		operation := "unchanged_rescan"
		if repetition == 0 {
			operation = "initial_scan"
		}
		samples = append(samples, scanSample{Operation: operation, Files: files, Bytes: bytes, Seconds: seconds, Digest: digest})
		durations = append(durations, seconds)
	}
	stats := calculateStats(durations)
	metrics := map[string]metric{
		"error_count":           {Value: 0, Unit: "count", LowerIsBetter: true},
		"scan_files":            {Value: float64(expectedFiles), Unit: "count", LowerIsBetter: false},
		"scan_physical_bytes":   {Value: float64(physicalBytes), Unit: "bytes", LowerIsBetter: true},
		"scan_wall_min":         {Value: stats.Min, Unit: "seconds", LowerIsBetter: true},
		"scan_wall_max":         {Value: stats.Max, Unit: "seconds", LowerIsBetter: true},
		"scan_wall_median":      {Value: stats.Median, Unit: "seconds", LowerIsBetter: true},
		"scan_wall_p95":         {Value: stats.P95, Unit: "seconds", LowerIsBetter: true},
		"scan_wall_cv":          {Value: stats.CV, Unit: "ratio", LowerIsBetter: true},
		"scan_files_per_second": {Value: float64(expectedFiles) / stats.Median, Unit: "files/s", LowerIsBetter: false},
	}
	return samples, metrics, nil
}

func createScanFixture(root string, records int, seed int64, maxBytes int64, deadline time.Time) (int64, int, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return 0, 0, err
	}
	payload := make([]byte, 1024)
	if _, err := newDeterministicReader(seed).Read(payload); err != nil {
		return 0, 0, err
	}
	var physicalBytes int64
	created := 0
	for i := 1; i <= records; i++ {
		if time.Now().After(deadline) {
			return physicalBytes, created, errors.New("scan fixture generation exceeded the configured wall-time ceiling")
		}
		if i%101 == 0 {
			continue
		}
		dir := filepath.Join(root, fmt.Sprintf("batch-%05d", i/1000))
		if i%97 == 0 {
			dir = filepath.Join(dir, "deep", "nested", "測試")
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return physicalBytes, created, err
		}
		if physicalBytes+int64(len(payload)) > maxBytes {
			return physicalBytes, created, errors.New("scan fixture exceeded the configured physical-byte ceiling")
		}
		path := filepath.Join(dir, fmt.Sprintf("synthetic-%06d.dat", i))
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return physicalBytes, created, err
		}
		physicalBytes += int64(len(payload))
		created++
	}
	return physicalBytes, created, nil
}

func scanFixture(root string) (int, int64, string, error) {
	h := sha256.New()
	files := 0
	var bytes int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\n%d\n", filepath.ToSlash(relative), info.Size())
		files++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, "", err
	}
	return files, bytes, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
