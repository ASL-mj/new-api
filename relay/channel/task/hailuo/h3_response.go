package hailuo

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type h3Error struct {
	Message  string          `json:"message"`
	HTTPCode json.RawMessage `json:"http_code"`
	Code     json.RawMessage `json:"code"`
}
type h3CreateResponse struct {
	TaskID string   `json:"task_id"`
	Error  *h3Error `json:"error"`
}
type h3QueryResponse struct {
	Task  *h3Task  `json:"task"`
	Error *h3Error `json:"error"`
}
type h3Task struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	Status     string          `json:"status"`
	Resolution json.RawMessage `json:"resolution"`
	Content    struct {
		URL string `json:"url"`
	} `json:"content"`
	Error *h3Error        `json:"error"`
	Usage json.RawMessage `json:"usage"`
}

func h3Code(raw json.RawMessage) int {
	var n int
	if common.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if common.Unmarshal(raw, &s) == nil {
		n, _ = strconv.Atoi(s)
	}
	return n
}

func (e *h3Error) statusCode() int {
	if e == nil {
		return 0
	}
	for _, raw := range []json.RawMessage{e.HTTPCode, e.Code} {
		if n := h3Code(raw); n >= 400 && n <= 599 {
			return n
		}
	}
	return 0
}

// Upstream messages may contain credentials, signed URLs, or echoed prompts.
// Retain them only in controlled raw task data; return fixed diagnostics here.
func h3ErrorMessage(code int) string {
	switch {
	case code == 408:
		return "H3 upstream request timed out"
	case code == 429:
		return "H3 upstream rate limit reached"
	case code >= 500:
		return "H3 upstream temporarily unavailable"
	case code == 401 || code == 403:
		return "H3 upstream authentication failed"
	case code == 402:
		return "H3 upstream balance insufficient"
	default:
		return "H3 upstream rejected the task"
	}
}

func parseH3Task(raw []byte, expectedID string) (*relaycommon.TaskInfo, error) {
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("H3 response exceeds size limit")
	}
	var response h3QueryResponse
	if err := common.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("invalid H3 response JSON")
	}
	if response.Error != nil {
		code := response.Error.statusCode()
		if code == 0 {
			return nil, fmt.Errorf("unclassified H3 error response")
		}
		return &relaycommon.TaskInfo{Code: code, Reason: h3ErrorMessage(code)}, nil
	}
	task := response.Task
	if task == nil {
		return nil, fmt.Errorf("H3 task wrapper is missing")
	}
	if expectedID != "" && task.ID != "" && expectedID != task.ID {
		return nil, fmt.Errorf("H3 response task ID mismatch")
	}
	if task.Model != "" && task.Model != H3Model {
		return nil, fmt.Errorf("H3 response model mismatch")
	}
	result := &relaycommon.TaskInfo{TaskID: task.ID}
	switch task.Status {
	case "queued":
		result.Status, result.Progress = model.TaskStatusQueued, "30%"
	case "running":
		result.Status, result.Progress = model.TaskStatusInProgress, "50%"
	case "succeeded":
		if task.Error != nil || !h3ResultURL(task.Content.URL) {
			return nil, fmt.Errorf("H3 success has invalid content or error")
		}
		result.Status, result.Progress, result.Url = model.TaskStatusSuccess, "100%", task.Content.URL
		usage := parseH3Usage(task)
		result.TaskUsage = &usage
	case "failed", "cancelled":
		result.Status, result.Progress = model.TaskStatusFailure, "100%"
		result.Reason = "H3 task " + task.Status
	default:
		return nil, fmt.Errorf("unknown H3 task status")
	}
	// Explicit terminal task status takes precedence over any task error code.
	if result.Status != model.TaskStatusFailure && task.Error != nil {
		code := task.Error.statusCode()
		if code == 0 {
			return nil, fmt.Errorf("unclassified H3 task error")
		}
		result.Code, result.Reason = code, h3ErrorMessage(code)
	}
	return result, nil
}

func parseH3Usage(task *h3Task) types.H3UsageOverrides {
	var out types.H3UsageOverrides
	var resolution string
	if common.Unmarshal(task.Resolution, &resolution) == nil && (resolution == "768P" || resolution == "2K") {
		out.Resolution = resolution
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(task.Usage, &fields) != nil {
		return out
	}
	for _, field := range []struct {
		key      string
		min, max float64
		target   **float64
	}{{"output_seconds", 4, 15, &out.OutputSeconds}, {"input_seconds", 0, 15, &out.InputVideoSeconds}} {
		var v float64
		raw := fields[field.key]
		if len(raw) > 0 && string(raw) != "null" && common.Unmarshal(raw, &v) == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= field.min && v <= field.max {
			value := v
			*field.target = &value
		}
	}
	var images float64
	raw := fields["input_image_count"]
	if len(raw) > 0 && string(raw) != "null" && common.Unmarshal(raw, &images) == nil && images >= 0 && images <= 9 && math.Trunc(images) == images {
		n := int(images)
		out.InputImages = &n
	}
	return out
}

func validH3TaskID(id string) bool {
	if id == "" || len(id) > 256 || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-.", r)) {
			return false
		}
	}
	return true
}
