package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestRedactSystemEventRemovesSecretsAndBodies(t *testing.T) {
	redacted := redactSystemEvent(model.SystemEventLog{
		Message: "request failed with sk-secret",
		Extra:   `{"authorization":"Bearer secret","request_body":{"prompt":"private"},"status":500}`,
	})
	assert.NotContains(t, redacted.Message, "sk-secret")
	assert.Contains(t, redacted.Extra, "REDACTED")
	assert.NotContains(t, redacted.Extra, "private")
	assert.Contains(t, redacted.Extra, "500")
}
