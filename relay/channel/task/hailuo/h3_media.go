package hailuo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/setting/task_setting"
	"github.com/abema/go-mp4"
	_ "golang.org/x/image/webp"
)

func validateH3URL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("H3 media requires an HTTP(S) URL without credentials or fragment")
	}
	s := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(raw, s.EnableSSRFProtection, s.AllowPrivateIp, s.DomainFilterMode, s.IpFilterMode, s.DomainList, s.IpList, s.AllowedPorts, s.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("H3 URL blocked by fetch policy")
	}
	return nil
}

func validateH3Media(ctx context.Context, request *h3Request, client *http.Client) error {
	if request.CallbackURL != "" {
		if !task_setting.H3CallbackAllowed(request.CallbackURL) {
			return fmt.Errorf("H3 callback URL is not allowlisted")
		}
		if err := validateH3URL(request.CallbackURL); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	remaining := int64(128 << 20)
	videoSeconds, audioSeconds := 0.0, 0.0
	for _, item := range request.Content {
		var raw string
		var limit int64
		switch item.Type {
		case "image_url":
			raw, limit = item.ImageURL.URL, 30<<20
		case "video_url":
			raw, limit = item.VideoURL.URL, 50<<20
		case "audio_url":
			raw, limit = item.AudioURL.URL, 15<<20
		default:
			continue
		}
		if limit > remaining {
			limit = remaining
		}
		data, err := downloadH3Media(ctx, client, raw, limit)
		if err != nil {
			return err
		}
		remaining -= int64(len(data))
		switch item.Type {
		case "image_url":
			cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil || !contains([]string{"jpeg", "png", "webp"}, format) || !h3Dimensions(float64(cfg.Width), float64(cfg.Height)) {
				return fmt.Errorf("H3 image must be valid JPEG, PNG or WEBP with supported dimensions")
			}
		case "video_url":
			seconds, err := h3VideoDuration(data)
			if err != nil {
				return err
			}
			videoSeconds += seconds
		case "audio_url":
			ext := ".mp3"
			if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
				ext = ".wav"
			}
			seconds, err := common.GetAudioDuration(ctx, bytes.NewReader(data), ext)
			if err != nil || !h3DurationInBounds(seconds) {
				return fmt.Errorf("H3 audio must be WAV or MP3 of 2-15 seconds")
			}
			audioSeconds += seconds
		}
		if videoSeconds > 15 || audioSeconds > 15 {
			return fmt.Errorf("H3 reference video/audio total duration exceeds 15 seconds")
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("H3 media validation timed out")
		}
	}
	return nil
}

func downloadH3Media(ctx context.Context, client *http.Client, raw string, limit int64) ([]byte, error) {
	if err := validateH3URL(raw); err != nil {
		return nil, err
	}
	if client == nil || limit <= 0 {
		return nil, fmt.Errorf("H3 media validation client or byte budget unavailable")
	}
	bounded := *client
	bounded.Timeout = 10 * time.Second
	bounded.Jar = nil
	bounded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("H3 media redirect limit exceeded")
		}
		return validateH3URL(req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid H3 media URL")
	}
	// Media requests never carry the provider's bearer token or caller cookies.
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, fmt.Errorf("H3 media is not accessible")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil, fmt.Errorf("H3 media exceeds size limit or is not accessible")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit || len(data) == 0 {
		return nil, fmt.Errorf("H3 media exceeds size limit or is incomplete")
	}
	return data, nil
}

func h3VideoDuration(data []byte) (float64, error) {
	reader := bytes.NewReader(data)
	// Extract only bounded metadata, avoiding sample-table allocation and decoding
	// attacker-controlled media. go-mp4 is already used by common/audio.go.
	headers, err := mp4.ExtractBoxWithPayload(reader, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()})
	if err != nil || len(headers) != 1 {
		return 0, fmt.Errorf("H3 video must contain a valid MP4/MOV movie header")
	}
	header, ok := headers[0].Payload.(*mp4.Mvhd)
	if !ok || header.Timescale == 0 {
		return 0, fmt.Errorf("invalid H3 video timescale")
	}
	seconds := float64(header.GetDuration()) / float64(header.Timescale)
	if !h3DurationInBounds(seconds) {
		return 0, fmt.Errorf("H3 reference video duration must be 2-15 seconds")
	}
	tracks, err := mp4.ExtractBoxWithPayload(reader, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeTkhd()})
	if err != nil {
		return 0, fmt.Errorf("invalid H3 video tracks")
	}
	visual := false
	for _, track := range tracks {
		if tkhd, ok := track.Payload.(*mp4.Tkhd); ok && tkhd.GetWidth() > 0 {
			if !h3Dimensions(tkhd.GetWidth(), tkhd.GetHeight()) {
				return 0, fmt.Errorf("H3 video dimensions outside supported bounds")
			}
			visual = true
		}
	}
	if !visual {
		return 0, fmt.Errorf("H3 video has no visual track")
	}
	return seconds, nil
}

func h3Dimensions(w, h float64) bool {
	return w >= 256 && w <= 5760 && h >= 256 && h <= 5760 && w/h >= 0.4 && w/h <= 2.5
}
func h3DurationInBounds(seconds float64) bool {
	return !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= 2 && seconds <= 15
}

func h3ProviderClient(proxy string) (*http.Client, error) {
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil || client == nil {
		return nil, fmt.Errorf("H3 HTTP client unavailable")
	}
	bounded := *client
	bounded.Timeout = 30 * time.Second
	// Do not redirect credential-bearing API requests to another endpoint.
	bounded.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &bounded, nil
}

func h3ResultURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.TrimSpace(raw) == raw && len(raw) <= 8192 && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
