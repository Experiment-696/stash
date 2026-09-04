package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

type generatedStorageSample struct {
	Policy         string  `json:"policy"`
	Artifacts      int     `json:"artifacts"`
	Bytes          int64   `json:"bytes"`
	UniqueDigests  int     `json:"unique_digests"`
	DuplicateBytes int64   `json:"duplicate_bytes"`
	Seconds        float64 `json:"seconds"`
}

func runGeneratedStorageSuite(root string, records, repetitions int, seed int64, maxBytes int64, deadline time.Time) ([]generatedStorageSample, map[string]metric, error) {
	if records <= 0 {
		return nil, nil, errors.New("generated-storage record count must be positive")
	}
	policies := []struct {
		name  string
		sizes []int
	}{{name: "eager", sizes: []int{64, 128, 256}}, {name: "lazy", sizes: []int{64}}}
	var samples []generatedStorageSample
	durations := map[string][]float64{"eager": {}, "lazy": {}}
	var finalByPolicy = map[string]generatedStorageSample{}
	for repetition := -1; repetition < repetitions; repetition++ {
		for _, policy := range policies {
			if time.Now().After(deadline) {
				return nil, nil, errors.New("generated-storage suite exceeded the configured wall-time ceiling")
			}
			policyRoot := filepath.Join(root, policy.name)
			if err := os.RemoveAll(policyRoot); err != nil {
				return nil, nil, err
			}
			started := time.Now()
			artifacts, bytes, unique, duplicateBytes, err := generateArtifacts(policyRoot, records, policy.sizes, seed, maxBytes)
			seconds := time.Since(started).Seconds()
			if err != nil {
				return nil, nil, err
			}
			if artifacts != records*len(policy.sizes) {
				return nil, nil, errors.New("generated-storage correctness gate: artifact count mismatch")
			}
			if err := validateGeneratedPNGs(policyRoot, artifacts); err != nil {
				return nil, nil, err
			}
			sample := generatedStorageSample{Policy: policy.name, Artifacts: artifacts, Bytes: bytes, UniqueDigests: unique, DuplicateBytes: duplicateBytes, Seconds: seconds}
			finalByPolicy[policy.name] = sample
			if repetition >= 0 {
				samples = append(samples, sample)
				durations[policy.name] = append(durations[policy.name], seconds)
			}
		}
	}
	metrics := map[string]metric{"error_count": {Value: 0, Unit: "count", LowerIsBetter: true}}
	for _, policy := range policies {
		stats := calculateStats(durations[policy.name])
		final := finalByPolicy[policy.name]
		prefix := "generated_" + policy.name + "_"
		metrics[prefix+"artifacts"] = metric{Value: float64(final.Artifacts), Unit: "count", LowerIsBetter: true}
		metrics[prefix+"bytes"] = metric{Value: float64(final.Bytes), Unit: "bytes", LowerIsBetter: true}
		metrics[prefix+"unique_digests"] = metric{Value: float64(final.UniqueDigests), Unit: "count", LowerIsBetter: true}
		metrics[prefix+"wall_median"] = metric{Value: stats.Median, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"wall_p95"] = metric{Value: stats.P95, Unit: "seconds", LowerIsBetter: true}
		metrics[prefix+"wall_cv"] = metric{Value: stats.CV, Unit: "ratio", LowerIsBetter: true}
	}
	metrics["generated_eager_dedup_potential_bytes"] = metric{Value: float64(finalByPolicy["eager"].DuplicateBytes), Unit: "bytes", LowerIsBetter: false}
	return samples, metrics, nil
}

func generateArtifacts(root string, records int, sizes []int, seed int64, maxBytes int64) (int, int64, int, int64, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return 0, 0, 0, 0, err
	}
	unique := map[string]int64{}
	artifacts := 0
	var totalBytes int64
	for record := 1; record <= records; record++ {
		for _, size := range sizes {
			path := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(filepath.Join(formatBatch(record), formatArtifact(record, size)))))
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return artifacts, totalBytes, len(unique), 0, err
			}
			img := syntheticImage(size, seed+int64(record%7))
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, img); err != nil {
				return artifacts, totalBytes, len(unique), 0, err
			}
			data := encoded.Bytes()
			if totalBytes+int64(len(data)) > maxBytes {
				return artifacts, totalBytes, len(unique), 0, errors.New("generated artifacts exceeded the configured physical-byte ceiling")
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return artifacts, totalBytes, len(unique), 0, err
			}
			if _, err := file.Write(data); err != nil {
				file.Close()
				return artifacts, totalBytes, len(unique), 0, err
			}
			if err := file.Close(); err != nil {
				return artifacts, totalBytes, len(unique), 0, err
			}
			sum := sha256.Sum256(data)
			unique[hex.EncodeToString(sum[:])] = int64(len(data))
			totalBytes += int64(len(data))
			artifacts++
		}
	}
	var uniqueBytes int64
	for _, size := range unique {
		uniqueBytes += size
	}
	return artifacts, totalBytes, len(unique), totalBytes - uniqueBytes, nil
}

func validateGeneratedPNGs(root string, expected int) error {
	validated := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, format, err := image.DecodeConfig(file)
		file.Close()
		if err != nil {
			return err
		}
		if format != "png" {
			return errors.New("generated-storage correctness gate: artifact is not PNG")
		}
		validated++
		return nil
	})
	if err != nil {
		return err
	}
	if validated != expected {
		return errors.New("generated-storage correctness gate: decodable artifact count mismatch")
	}
	return nil
}

func formatBatch(record int) string { return fmt.Sprintf("batch-%05d", record/1000) }
func formatArtifact(record, size int) string {
	return fmt.Sprintf("artifact-%06d-%d.png", record, size)
}
