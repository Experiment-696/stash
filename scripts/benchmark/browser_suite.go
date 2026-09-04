package main

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type browserSample struct {
	URL              string  `json:"url"`
	Seconds          float64 `json:"seconds"`
	DOMNodes         int64   `json:"dom_nodes"`
	ResourceRequests int64   `json:"resource_requests"`
	JSHeapUsedBytes  int64   `json:"js_heap_used_bytes"`
}

func validateLocalBrowserURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("browser URL must use http or https")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("browser benchmark targets are restricted to localhost/loopback")
	}
	return nil
}

func runBrowserSuite(baseURL, chromePath string, repetitions int, deadline time.Time) ([]browserSample, map[string]metric, error) {
	if err := validateLocalBrowserURL(baseURL); err != nil {
		return nil, nil, err
	}
	var samples []browserSample
	durations := make([]float64, 0, repetitions)
	for repetition := -1; repetition < repetitions; repetition++ {
		if time.Now().After(deadline) {
			return nil, nil, errors.New("browser suite exceeded the configured wall-time ceiling")
		}
		profile, err := os.MkdirTemp("", "stash-benchmark-browser-")
		if err != nil {
			return nil, nil, err
		}
		sample, runErr := measureBrowserPage(baseURL, chromePath, profile, deadline)
		removeErr := os.RemoveAll(profile)
		if runErr != nil {
			return nil, nil, runErr
		}
		if removeErr != nil {
			return nil, nil, removeErr
		}
		if repetition >= 0 {
			samples = append(samples, sample)
			durations = append(durations, sample.Seconds)
		}
	}
	stats := calculateStats(durations)
	metrics := map[string]metric{
		"error_count":         {Value: 0, Unit: "count", LowerIsBetter: true},
		"browser_wall_min":    {Value: stats.Min, Unit: "seconds", LowerIsBetter: true},
		"browser_wall_max":    {Value: stats.Max, Unit: "seconds", LowerIsBetter: true},
		"browser_wall_median": {Value: stats.Median, Unit: "seconds", LowerIsBetter: true},
		"browser_wall_p95":    {Value: stats.P95, Unit: "seconds", LowerIsBetter: true},
		"browser_wall_cv":     {Value: stats.CV, Unit: "ratio", LowerIsBetter: true},
	}
	metrics["browser_dom_nodes_median"] = metric{Value: medianInt64(samples, func(sample browserSample) int64 { return sample.DOMNodes }), Unit: "count", LowerIsBetter: true}
	metrics["browser_requests_median"] = metric{Value: medianInt64(samples, func(sample browserSample) int64 { return sample.ResourceRequests }), Unit: "count", LowerIsBetter: true}
	metrics["browser_js_heap_used_median"] = metric{Value: medianInt64(samples, func(sample browserSample) int64 { return sample.JSHeapUsedBytes }), Unit: "bytes", LowerIsBetter: true}
	return samples, metrics, nil
}

func measureBrowserPage(baseURL, chromePath, profile string, deadline time.Time) (browserSample, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.UserDataDir(profile), chromedp.Headless)
	if strings.TrimSpace(chromePath) != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAllocator()
	ctx, cancelContext := chromedp.NewContext(allocator)
	defer cancelContext()
	ctx, cancelDeadline := context.WithDeadline(ctx, deadline)
	defer cancelDeadline()
	var domNodes, requests, heap float64
	started := time.Now()
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(`document.getElementsByTagName('*').length`, &domNodes),
		chromedp.Evaluate(`performance.getEntriesByType('resource').length + 1`, &requests),
		chromedp.Evaluate(`performance.memory ? performance.memory.usedJSHeapSize : 0`, &heap),
	)
	if err != nil {
		return browserSample{}, err
	}
	if domNodes < 1 {
		return browserSample{}, errors.New("browser correctness gate: page has no DOM nodes")
	}
	return browserSample{URL: baseURL, Seconds: time.Since(started).Seconds(), DOMNodes: int64(domNodes), ResourceRequests: int64(requests), JSHeapUsedBytes: int64(heap)}, nil
}

func medianInt64(samples []browserSample, selectValue func(browserSample) int64) float64 {
	values := make([]float64, len(samples))
	for i, sample := range samples {
		values[i] = float64(selectValue(sample))
	}
	return median(values)
}
