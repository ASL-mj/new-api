package service

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var systemEventSecretKeys = map[string]struct{}{
	"authorization": {}, "api_key": {}, "apikey": {}, "key": {}, "token": {}, "cookie": {}, "password": {}, "secret": {},
	"request_body": {}, "response_body": {},
}

var systemEventSecretText = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]+|bearer\s+[a-z0-9._~+/=-]+)`)

func redactSystemEvent(event model.SystemEventLog) model.SystemEventLog {
	event.Message = systemEventSecretText.ReplaceAllString(common.MaskSensitiveInfo(strings.TrimSpace(event.Message)), "[REDACTED]")
	if len(event.Extra) > 0 {
		var value any
		if err := common.Unmarshal([]byte(event.Extra), &value); err == nil {
			redacted := redactSystemValue(value)
			if encoded, err := common.Marshal(redacted); err == nil {
				event.Extra = string(encoded)
			}
		} else {
			event.Extra = "{}"
		}
	}
	return event
}

func redactSystemValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if _, secret := systemEventSecretKeys[strings.ToLower(key)]; secret {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactSystemValue(nested)
		}
	case []any:
		for index := range typed {
			typed[index] = redactSystemValue(typed[index])
		}
	}
	return value
}
