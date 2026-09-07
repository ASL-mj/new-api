package hailuo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/setting/task_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/abema/go-mp4"
	"github.com/gin-gonic/gin"
)

const h3FixtureRules = `{"version":"fixture-v1","base_quota":100,"duration_base_seconds":5,"resolution_ratios":{"768P":1,"2K":2},"image_reference_ratio_per_image":0.1,"video_reference_ratio_per_second":0.2,"audio_reference_ratio_per_audio":0.3}`

func h3TestContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func h3TestInfo(base, modelName string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{OriginModelName: modelName, UsingGroup: "default", UserGroup: "default",
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: 7, ChannelBaseUrl: base, ApiKey: "fixture-secret", UpstreamModelName: modelName},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_fixture"}}
}

func h3EnableFixture(t *testing.T) {
	t.Helper()
	t.Setenv("MINIMAX_H3_ENABLED", "true")
	t.Setenv("TASK_POLLING_V2_ENABLED", "true")
	t.Setenv("TASK_POLLING_V2_CHANNEL_IDS", "7")
	if err := task_setting.UpdateH3PricingRules(h3FixtureRules); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = task_setting.UpdateH3PricingRules("") })
	service.InitHttpClient()
}

func TestH3RequestValidation(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		bad        bool
	}{
		{"defaults", `{"prompt":"fixture"}`, false},
		{"fractional duration", `{"prompt":"fixture","duration":4.5}`, true},
		{"explicit zero", `{"prompt":"fixture","duration":0}`, true},
		{"string duration", `{"prompt":"fixture","duration":"5"}`, true},
		{"null duration", `{"prompt":"fixture","duration":null}`, true},
		{"range", `{"prompt":"fixture","duration":16}`, true},
		{"seconds alias", `{"prompt":"fixture","seconds":"6"}`, false},
		{"seconds conflict", `{"prompt":"fixture","duration":5,"seconds":"6"}`, true},
		{"resolution substring", `{"prompt":"fixture","size":"not2K"}`, true},
		{"resolution metadata", `{"prompt":"fixture","metadata":{"resolution":"2K"}}`, false},
		{"resolution conflict", `{"prompt":"fixture","size":"2K","resolution":"768P"}`, true},
		{"metadata string", `{"prompt":"fixture","metadata":"{}"}`, true},
		{"metadata array", `{"prompt":"fixture","metadata":[]}`, true},
		{"unknown metadata", `{"prompt":"fixture","metadata":{"seed":1}}`, true},
		{"model override", `{"prompt":"fixture","metadata":{"model":"other"}}`, true},
		{"content object", `{"prompt":"fixture","metadata":{"content":{}}}`, true},
		{"content null", `{"prompt":"fixture","metadata":{"content":null}}`, true},
		{"content item primitive", `{"prompt":"fixture","metadata":{"content":[1]}}`, true},
		{"content only prompt", `{"metadata":{"content":[{"type":"text","text":"metadata prompt"}]}}`, false},
		{"empty text", `{"prompt":"fallback","metadata":{"content":[{"type":"text","text":" "}]}}`, true},
		{"duplicate text", `{"metadata":{"content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}`, true},
		{"missing prompt", `{}`, true},
		{"watermark false", `{"prompt":"fixture","metadata":{"aigc_watermark":false}}`, false},
		{"watermark wrong type", `{"prompt":"fixture","metadata":{"aigc_watermark":"false"}}`, true},
		{"adaptive text", `{"prompt":"fixture","metadata":{"ratio":"adaptive"}}`, true},
		{"unknown ratio", `{"prompt":"fixture","metadata":{"ratio":"5:4"}}`, true},
		{"first frame", `{"prompt":"fixture","image":"https://media.example/frame.png"}`, false},
		{"duplicate first aliases", `{"prompt":"fixture","image":"https://media.example/a.png","images":["https://media.example/b.png"]}`, true},
		{"duplicate first frames", `{"prompt":"fixture","images":["https://media.example/a.png","https://media.example/b.png"]}`, true},
		{"frame mixed reference", `{"prompt":"fixture","image":"https://media.example/a.png","metadata":{"reference_video":"https://media.example/a.mp4"}}`, true},
		{"content media conflict", `{"prompt":"fixture","image":"https://media.example/a.png","metadata":{"content":[]}}`, true},
		{"unknown role", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","role":"middle_frame","image_url":{"url":"https://media.example/a.png"}}]}}`, true},
		{"string media object", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","image_url":"https://media.example/a.png"}]}}`, true},
		{"missing URL", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","image_url":{}}]}}`, true},
		{"wrong matching URL", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","audio_url":{"url":"https://media.example/a.wav"}}]}}`, true},
		{"unknown media field", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","image_url":{"url":"https://media.example/a.png","headers":{}}}]}}`, true},
		{"multiple media URLs", `{"prompt":"fixture","metadata":{"content":[{"type":"image_url","image_url":{"url":"https://media.example/a.png"},"video_url":{"url":"https://media.example/a.mp4"}}]}}`, true},
		{"reference convenience", `{"prompt":"fixture","metadata":{"reference_video":[{"url":"https://media.example/a.mp4"}],"reference_audio":"https://media.example/a.wav"}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, _, err := decodeH3Request([]byte(tt.body))
			if (err != nil) != tt.bad {
				t.Fatalf("req=%+v err=%v", req, err)
			}
		})
	}

	for _, tt := range []struct {
		kind, role string
		count      int
		bad        bool
	}{{"image_url", "reference_image", 9, false}, {"image_url", "reference_image", 10, true}, {"video_url", "reference_video", 3, false}, {"video_url", "reference_video", 4, true}, {"audio_url", "reference_audio", 3, false}, {"audio_url", "reference_audio", 4, true}} {
		t.Run(fmt.Sprintf("%s_%d", tt.kind, tt.count), func(t *testing.T) {
			items := []map[string]any{{"type": "text", "text": "fixture"}}
			for i := 0; i < tt.count; i++ {
				items = append(items, map[string]any{"type": tt.kind, "role": tt.role, tt.kind: map[string]any{"url": "https://media.example/asset"}})
			}
			body, _ := common.Marshal(map[string]any{"metadata": map[string]any{"content": items}})
			_, facts, err := decodeH3Request(body)
			if (err != nil) != tt.bad {
				t.Fatal(err)
			}
			if !tt.bad && tt.kind == "video_url" && facts.InputVideoSeconds != 15 {
				t.Fatal("missing video reservation")
			}
		})
	}
}

func TestH3AdaptorHTTPFixture(t *testing.T) {
	h3EnableFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing headers")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v2/video_generation":
			var request h3Request
			if err := common.DecodeJson(r.Body, &request); err != nil {
				t.Error(err)
			}
			if request.Model != H3Model || request.Duration == nil || *request.Duration != 5 || request.Ratio != "16:9" || request.Resolution != "768P" || request.AigcWatermark == nil || *request.AigcWatermark {
				t.Errorf("bad payload: %+v", request)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing content type")
			}
			_, _ = io.WriteString(w, `{"task_id":"upstream_fixture"}`)
		case "GET /v2/query/video_generation/upstream_fixture":
			if r.URL.RawQuery != "" {
				t.Error("query ID must be path-based")
			}
			_, _ = io.WriteString(w, `{"task":{"id":"upstream_fixture","model":"MiniMax-H3","status":"succeeded","content":{"url":"https://result.example/video.mp4"},"resolution":"2K","usage":{"output_seconds":5.5,"input_seconds":0,"input_image_count":0}}}`)
		default:
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, w := h3TestContext(`{"model":"old-alias","prompt":"fixture","metadata":{"aigc_watermark":false}}`)
	info := h3TestInfo(server.URL, H3Model)
	info.OriginModelName = "old-alias"
	a := &TaskAdaptor{}
	a.Init(info)
	if err := a.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatal(err)
	}
	handled, err := a.PrepareTaskBilling(c, info)
	if err != nil || !handled {
		t.Fatal(handled, err)
	}
	if info.PriceData.Quota != 100 || info.TaskPricingSnapshot == nil {
		t.Fatal("price not prepared")
	}
	body, err := a.BuildRequestBody(c, info)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.DoRequest(c, info, body)
	if err != nil {
		t.Fatal(err)
	}
	id, _, taskErr := a.DoResponse(c, resp, info)
	if taskErr != nil || id != "upstream_fixture" {
		t.Fatal(id, taskErr)
	}
	if w.Body.Len() != 0 || c.Writer.Written() {
		t.Fatal("provider wrote response before durable bookkeeping")
	}
	t.Setenv("MINIMAX_H3_ENABLED", "false")
	t.Setenv("TASK_POLLING_V2_ENABLED", "false")
	resp, err = a.FetchTask(server.URL, "fixture-secret", map[string]any{"task_id": id, "model": H3Model}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	result, err := a.ParseTaskResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.TaskStatusSuccess || result.TaskUsage == nil || result.Url != "https://result.example/video.mp4" {
		t.Fatalf("result %+v", result)
	}
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{TaskPricingSnapshot: info.TaskPricingSnapshot, GroupRatio: 1}}}
	if q := a.AdjustBillingOnComplete(task, result); q != 220 {
		t.Fatalf("completion quota %d", q)
	}
	if err := task_setting.UpdateH3PricingRules(strings.Replace(h3FixtureRules, `"base_quota":100`, `"base_quota":999`, 1)); err != nil {
		t.Fatal(err)
	}
	if q := a.AdjustBillingOnComplete(task, result); q != 220 {
		t.Fatal("used live prices")
	}
}

func TestH3SubmissionGates(t *testing.T) {
	h3EnableFixture(t)
	for _, tt := range []struct {
		name, enabled, poll, channels, pricing string
		bad                                    bool
	}{
		{"disabled", "false", "true", "7", h3FixtureRules, true},
		{"poll disabled", "true", "false", "7", h3FixtureRules, true},
		{"wrong channel", "true", "true", "8", h3FixtureRules, true},
		{"missing prices", "true", "true", "7", "", true},
		{"enabled", "true", "true", "7", h3FixtureRules, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MINIMAX_H3_ENABLED", tt.enabled)
			t.Setenv("TASK_POLLING_V2_ENABLED", tt.poll)
			t.Setenv("TASK_POLLING_V2_CHANNEL_IDS", tt.channels)
			if err := task_setting.UpdateH3PricingRules(tt.pricing); err != nil {
				t.Fatal(err)
			}
			c, _ := h3TestContext(`{"prompt":"fixture"}`)
			info := h3TestInfo("http://unused.invalid", H3Model)
			a := &TaskAdaptor{}
			a.Init(info)
			handled, err := a.PrepareTaskBilling(c, info)
			if !handled || (err != nil) != tt.bad {
				t.Fatal(handled, err)
			}
		})
	}
	c, _ := h3TestContext(`{"prompt":"fixture"}`)
	info := h3TestInfo("http://unused.invalid", H3Model)
	a := &TaskAdaptor{}
	a.Init(info)
	if _, err := a.BuildRequestBody(c, info); err == nil {
		t.Fatal("unprepared submission accepted")
	}
	info.UpstreamModelName = "MiniMax-Hailuo-2.3"
	if handled, err := a.PrepareTaskBilling(c, info); handled || err != nil {
		t.Fatal("legacy changed")
	}
	info.UpstreamModelName = "MiniMax-H3-Fast"
	uri, _ := a.BuildRequestURL(info)
	if strings.Contains(uri, "/v2/") {
		t.Fatal("alias implicitly selects H3")
	}
}

func TestH3MappedTargetUsesV2Adaptor(t *testing.T) {
	h3EnableFixture(t)
	c, _ := h3TestContext(`{"model":"MiniMax-H3","prompt":"fixture"}`)
	info := h3TestInfo("http://unused.invalid", H3Model)
	a := &TaskAdaptor{}
	a.Init(info)
	if taskErr := a.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatal(taskErr)
	}
	// RelayTaskSubmit applies the channel mapping after request validation.
	info.UpstreamModelName = "minimax_h3"
	handled, err := a.PrepareTaskBilling(c, info)
	if err != nil || !handled {
		t.Fatal(handled, err)
	}
	uri, err := a.BuildRequestURL(info)
	if err != nil || !strings.HasSuffix(uri, H3SubmitEndpoint) {
		t.Fatalf("mapped H3 target used the legacy endpoint: %s (%v)", uri, err)
	}
}

func TestH3ResponseFixtures(t *testing.T) {
	for _, tt := range []struct {
		name, body, status string
		code               int
		bad                bool
	}{
		{"queued", `{"task":{"status":"queued"}}`, model.TaskStatusQueued, 0, false},
		{"running", `{"task":{"status":"running"}}`, model.TaskStatusInProgress, 0, false},
		{"failed", `{"task":{"status":"failed","error":{"message":"Bearer secret","code":"1026"}}}`, model.TaskStatusFailure, 0, false},
		{"cancelled", `{"task":{"status":"cancelled"}}`, model.TaskStatusFailure, 0, false},
		{"string 429", `{"error":{"http_code":"429","message":"secret"}}`, "", 429, false},
		{"numeric 408", `{"error":{"code":408}}`, "", 408, false},
		{"numeric 503", `{"error":{"http_code":503}}`, "", 503, false},
		{"4xx", `{"error":{"http_code":"401"}}`, "", 401, false},
		{"unknown status", `{"task":{"status":"finished"}}`, "", 0, true},
		{"missing wrapper", `{"status":"succeeded"}`, "", 0, true},
		{"invalid JSON", `not JSON`, "", 0, true},
		{"unknown error", `{"error":{"message":"secret"}}`, "", 0, true},
		{"missing result", `{"task":{"status":"succeeded"}}`, "", 0, true},
		{"invalid result", `{"task":{"status":"succeeded","content":{"url":"javascript:alert(1)"}}}`, "", 0, true},
		{"wrong ID", `{"task":{"id":"other","status":"running"}}`, "", 0, true},
		{"wrong model", `{"task":{"model":"MiniMax-H3-Max","status":"running"}}`, "", 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseH3Task([]byte(tt.body), "fixture")
			if (err != nil) != tt.bad {
				t.Fatal(result, err)
			}
			if err == nil && (result.Status != tt.status || result.Code != tt.code || strings.Contains(result.Reason, "secret")) {
				t.Fatalf("bad result %+v", result)
			}
		})
	}
	for _, usage := range []string{`{}`, `null`, `[]`, `{"output_seconds":null,"input_image_count":null,"input_seconds":null}`, `{"output_seconds":"5","input_image_count":1.2,"input_seconds":16}`} {
		body := `{"task":{"status":"succeeded","resolution":123,"content":{"url":"https://result.example/video"},"usage":` + usage + `}}`
		result, err := parseH3Task([]byte(body), "")
		if err != nil {
			t.Fatal(err)
		}
		facts := types.H3UsageFacts{OutputSeconds: 5, Resolution: "2K", InputImages: 2, InputVideoSeconds: 15, ReferenceAudios: 1}
		if got := result.TaskUsage.Merge(facts); got != facts {
			t.Fatalf("invalid usage accepted %+v", got)
		}
	}
}

func TestH3SubmitErrorsAndLegacy(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		bad        bool
	}{
		{"valid", `{"task_id":"fixture"}`, 200, false}, {"missing", `{}`, 200, true}, {"empty", `{"task_id":""}`, 200, true},
		{"path injection", `{"task_id":"../other?key=secret"}`, 200, true}, {"embedded", `{"error":{"http_code":"429","message":"secret"}}`, 200, true},
		{"http error", `{}`, 503, true}, {"bad json", `<html>secret</html>`, 200, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, w := h3TestContext(`{}`)
			a := &TaskAdaptor{}
			info := h3TestInfo("", H3Model)
			_, _, err := a.DoResponse(c, &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, info)
			if (err != nil) != tt.bad {
				t.Fatal(err)
			}
			if err != nil && strings.Contains(err.Message, "secret") {
				t.Fatal("secret exposed")
			}
			if tt.bad && w.Body.Len() > 0 {
				t.Fatal("error wrote success")
			}
		})
	}
	info := h3TestInfo("http://fixture", "MiniMax-Hailuo-2.3")
	a := &TaskAdaptor{}
	a.Init(info)
	uri, _ := a.BuildRequestURL(info)
	if uri != "http://fixture/v1/video_generation" {
		t.Fatal(uri)
	}
	res, err := a.ParseTaskResult([]byte(`{"task_id":"legacy","status":"Processing","base_resp":{"status_code":0}}`))
	if err != nil || res.Status != model.TaskStatusInProgress {
		t.Fatal(res, err)
	}
}

func TestH3MediaSSRFAndBounds(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/a", "http://169.254.169.254/latest", "file:///tmp/a", "data:image/png;base64,AAAA", "mm_file://id", "https://user:pass@example.com/a", "https://example.com/a#fragment"} {
		if err := validateH3URL(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	old := *system_setting.GetFetchSetting()
	t.Cleanup(func() { *system_setting.GetFetchSetting() = old })
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 256, 256))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("media credentials leaked")
		}
		switch r.URL.Path {
		case "/image":
			_, _ = w.Write(imageData.Bytes())
		case "/bad":
			_, _ = io.WriteString(w, "not an image")
		case "/oversize":
			w.Header().Set("Content-Length", "999999999")
		case "/redirect":
			http.Redirect(w, r, "file:///tmp/secret", 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		case "/audio":
			_, _ = w.Write(h3WAVFixture(8))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	for _, tt := range []struct {
		path string
		bad  bool
	}{{"/image", false}, {"/bad", true}, {"/oversize", true}, {"/redirect", true}, {"/loop", true}, {"/missing", true}} {
		body, _ := common.Marshal(map[string]any{"prompt": "fixture", "image": server.URL + tt.path})
		req, _, err := decodeH3Request(body)
		if err != nil {
			t.Fatal(err)
		}
		err = validateH3Media(context.Background(), req, server.Client())
		if (err != nil) != tt.bad {
			t.Fatalf("%s: %v", tt.path, err)
		}
	}
	if _, err := downloadH3Media(context.Background(), server.Client(), server.URL+"/image", 5); err == nil {
		t.Fatal("streamed byte limit ignored")
	}
	body, _ := common.Marshal(map[string]any{"prompt": "fixture", "metadata": map[string]any{"reference_audio": []string{server.URL + "/audio", server.URL + "/audio"}}})
	req, _, err := decodeH3Request(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateH3Media(context.Background(), req, server.Client()); err == nil {
		t.Fatal("audio total over 15 seconds accepted")
	}
}

func h3WAVFixture(seconds int) []byte {
	const rate = 8000
	data := make([]byte, 44+seconds*rate*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], rate)
	binary.LittleEndian.PutUint32(data[28:32], rate*2)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(len(data)-44))
	return data
}

func TestH3UnknownSubmissionOutcomes(t *testing.T) {
	statusAdaptor := &TaskAdaptor{}
	if !statusAdaptor.HandlesTaskSubmissionHTTPStatus(h3TestInfo("", H3Model)) {
		t.Fatal("H3 non-200 response would bypass H3 classification")
	}
	if statusAdaptor.HandlesTaskSubmissionHTTPStatus(h3TestInfo("", "MiniMax-Hailuo-2.3")) {
		t.Fatal("legacy Hailuo non-200 response handling changed")
	}
	for _, tt := range []struct {
		name, body string
		status     int
		unknown    bool
	}{
		{"missing id", `{}`, 200, true},
		{"malformed success", `<html>secret</html>`, 200, true},
		{"server failure", `{"error":{"http_code":"400"}}`, 503, true},
		{"timeout", `{}`, 408, true},
		{"embedded server failure", `{"error":{"http_code":503}}`, 200, true},
		{"unclassified embedded", `{"error":{"message":"secret"}}`, 200, true},
		{"bad request", `{"error":{"http_code":400,"message":"secret"}}`, 400, false},
		{"rate limit", `{"error":{"http_code":"429"}}`, 200, false},
		{"malformed rejection", `<html>secret</html>`, 401, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, w := h3TestContext(`{}`)
			a := &TaskAdaptor{}
			info := h3TestInfo("", H3Model)
			_, _, err := a.DoResponse(c, &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, info)
			if err == nil || (err.Code == H3SubmitOutcomeUnknownCode) != tt.unknown {
				t.Fatal(err)
			}
			if w.Body.Len() != 0 || c.Writer.Written() || strings.Contains(err.Message, "secret") {
				t.Fatal("response leaked before bookkeeping")
			}
		})
	}
	h3EnableFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	c, _ := h3TestContext(`{"prompt":"fixture"}`)
	info := h3TestInfo(server.URL, H3Model)
	a := &TaskAdaptor{}
	a.Init(info)
	if _, err := a.PrepareTaskBilling(c, info); err != nil {
		t.Fatal(err)
	}
	_, err := a.DoRequest(c, info, nil)
	var unknown *H3SubmissionOutcomeUnknownError
	if !errors.As(err, &unknown) || !unknown.SubmissionOutcomeUnknown() {
		t.Fatalf("missing unknown transport marker: %v", err)
	}
}

func TestH3RequestFreezeAndLimits(t *testing.T) {
	h3EnableFixture(t)
	c, _ := h3TestContext(`{"metadata":{"content":[{"type":"text","text":"metadata prompt"}]}}`)
	info := h3TestInfo("http://unused.invalid", "alias")
	a := &TaskAdaptor{}
	a.Init(info)
	if err := a.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatal(err)
	}
	info.UpstreamModelName = H3Model
	if handled, err := a.PrepareTaskBilling(c, info); !handled || err != nil {
		t.Fatal(handled, err)
	}
	if a.h3Request.Content[0].Text != "metadata prompt" {
		t.Fatal("metadata prompt lost")
	}
	if err := task_setting.UpdateH3PricingRules(strings.Replace(h3FixtureRules, `"base_quota":100`, `"base_quota":900`, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PrepareTaskBilling(c, info); err != nil || info.PriceData.Quota != 100 {
		t.Fatal("retry changed frozen price", err)
	}
	c2, _ := h3TestContext(`{"prompt":"fixture","duration":6}`)
	if _, err := a.PrepareTaskBilling(c2, info); err == nil {
		t.Fatal("changed retry facts accepted")
	}
	if _, _, err := decodeH3Request([]byte(`{"prompt":"` + strings.Repeat("x", 7001) + `"}`)); err == nil {
		t.Fatal("text bound ignored")
	}
	if _, _, err := decodeH3Request(make([]byte, h3MaxRequestBytes+1)); err == nil {
		t.Fatal("body limit ignored")
	}
	t.Setenv("MINIMAX_H3_ENABLED", "false")
	if _, err := a.BuildRequestBody(c, info); err == nil {
		t.Fatal("gate disabled after prepare was ignored")
	}
}

func TestH3VideoMetadataBounds(t *testing.T) {
	for _, tt := range []struct {
		name          string
		seconds       float64
		width, height uint32
		bad           bool
	}{
		{"valid", 5, 256, 256, false}, {"fractional", 5.25, 512, 256, false},
		{"short", 1, 256, 256, true}, {"long", 15.1, 256, 256, true},
		{"missing visual", 5, 0, 0, true}, {"small", 5, 255, 256, true}, {"aspect", 5, 1024, 256, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := h3MP4Fixture(t, tt.seconds, tt.width, tt.height)
			seconds, err := h3VideoDuration(data)
			if (err != nil) != tt.bad || (!tt.bad && seconds != tt.seconds) {
				t.Fatal(seconds, err)
			}
		})
	}
	if _, err := h3VideoDuration([]byte("not a movie")); err == nil {
		t.Fatal("invalid movie accepted")
	}
	old := *system_setting.GetFetchSetting()
	t.Cleanup(func() { *system_setting.GetFetchSetting() = old })
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	data := h3MP4Fixture(t, 8, 256, 256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	body, _ := common.Marshal(map[string]any{"prompt": "fixture", "metadata": map[string]any{"reference_video": []string{server.URL, server.URL}}})
	req, _, err := decodeH3Request(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateH3Media(context.Background(), req, server.Client()); err == nil {
		t.Fatal("video duration total not enforced")
	}
}

// These fixtures test container metadata validation, not codec decodability.
func h3MP4Fixture(t *testing.T, seconds float64, width, height uint32) []byte {
	t.Helper()
	box := func(name string, payload []byte) []byte {
		b := make([]byte, len(payload)+8)
		binary.BigEndian.PutUint32(b, uint32(len(b)))
		copy(b[4:8], name)
		copy(b[8:], payload)
		return b
	}
	marshal := func(b mp4.IImmutableBox) []byte {
		var out bytes.Buffer
		if _, err := mp4.Marshal(&out, b, mp4.Context{}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	mvhd := marshal(&mp4.Mvhd{Timescale: 1000, DurationV0: uint32(seconds * 1000)})
	tkhd := marshal(&mp4.Tkhd{Width: width << 16, Height: height << 16})
	return box("moov", append(box("mvhd", mvhd), box("trak", box("tkhd", tkhd))...))
}
