package opsmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		sample Sample
		want   ErrorClass
	}{
		{"success", Sample{Success: true, StatusCode: 200}, ErrorClassNone},
		{"quota 403", Sample{StatusCode: 403, ErrorCode: "insufficient_user_quota"}, ErrorClassBusinessLimited},
		{"auth 401", Sample{StatusCode: 401, ErrorCode: "access_denied"}, ErrorClassBusinessLimited},
		{"upstream 429", Sample{StatusCode: 429}, ErrorClassUpstream},
		{"upstream 502", Sample{StatusCode: 502}, ErrorClassUpstream},
		{"local 500", Sample{StatusCode: 500, LocalError: true}, ErrorClassSystem},
		{"timeout", Sample{StatusCode: 504, ErrorCode: "timeout"}, ErrorClassUpstream},
		{"upstream 529", Sample{StatusCode: 529}, ErrorClassUpstream},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ClassifyError(test.sample))
		})
	}
}
