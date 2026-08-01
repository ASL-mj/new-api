package opsmetrics

import "math"

const histogramInfiniteBound int64 = math.MaxInt64

var histogramBounds = []int64{100, 250, 500, 1000, 2000, 3000, 5000, 10000, 20000, 30000, 60000, histogramInfiniteBound}

type HistogramBucket struct {
	UpperBoundMs int64
	Count        int64
}

func histogramUpperBound(value int64) int64 {
	for _, bound := range histogramBounds {
		if value <= bound {
			return bound
		}
	}
	return histogramInfiniteBound
}

func percentileFromHistogram(buckets []HistogramBucket, percentile float64) *int64 {
	if percentile <= 0 || percentile > 1 {
		return nil
	}
	var total int64
	for _, bucket := range buckets {
		total += bucket.Count
	}
	if total == 0 {
		return nil
	}
	target := int64(math.Ceil(float64(total) * percentile))
	var cumulative int64
	for _, bucket := range buckets {
		cumulative += bucket.Count
		if cumulative >= target {
			value := bucket.UpperBoundMs
			return &value
		}
	}
	return nil
}

func PercentileFromHistogram(buckets []HistogramBucket, percentile float64) *int64 {
	return percentileFromHistogram(buckets, percentile)
}

func cloneHistogram(source map[int64]int64) map[int64]int64 {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[int64]int64, len(source))
	for upperBound, count := range source {
		cloned[upperBound] = count
	}
	return cloned
}

func mergeHistogram(destination, source map[int64]int64) map[int64]int64 {
	if len(source) == 0 {
		return destination
	}
	if destination == nil {
		destination = make(map[int64]int64, len(source))
	}
	for upperBound, count := range source {
		destination[upperBound] += count
	}
	return destination
}
