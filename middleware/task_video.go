package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const taskVideoBodyLimit = 1 << 20
const taskVideoUserID = "task_video_session_user_id"
const taskVideoTokenID = "task_video_selected_token_id"

// TaskVideoSessionAuth deliberately excludes UserAuth's access-token fallback.
func TaskVideoSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id, ok := session.Get("id").(int)
		username, _ := session.Get("username").(string)
		if !ok || id <= 0 || username == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Login required"})
			return
		}
		if status, ok := session.Get("status").(int); !ok || status != common.UserStatusEnabled {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Set(taskVideoUserID, id)
		UserAuth()(c)
	}
}

// TaskVideoToken prepares a normal video request using only the session owner's
// selected token. The client never supplies the bearer key used by TokenAuth.
func TaskVideoToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt(taskVideoUserID) <= 0 || c.GetInt("id") != c.GetInt(taskVideoUserID) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if c.ContentType() != gin.MIMEJSON {
			c.AbortWithStatus(http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, taskVideoBodyLimit+1))
		if err != nil || len(body) > taskVideoBodyLimit {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		var fields map[string]json.RawMessage
		if common.Unmarshal(body, &fields) != nil || len(fields) == 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		for key := range fields {
			switch key {
			case "token_id", "model", "prompt", "duration", "size", "metadata":
			default:
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
		}
		var request struct {
			TokenID  int                        `json:"token_id"`
			Model    string                     `json:"model"`
			Prompt   string                     `json:"prompt"`
			Duration *int                       `json:"duration"`
			Size     *string                    `json:"size"`
			Metadata map[string]json.RawMessage `json:"metadata"`
		}
		if common.Unmarshal(body, &request) != nil || request.TokenID <= 0 || strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Prompt) == "" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		for key := range request.Metadata {
			switch key {
			case "content", "ratio", "first_frame_image", "last_frame_image", "reference_video", "reference_audio", "callback_url", "aigc_watermark":
			default:
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
		}
		token, err := model.GetTokenByIds(request.TokenID, c.GetInt(taskVideoUserID))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, gorm.ErrRecordNotFound) {
				status = http.StatusForbidden
			}
			c.AbortWithStatus(status)
			return
		}
		videoModels, err := service.GetTaskVideoModelsForToken(c.GetInt(taskVideoUserID), token)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, service.ErrTaskVideoTokenUnavailable) {
				status = http.StatusUnauthorized
			} else if errors.Is(err, service.ErrTaskVideoTokenForbidden) {
				status = http.StatusForbidden
			}
			c.AbortWithStatus(status)
			return
		}
		if !service.TaskVideoModelAllowed(videoModels, request.Model) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		// TokenAuth interprets hyphens as an admin channel selector. Reject such
		// stored keys here instead of allowing dashboard channel pinning.
		if token.UserId != c.GetInt(taskVideoUserID) || token.Key == "" || strings.Contains(token.Key, "-") {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		delete(fields, "token_id")
		body, err = common.Marshal(fields)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		common.CleanupBodyStorage(c)
		storage, err := common.CreateBodyStorage(body)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Set(common.KeyBodyStorage, storage)
		defer common.CleanupBodyStorage(c)
		original := c.Request
		c.Request = original.Clone(original.Context())
		defer func() { c.Request = original }()
		c.Request.Body = io.NopCloser(storage)
		c.Request.ContentLength = int64(len(body))
		c.Request.URL.Path = "/v1/videos"
		c.Request.URL.RawPath = ""
		c.Request.URL.RawQuery = ""
		c.Request.RequestURI = "/v1/videos"
		for _, header := range []string{"Authorization", "Sec-WebSocket-Protocol", "X-Api-Key", "X-Goog-Api-Key", "Mj-Api-Secret", "Content-Length"} {
			c.Request.Header.Del(header)
		}
		c.Request.Header.Set("Authorization", "Bearer "+token.Key)
		c.Set(taskVideoTokenID, token.Id)
		c.Next()
	}
}

// TaskVideoIdentity runs after TokenAuth and before channel selection, including
// when a stale token cache resolves to a different identity than the DB row.
func TaskVideoIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt("id") != c.GetInt(taskVideoUserID) || c.GetInt("token_id") != c.GetInt(taskVideoTokenID) || c.GetInt(taskVideoTokenID) <= 0 {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if _, exists := c.Get("specific_channel_id"); exists {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
