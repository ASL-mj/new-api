package controller

import (
	"bytes"
	"net/http"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const taskVideoResponseLimit = 64 << 10

var publicVideoTaskID = regexp.MustCompile(`^task_[A-Za-z0-9_-]{1,128}$`)

type taskVideoWriter struct {
	gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	written  bool
	overflow bool
}

func (w *taskVideoWriter) Header() http.Header { return w.header }
func (w *taskVideoWriter) WriteHeader(status int) {
	if !w.written && status > 0 {
		w.status = status
	}
}
func (w *taskVideoWriter) WriteHeaderNow() { w.written = true }
func (w *taskVideoWriter) Status() int     { return w.status }
func (w *taskVideoWriter) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *taskVideoWriter) Written() bool { return w.written }
func (w *taskVideoWriter) Flush()        { w.WriteHeaderNow() }
func (w *taskVideoWriter) Write(p []byte) (int, error) {
	w.WriteHeaderNow()
	if w.overflow || len(p) > taskVideoResponseLimit-w.body.Len() {
		w.overflow = true
		return len(p), nil
	}
	return w.body.Write(p)
}
func (w *taskVideoWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TaskVideoResponse keeps upstream bodies/headers private and emits only the
// public ID. Buffer overflow must not interrupt billing after upstream success.
func TaskVideoResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		original := c.Writer
		buffer := &taskVideoWriter{ResponseWriter: original, header: original.Header().Clone(), status: http.StatusOK}
		c.Writer = buffer
		defer func() { c.Writer = original }()
		c.Next()
		c.Writer = original
		var video struct {
			ID string `json:"id"`
		}
		if buffer.status >= 200 && buffer.status < 300 && !buffer.overflow && common.Unmarshal(buffer.body.Bytes(), &video) == nil && publicVideoTaskID.MatchString(video.ID) {
			common.ApiSuccess(c, gin.H{"task_id": video.ID})
			return
		}
		status := buffer.status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		message := "Video submission failed; check the task list before submitting again"
		switch status {
		case http.StatusBadRequest:
			message = "Invalid video request; check the model, prompt and media parameters"
		case http.StatusUnauthorized:
			message = "Login and an active token are required"
		case http.StatusForbidden:
			message = "Selected token or video model is not available for this account, IP or group"
		case http.StatusRequestEntityTooLarge:
			message = "Video request is too large"
		case http.StatusUnsupportedMediaType:
			message = "Video request must use application/json"
		case http.StatusTooManyRequests:
			message = "Video submission is rate limited; try later"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
	}
}
