package opsmetrics

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPercentileFromHistogram(t *testing.T) {
	value := percentileFromHistogram([]HistogramBucket{{UpperBoundMs: 100, Count: 3}, {UpperBoundMs: 500, Count: 7}}, 0.9)
	assert.NotNil(t, value)
	assert.EqualValues(t, 500, *value)
	assert.Nil(t, percentileFromHistogram(nil, 0.95))
}
