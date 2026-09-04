package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/corona10/goimagehash"
)

type pHashSample struct {
	Algorithm string  `json:"algorithm"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Seconds   float64 `json:"seconds"`
	Hash      string  `json:"hash"`
}

func runPHashSuite(imageSize, repetitions int, seed int64, deadline time.Time) ([]pHashSample, map[string]metric, error) {
	if imageSize < 32 || imageSize > 8192 {
		return nil, nil, errors.New("image size must be between 32 and 8192 pixels")
	}
	synthetic := syntheticImage(imageSize, seed)
	var samples []pHashSample
	var expected uint64
	durations := make([]float64, 0, repetitions)
	for repetition := -1; repetition < repetitions; repetition++ {
		if time.Now().After(deadline) {
			return nil, nil, errors.New("pHash suite exceeded the configured wall-time ceiling")
		}
		started := time.Now()
		hashValue, err := goimagehash.PerceptionHash(synthetic)
		seconds := time.Since(started).Seconds()
		if err != nil {
			return nil, nil, err
		}
		value := hashValue.GetHash()
		if repetition == -1 {
			expected = value
		} else {
			if value != expected {
				return nil, nil, errors.New("pHash correctness gate: deterministic image hash changed")
			}
			samples = append(samples, pHashSample{Algorithm: "image_phash", Width: imageSize, Height: imageSize, Seconds: seconds, Hash: fmt.Sprintf("%016x", value)})
			durations = append(durations, seconds)
		}
	}
	stats := calculateStats(durations)
	metrics := map[string]metric{
		"error_count":             {Value: 0, Unit: "count", LowerIsBetter: true},
		"phash_wall_min":          {Value: stats.Min, Unit: "seconds", LowerIsBetter: true},
		"phash_wall_max":          {Value: stats.Max, Unit: "seconds", LowerIsBetter: true},
		"phash_wall_median":       {Value: stats.Median, Unit: "seconds", LowerIsBetter: true},
		"phash_wall_p95":          {Value: stats.P95, Unit: "seconds", LowerIsBetter: true},
		"phash_wall_cv":           {Value: stats.CV, Unit: "ratio", LowerIsBetter: true},
		"phash_pixels_per_second": {Value: float64(imageSize*imageSize) / stats.Median, Unit: "pixels/s", LowerIsBetter: false},
	}
	return samples, metrics, nil
}

func syntheticImage(size int, seed int64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	seedByte := uint8(seed)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x*255)/size) ^ seedByte,
				G: uint8((y*255)/size) ^ uint8(seed>>8),
				B: uint8((x + y + int(seedByte)) % 256),
				A: 255,
			})
		}
	}
	return img
}
