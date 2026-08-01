package opsmetrics

import "strings"

func ClassifyError(sample Sample) ErrorClass {
	if sample.Success {
		return ErrorClassNone
	}
	code := strings.ToLower(strings.TrimSpace(sample.ErrorCode))
	if isBusinessLimitedCode(code) || sample.StatusCode == 401 || sample.StatusCode == 403 {
		return ErrorClassBusinessLimited
	}
	if sample.LocalError {
		return ErrorClassSystem
	}
	if sample.StatusCode == 429 || sample.StatusCode == 529 || sample.StatusCode >= 500 || isUpstreamCode(code) {
		return ErrorClassUpstream
	}
	return ErrorClassBusinessLimited
}

func isBusinessLimitedCode(code string) bool {
	for _, marker := range []string{
		"insufficient", "quota", "access_denied", "permission", "model_not_found",
		"invalid_api_key", "invalid_key", "pre_consume", "unauthorized",
	} {
		if strings.Contains(code, marker) {
			return true
		}
	}
	return false
}

func isUpstreamCode(code string) bool {
	for _, marker := range []string{"upstream", "relay", "do_request", "bad_response", "timeout"} {
		if strings.Contains(code, marker) {
			return true
		}
	}
	return false
}
