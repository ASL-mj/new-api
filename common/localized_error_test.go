package common

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalizedErrorCarriesTranslationMetadata(t *testing.T) {
	err := NewLocalizedError("channel_usage.max_batch", map[string]any{"Max": 200})

	var localized *LocalizedError
	require.ErrorAs(t, err, &localized)
	assert.Equal(t, "channel_usage.max_batch", localized.Key)
	assert.EqualValues(t, 200, localized.Args["Max"])
	assert.Equal(t, "channel_usage.max_batch", err.Error())
}

func TestWrappedLocalizedErrorPreservesCause(t *testing.T) {
	cause := errors.New("database unavailable")
	err := WrapLocalizedError(cause, "common.database_error")

	var localized *LocalizedError
	require.ErrorAs(t, err, &localized)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, cause.Error(), err.Error())
}

func TestApiErrorTranslatesLocalizedError(t *testing.T) {
	previousTranslate := TranslateMessage
	TranslateMessage = func(_ *gin.Context, key string, args ...map[string]any) string {
		return key + ":" + args[0]["Field"].(string)
	}
	t.Cleanup(func() { TranslateMessage = previousTranslate })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	ApiError(context, NewLocalizedError("common.invalid_params", map[string]any{"Field": "quota"}))

	response := map[string]any{}
	require.NoError(t, Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response["success"].(bool))
	assert.Equal(t, "common.invalid_params:quota", response["message"])
}
