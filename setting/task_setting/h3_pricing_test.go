package task_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const fixtureRules = `{"version":"fixture-v1","base_quota":100,"duration_base_seconds":5,"resolution_ratios":{"768P":1,"2K":2},"image_reference_ratio_per_image":0,"video_reference_ratio_per_second":0,"audio_reference_ratio_per_audio":0}`

func TestH3PricingConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		bad       bool
	}{
		{"complete", fixtureRules, false}, {"empty", "", true}, {"null", "null", true}, {"array", "[]", true},
		{"missing", strings.Replace(fixtureRules, `,"audio_reference_ratio_per_audio":0`, "", 1), true},
		{"null field", strings.Replace(fixtureRules, `"base_quota":100`, `"base_quota":null`, 1), true},
		{"zero base", strings.Replace(fixtureRules, `"base_quota":100`, `"base_quota":0`, 1), true},
		{"unknown", strings.Replace(fixtureRules, `"version":`, `"extra":1,"version":`, 1), true},
		{"wrong type", strings.Replace(fixtureRules, `"duration_base_seconds":5`, `"duration_base_seconds":"5"`, 1), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseH3PricingRules(tt.raw)
			if (err != nil) != tt.bad {
				t.Fatal(err)
			}
		})
	}
}

func TestH3PricingStore(t *testing.T) {
	old := config.GlobalConfig.ExportAllConfigs()[H3PricingOption]
	t.Cleanup(func() { _ = config.GlobalConfig.LoadFromDB(map[string]string{H3PricingOption: old}) })
	if err := UpdateH3PricingRules(fixtureRules); err != nil {
		t.Fatal(err)
	}
	got, err := GetH3PricingRules()
	if err != nil {
		t.Fatal(err)
	}
	got.ResolutionRatios["2K"] = 99
	if next, _ := GetH3PricingRules(); next.ResolutionRatios["2K"] != 2 {
		t.Fatal("returned mutable configuration")
	}
	if err := UpdateH3PricingRules(`{}`); err == nil {
		t.Fatal("partial config accepted")
	}
	if next, _ := GetH3PricingRules(); next.Version != "fixture-v1" {
		t.Fatal("failed update changed configuration")
	}
	if err := UpdateH3PricingRules(""); err != nil {
		t.Fatal(err)
	}
	if _, err := GetH3PricingRules(); err == nil {
		t.Fatal("empty config accepted")
	}
	t.Setenv("MINIMAX_H3_ENABLED", "false")
	if H3Enabled() {
		t.Fatal("gate enabled")
	}
	t.Setenv("MINIMAX_H3_CALLBACK_URLS", `["https://callback.example/hook"]`)
	if !H3CallbackAllowed("https://callback.example/hook") || H3CallbackAllowed("https://callback.example/hook?other=1") {
		t.Fatal("callback not exact matched")
	}
	if _, err := common.Marshal(got); err != nil {
		t.Fatal(err)
	}
}
