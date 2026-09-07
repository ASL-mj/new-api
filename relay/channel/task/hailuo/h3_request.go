package hailuo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	H3Model           = "MiniMax-H3"
	H3MappedModel     = "minimax_h3"
	H3SubmitEndpoint  = "/v2/video_generation"
	H3QueryEndpoint   = "/v2/query/video_generation/"
	h3MaxRequestBytes = 1 << 20
)

func IsH3ModelName(name string) bool {
	switch strings.TrimSpace(name) {
	case H3Model, H3MappedModel:
		return true
	default:
		return false
	}
}

type h3MediaURL struct {
	URL string `json:"url"`
}
type h3ContentItem struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Role     string      `json:"role,omitempty"`
	ImageURL *h3MediaURL `json:"image_url,omitempty"`
	VideoURL *h3MediaURL `json:"video_url,omitempty"`
	AudioURL *h3MediaURL `json:"audio_url,omitempty"`
}

type h3Request struct {
	Model         string          `json:"model"`
	Content       []h3ContentItem `json:"content"`
	Resolution    string          `json:"resolution"`
	Duration      *int            `json:"duration"`
	Ratio         string          `json:"ratio"`
	CallbackURL   string          `json:"callback_url,omitempty"`
	AigcWatermark *bool           `json:"aigc_watermark,omitempty"`
}

// Separate from TaskSubmitReq: its legacy custom decoder silently drops malformed
// duration/metadata values, which is unsuitable for a billable H3 request.
type h3Input struct {
	Model          string          `json:"model"`
	Prompt         string          `json:"prompt"`
	Image          string          `json:"image"`
	Images         []string        `json:"images"`
	InputReference string          `json:"input_reference"`
	Duration       *int            `json:"duration"`
	Seconds        string          `json:"seconds"`
	Size           string          `json:"size"`
	Resolution     string          `json:"resolution"`
	Mode           string          `json:"mode"`
	Metadata       json.RawMessage `json:"metadata"`
}

type h3Metadata struct {
	Content         json.RawMessage `json:"content"`
	Ratio           string          `json:"ratio"`
	Resolution      string          `json:"resolution"`
	FirstFrameImage string          `json:"first_frame_image"`
	LastFrameImage  string          `json:"last_frame_image"`
	ReferenceVideo  json.RawMessage `json:"reference_video"`
	ReferenceAudio  json.RawMessage `json:"reference_audio"`
	CallbackURL     string          `json:"callback_url"`
	AigcWatermark   *bool           `json:"aigc_watermark"`
}

func strictH3Object(raw []byte, allowed []string) error {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("H3 field must be an object")
	}
	for key, value := range fields {
		if !contains(allowed, key) {
			return fmt.Errorf("unsupported H3 field %q", key)
		}
		if string(value) == "null" {
			return fmt.Errorf("H3 field %q must not be null", key)
		}
	}
	return nil
}

func decodeH3Request(raw []byte) (*h3Request, types.H3UsageFacts, error) {
	facts := types.H3UsageFacts{}
	if len(raw) > h3MaxRequestBytes {
		return nil, facts, fmt.Errorf("H3 request exceeds 1 MiB")
	}
	if err := strictH3Object(raw, []string{"model", "prompt", "image", "images", "input_reference", "duration", "seconds", "size", "resolution", "metadata", "mode"}); err != nil {
		return nil, facts, err
	}
	var input h3Input
	if err := common.Unmarshal(raw, &input); err != nil {
		return nil, facts, fmt.Errorf("invalid H3 request field type")
	}
	if input.Mode != "" {
		return nil, facts, fmt.Errorf("H3 mode is not supported")
	}
	var meta h3Metadata
	if len(input.Metadata) > 0 {
		if err := strictH3Object(input.Metadata, []string{"content", "ratio", "resolution", "first_frame_image", "last_frame_image", "reference_video", "reference_audio", "callback_url", "aigc_watermark"}); err != nil {
			return nil, facts, err
		}
		if err := common.Unmarshal(input.Metadata, &meta); err != nil {
			return nil, facts, fmt.Errorf("invalid H3 metadata field type")
		}
	}
	duration := 5
	if input.Duration != nil {
		duration = *input.Duration
	}
	if input.Seconds != "" {
		seconds, err := strconv.Atoi(input.Seconds)
		if err != nil || (input.Duration != nil && seconds != duration) {
			return nil, facts, fmt.Errorf("invalid or conflicting H3 seconds")
		}
		duration = seconds
	}
	if duration < 4 || duration > 15 {
		return nil, facts, fmt.Errorf("H3 duration must be an integer from 4 to 15")
	}
	resolution := ""
	for _, value := range []string{input.Size, input.Resolution, meta.Resolution} {
		if value == "" {
			continue
		}
		if value != "768P" && value != "2K" {
			return nil, facts, fmt.Errorf("H3 resolution must be 768P or 2K")
		}
		if resolution != "" && resolution != value {
			return nil, facts, fmt.Errorf("conflicting H3 resolutions")
		}
		resolution = value
	}
	if resolution == "" {
		resolution = "768P"
	}
	content, err := buildH3Content(input, meta)
	if err != nil {
		return nil, facts, err
	}
	facts, visual, err := validateH3Content(content)
	if err != nil {
		return nil, facts, err
	}
	facts.OutputSeconds, facts.Resolution = float64(duration), resolution
	ratio := meta.Ratio
	if ratio == "" {
		if visual {
			ratio = "adaptive"
		} else {
			ratio = "16:9"
		}
	}
	if !contains([]string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}, ratio) || (ratio == "adaptive" && !visual) {
		return nil, facts, fmt.Errorf("invalid H3 ratio for supplied media")
	}
	return &h3Request{Model: H3Model, Content: content, Duration: &duration, Resolution: resolution, Ratio: ratio, CallbackURL: meta.CallbackURL, AigcWatermark: meta.AigcWatermark}, facts, nil
}

func buildH3Content(input h3Input, meta h3Metadata) ([]h3ContentItem, error) {
	var content []h3ContentItem
	if len(meta.Content) > 0 {
		if input.Image != "" || len(input.Images) > 0 || input.InputReference != "" || meta.FirstFrameImage != "" || meta.LastFrameImage != "" || len(meta.ReferenceVideo) > 0 || len(meta.ReferenceAudio) > 0 {
			return nil, fmt.Errorf("H3 content and convenience media fields are mutually exclusive")
		}
		var items []json.RawMessage
		if err := common.Unmarshal(meta.Content, &items); err != nil || items == nil {
			return nil, fmt.Errorf("H3 content must be an array")
		}
		if len(items) > 16 {
			return nil, fmt.Errorf("too many H3 content items")
		}
		for _, raw := range items {
			if err := strictH3Object(raw, []string{"type", "text", "role", "image_url", "video_url", "audio_url"}); err != nil {
				return nil, err
			}
			var fields map[string]json.RawMessage
			_ = common.Unmarshal(raw, &fields)
			for _, key := range []string{"image_url", "video_url", "audio_url"} {
				if media, ok := fields[key]; ok {
					if err := strictH3Object(media, []string{"url"}); err != nil {
						return nil, err
					}
				}
			}
			var item h3ContentItem
			if err := common.Unmarshal(raw, &item); err != nil {
				return nil, fmt.Errorf("invalid H3 content field type")
			}
			content = append(content, item)
		}
	} else {
		// image/images/input_reference are compatibility aliases for ONE first
		// frame, never implicit reference images or an inferred last frame.
		frames := append([]string{}, input.Images...)
		for _, image := range []string{input.Image, input.InputReference, meta.FirstFrameImage} {
			if image != "" {
				frames = append(frames, image)
			}
		}
		if len(frames) > 1 {
			return nil, fmt.Errorf("H3 accepts at most one first frame")
		}
		for _, image := range frames {
			content = append(content, h3ContentItem{Type: "image_url", Role: "first_frame", ImageURL: &h3MediaURL{URL: image}})
		}
		if meta.LastFrameImage != "" {
			content = append(content, h3ContentItem{Type: "image_url", Role: "last_frame", ImageURL: &h3MediaURL{URL: meta.LastFrameImage}})
		}
		for _, source := range []struct {
			raw        json.RawMessage
			kind, role string
		}{{meta.ReferenceVideo, "video_url", "reference_video"}, {meta.ReferenceAudio, "audio_url", "reference_audio"}} {
			urls, err := h3MediaList(source.raw)
			if err != nil {
				return nil, err
			}
			for _, u := range urls {
				item := h3ContentItem{Type: source.kind, Role: source.role}
				if source.kind == "video_url" {
					item.VideoURL = &h3MediaURL{URL: u}
				} else {
					item.AudioURL = &h3MediaURL{URL: u}
				}
				content = append(content, item)
			}
		}
	}
	hasText := false
	for _, item := range content {
		if item.Type == "text" {
			hasText = true
		}
	}
	if !hasText && strings.TrimSpace(input.Prompt) != "" {
		content = append([]h3ContentItem{{Type: "text", Text: input.Prompt}}, content...)
	}
	return content, nil
}

func h3MediaList(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var items []json.RawMessage
	if raw[0] == '[' {
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("invalid H3 media list")
		}
	} else {
		items = []json.RawMessage{raw}
	}
	if len(items) == 0 || len(items) > 3 {
		return nil, fmt.Errorf("H3 reference media count must be 1-3")
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		var u string
		if err := common.Unmarshal(item, &u); err != nil {
			if err := strictH3Object(item, []string{"url"}); err != nil {
				return nil, err
			}
			var media h3MediaURL
			if err := common.Unmarshal(item, &media); err != nil {
				return nil, fmt.Errorf("invalid H3 media URL")
			}
			u = media.URL
		}
		if strings.TrimSpace(u) == "" {
			return nil, fmt.Errorf("H3 media URL is required")
		}
		urls = append(urls, u)
	}
	return urls, nil
}

func validateH3Content(content []h3ContentItem) (types.H3UsageFacts, bool, error) {
	facts := types.H3UsageFacts{}
	first, last, refs, videos, texts := 0, 0, 0, 0, 0
	for i := range content {
		item := &content[i]
		mediaCount := 0
		for _, media := range []*h3MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media != nil {
				mediaCount++
				if strings.TrimSpace(media.URL) == "" {
					return facts, false, fmt.Errorf("H3 media URL is required")
				}
			}
		}
		if item.Type == "text" {
			if mediaCount != 0 || item.Role != "" || strings.TrimSpace(item.Text) == "" || utf8.RuneCountInString(item.Text) > 7000 {
				return facts, false, fmt.Errorf("invalid H3 text item")
			}
			texts++
			continue
		}
		if mediaCount != 1 || item.Text != "" {
			return facts, false, fmt.Errorf("H3 media item must have exactly one matching URL")
		}
		switch item.Type {
		case "image_url":
			if item.ImageURL == nil {
				return facts, false, fmt.Errorf("H3 image URL is required")
			}
			if item.Role == "" {
				item.Role = "first_frame"
			}
			switch item.Role {
			case "first_frame":
				first++
			case "last_frame":
				last++
			case "reference_image":
				refs++
			default:
				return facts, false, fmt.Errorf("unsupported H3 image role")
			}
			facts.InputImages++
		case "video_url":
			if item.VideoURL == nil || item.Role != "reference_video" {
				return facts, false, fmt.Errorf("invalid H3 video role or URL")
			}
			videos++
			refs++
		case "audio_url":
			if item.AudioURL == nil || item.Role != "reference_audio" {
				return facts, false, fmt.Errorf("invalid H3 audio role or URL")
			}
			facts.ReferenceAudios++
			refs++
		default:
			return facts, false, fmt.Errorf("unsupported H3 content type")
		}
	}
	if texts != 1 || first > 1 || last > 1 || facts.InputImages > 9 || videos > 3 || facts.ReferenceAudios > 3 || ((first+last) > 0 && refs > 0) {
		return facts, false, fmt.Errorf("invalid H3 content counts or mixed frame/reference inputs")
	}
	if videos > 0 {
		facts.InputVideoSeconds = 15
	}
	return facts, facts.InputImages > 0 || videos > 0, nil
}
