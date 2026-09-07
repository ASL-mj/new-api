package types

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// Synthetic values only; these are arithmetic fixtures, not MiniMax prices.
func h3Rule() H3PricingRule {
	return H3PricingRule{DurationBaseSeconds: 5, ResolutionRatios: map[string]float64{"768P": 1, "2K": 2}, ImageReferenceRatioPerImage: 0.1, VideoReferenceRatioPerSecond: 0.2, AudioReferenceRatioPerAudio: 0.3}
}

func TestH3PricingSnapshot(t *testing.T) {
	s, err := NewH3PricingSnapshot("fixture-v1", 100, h3Rule(), H3UsageFacts{OutputSeconds: 5, Resolution: "768P"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		facts H3UsageFacts
		group float64
		want  int
		bad   bool
	}{
		{"base", H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}, 1, 100, false},
		{"fractional output", H3UsageFacts{OutputSeconds: 5.25, Resolution: "768P"}, 1, 105, false},
		{"reference factors", H3UsageFacts{OutputSeconds: 5, Resolution: "2K", InputImages: 2, InputVideoSeconds: 2.5, ReferenceAudios: 1}, 1, 468, false},
		{"round once", H3UsageFacts{OutputSeconds: 5.025, Resolution: "768P"}, 2, 201, false},
		{"free group", H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}, 0, 0, false},
		{"negative group", H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}, -1, 0, true},
		{"nan group", H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}, math.NaN(), 0, true},
		{"overflow group", H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}, math.MaxFloat64, 0, true},
		{"invalid duration", H3UsageFacts{OutputSeconds: 3, Resolution: "768P"}, 1, 0, true},
		{"nan duration", H3UsageFacts{OutputSeconds: math.NaN(), Resolution: "768P"}, 1, 0, true},
		{"invalid resolution", H3UsageFacts{OutputSeconds: 5, Resolution: "1080P"}, 1, 0, true},
		{"image bounds", H3UsageFacts{OutputSeconds: 5, Resolution: "768P", InputImages: 10}, 1, 0, true},
		{"video bounds", H3UsageFacts{OutputSeconds: 5, Resolution: "768P", InputVideoSeconds: 15.1}, 1, 0, true},
		{"audio bounds", H3UsageFacts{OutputSeconds: 5, Resolution: "768P", ReferenceAudios: 4}, 1, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Quota(tt.facts, tt.group)
			if (err != nil) != tt.bad || (!tt.bad && got != tt.want) {
				t.Fatalf("got %d, %v; want %d bad=%v", got, err, tt.want, tt.bad)
			}
		})
	}
}

func TestH3PricingSnapshotFreezeHashAndRoundtrip(t *testing.T) {
	r := h3Rule()
	facts := H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}
	s, err := NewH3PricingSnapshot("fixture-v1", 100, r, facts)
	if err != nil {
		t.Fatal(err)
	}
	r.ResolutionRatios["768P"] = 99
	if q, err := s.StandardQuota(facts); err != nil || q != 100 {
		t.Fatalf("not frozen: %d %v", q, err)
	}
	b, err := common.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored TaskPricingSnapshot
	if err := common.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	same, _ := NewH3PricingSnapshot("fixture-v1", 100, h3Rule(), facts)
	if same.RuleHash != s.RuleHash {
		t.Fatal("unstable hash")
	}
	changed, _ := NewH3PricingSnapshot("fixture-v2", 100, h3Rule(), facts)
	if changed.RuleHash == s.RuleHash {
		t.Fatal("version not hashed")
	}
	ratios, err := s.OtherRatios()
	if err != nil || ratios["duration"] != 1 {
		t.Fatal(ratios, err)
	}
	ratios["resolution"] = 20
	if q, _ := s.StandardQuota(facts); q != 100 {
		t.Fatal("display ratios mutate snapshot")
	}
	restored.H3Rule.ResolutionRatios["768P"] = 3
	if _, err := restored.StandardQuota(facts); err == nil {
		t.Fatal("tampering accepted")
	}
}

func TestH3PricingRuleValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*H3PricingRule)
	}{
		{"missing resolution", func(r *H3PricingRule) { delete(r.ResolutionRatios, "2K") }},
		{"extra resolution", func(r *H3PricingRule) { r.ResolutionRatios["480P"] = 1 }},
		{"zero duration base", func(r *H3PricingRule) { r.DurationBaseSeconds = 0 }},
		{"negative rate", func(r *H3PricingRule) { r.ImageReferenceRatioPerImage = -1 }},
		{"nan rate", func(r *H3PricingRule) { r.AudioReferenceRatioPerAudio = math.NaN() }},
		{"overflow price", func(r *H3PricingRule) { r.VideoReferenceRatioPerSecond = math.MaxFloat64 }},
		{"rounds to zero", func(r *H3PricingRule) { r.ResolutionRatios["768P"] = 0.000001 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := h3Rule()
			tt.mutate(&r)
			if _, err := NewH3PricingSnapshot("fixture", 100, r, H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}); err == nil {
				t.Fatal("accepted invalid rule")
			}
		})
	}
}

func TestH3UsageOverridesMerge(t *testing.T) {
	f := H3UsageFacts{OutputSeconds: 5, Resolution: "2K", InputImages: 2, InputVideoSeconds: 15, ReferenceAudios: 3}
	zero, images, bad := 0.0, 0, math.Inf(1)
	u := H3UsageOverrides{OutputSeconds: &bad, Resolution: "480P", InputImages: &images, InputVideoSeconds: &zero}
	got := u.Merge(f)
	if got.OutputSeconds != 5 || got.Resolution != "2K" || got.InputImages != 0 || got.InputVideoSeconds != 0 || got.ReferenceAudios != 3 {
		t.Fatalf("bad merge: %+v", got)
	}
}
