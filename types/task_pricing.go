package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const H3PricingKind = "minimax_h3"

// H3PricingRule contains configured multipliers, never built-in provider prices.
// Reference factors are 1 + quantity * rate; duration is continuous.
type H3PricingRule struct {
	DurationBaseSeconds          float64            `json:"duration_base_seconds"`
	ResolutionRatios             map[string]float64 `json:"resolution_ratios"`
	ImageReferenceRatioPerImage  float64            `json:"image_reference_ratio_per_image"`
	VideoReferenceRatioPerSecond float64            `json:"video_reference_ratio_per_second"`
	AudioReferenceRatioPerAudio  float64            `json:"audio_reference_ratio_per_audio"`
}

type H3UsageFacts struct {
	OutputSeconds     float64 `json:"output_seconds"`
	Resolution        string  `json:"resolution"`
	InputImages       int     `json:"input_images"`
	InputVideoSeconds float64 `json:"input_video_seconds"`
	ReferenceAudios   int     `json:"reference_audios"`
}

// H3UsageOverrides is a partial provider observation. Audio counts remain frozen
// because H3 does not report a reference-audio count.
type H3UsageOverrides struct {
	OutputSeconds     *float64 `json:"output_seconds,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	InputImages       *int     `json:"input_images,omitempty"`
	InputVideoSeconds *float64 `json:"input_video_seconds,omitempty"`
}

func (u H3UsageOverrides) Merge(f H3UsageFacts) H3UsageFacts {
	if u.OutputSeconds != nil && inRange(*u.OutputSeconds, 4, 15) {
		f.OutputSeconds = *u.OutputSeconds
	}
	if u.Resolution == "768P" || u.Resolution == "2K" {
		f.Resolution = u.Resolution
	}
	if u.InputImages != nil && *u.InputImages >= 0 && *u.InputImages <= 9 {
		f.InputImages = *u.InputImages
	}
	if u.InputVideoSeconds != nil && inRange(*u.InputVideoSeconds, 0, 15) {
		f.InputVideoSeconds = *u.InputVideoSeconds
	}
	return f
}

type TaskPricingSnapshot struct {
	Kind      string        `json:"kind"`
	Version   string        `json:"version"`
	RuleHash  string        `json:"rule_hash"`
	BaseQuota int           `json:"base_quota"`
	H3Rule    H3PricingRule `json:"h3_rule"`
	Submitted H3UsageFacts  `json:"submitted"`
}

func (r H3PricingRule) Validate() error {
	if !finite(r.DurationBaseSeconds) || r.DurationBaseSeconds <= 0 {
		return fmt.Errorf("duration_base_seconds must be finite and positive")
	}
	if len(r.ResolutionRatios) != 2 {
		return fmt.Errorf("resolution_ratios must specify exactly 768P and 2K")
	}
	for _, key := range []string{"768P", "2K"} {
		v := r.ResolutionRatios[key]
		if !finite(v) || v <= 0 {
			return fmt.Errorf("resolution ratio %s must be finite and positive", key)
		}
	}
	for _, v := range []float64{r.ImageReferenceRatioPerImage, r.VideoReferenceRatioPerSecond, r.AudioReferenceRatioPerAudio} {
		if !finite(v) || v < 0 {
			return fmt.Errorf("reference rates must be finite and non-negative")
		}
	}
	return nil
}

func (f H3UsageFacts) Validate() error {
	if !inRange(f.OutputSeconds, 4, 15) || (f.Resolution != "768P" && f.Resolution != "2K") ||
		f.InputImages < 0 || f.InputImages > 9 || !inRange(f.InputVideoSeconds, 0, 15) || f.ReferenceAudios < 0 || f.ReferenceAudios > 3 {
		return fmt.Errorf("H3 usage is outside supported bounds")
	}
	return nil
}

func NewH3PricingSnapshot(version string, baseQuota int, rule H3PricingRule, submitted H3UsageFacts) (*TaskPricingSnapshot, error) {
	if strings.TrimSpace(version) == "" || len(version) > 128 || baseQuota <= 0 {
		return nil, fmt.Errorf("H3 pricing requires a version and positive base_quota")
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	if err := submitted.Validate(); err != nil {
		return nil, err
	}
	ratios := make(map[string]float64, len(rule.ResolutionRatios))
	for k, v := range rule.ResolutionRatios {
		ratios[k] = v
	}
	rule.ResolutionRatios = ratios
	s := &TaskPricingSnapshot{Kind: H3PricingKind, Version: version, BaseQuota: baseQuota, H3Rule: rule, Submitted: submitted}
	hash, err := s.ruleHash()
	if err != nil {
		return nil, err
	}
	s.RuleHash = hash
	// Validate the entire supported price surface, not just the submitted mode.
	for _, resolution := range []string{"768P", "2K"} {
		for _, f := range []H3UsageFacts{{OutputSeconds: 4, Resolution: resolution}, {OutputSeconds: 15, Resolution: resolution, InputImages: 9, InputVideoSeconds: 15, ReferenceAudios: 3}} {
			if _, err := s.StandardQuota(f); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}

func (s *TaskPricingSnapshot) ruleHash() (string, error) {
	b, err := common.Marshal(struct {
		Kind      string        `json:"kind"`
		Version   string        `json:"version"`
		BaseQuota int           `json:"base_quota"`
		Rule      H3PricingRule `json:"rule"`
	}{s.Kind, s.Version, s.BaseQuota, s.H3Rule})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func (s *TaskPricingSnapshot) Validate() error {
	if s == nil || s.Kind != H3PricingKind || strings.TrimSpace(s.Version) == "" || s.BaseQuota <= 0 {
		return fmt.Errorf("invalid H3 pricing snapshot")
	}
	if err := s.H3Rule.Validate(); err != nil {
		return err
	}
	if err := s.Submitted.Validate(); err != nil {
		return err
	}
	h, err := s.ruleHash()
	if err != nil {
		return err
	}
	if h != s.RuleHash {
		return fmt.Errorf("H3 pricing snapshot hash mismatch")
	}
	return nil
}

func (s *TaskPricingSnapshot) rawQuota(f H3UsageFacts) (float64, error) {
	if err := s.Validate(); err != nil {
		return 0, err
	}
	if err := f.Validate(); err != nil {
		return 0, err
	}
	r := s.H3Rule
	v := float64(s.BaseQuota) * (f.OutputSeconds / r.DurationBaseSeconds) * r.ResolutionRatios[f.Resolution]
	v *= 1 + float64(f.InputImages)*r.ImageReferenceRatioPerImage
	v *= 1 + f.InputVideoSeconds*r.VideoReferenceRatioPerSecond
	v *= 1 + float64(f.ReferenceAudios)*r.AudioReferenceRatioPerAudio
	return v, nil
}

func roundedQuota(v float64) (int, error) {
	v = math.Round(v)
	if !finite(v) || v < 0 || v >= math.Ldexp(1, strconv.IntSize-1) {
		return 0, fmt.Errorf("H3 quota overflow or invalid value")
	}
	return int(v), nil
}

func (s *TaskPricingSnapshot) StandardQuota(f H3UsageFacts) (int, error) {
	v, err := s.rawQuota(f)
	if err != nil {
		return 0, err
	}
	n, err := roundedQuota(v)
	if err == nil && n == 0 {
		return 0, fmt.Errorf("configured H3 standard price rounds to zero")
	}
	return n, err
}

// Quota rounds once, after the frozen group multiplier is applied. An explicitly
// configured zero group ratio is supported, unlike a missing standard price.
func (s *TaskPricingSnapshot) Quota(f H3UsageFacts, groupRatio float64) (int, error) {
	if !finite(groupRatio) || groupRatio < 0 {
		return 0, fmt.Errorf("invalid group ratio")
	}
	v, err := s.rawQuota(f)
	if err != nil {
		return 0, err
	}
	return roundedQuota(v * groupRatio)
}

func (s *TaskPricingSnapshot) OtherRatios() (map[string]float64, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	f, r := s.Submitted, s.H3Rule
	return map[string]float64{
		"duration":            f.OutputSeconds / r.DurationBaseSeconds,
		"resolution":          r.ResolutionRatios[f.Resolution],
		"input_images":        1 + float64(f.InputImages)*r.ImageReferenceRatioPerImage,
		"input_video_seconds": 1 + f.InputVideoSeconds*r.VideoReferenceRatioPerSecond,
		"reference_audios":    1 + float64(f.ReferenceAudios)*r.AudioReferenceRatioPerAudio,
	}, nil
}

func finite(v float64) bool            { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func inRange(v, min, max float64) bool { return finite(v) && v >= min && v <= max }
