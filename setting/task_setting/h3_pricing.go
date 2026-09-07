package task_setting

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

const H3PricingOption = "minimax_h3_pricing.rules"

type H3PricingConfig struct {
	Version   string `json:"version"`
	BaseQuota int    `json:"base_quota"`
	types.H3PricingRule
}

func init() {
	config.GlobalConfig.Register("minimax_h3_pricing", &struct {
		Rules string `json:"rules"`
	}{})
}

// ParseH3PricingRules rejects partial JSON, including omitted zero-cost reference
// rates. An explicit zero rate means that reference category has no surcharge.
func ParseH3PricingRules(raw string) (*H3PricingConfig, error) {
	var fields map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(raw, &fields); err != nil {
		return nil, fmt.Errorf("H3 pricing must be a JSON object")
	}
	keys := []string{"version", "base_quota", "duration_base_seconds", "resolution_ratios", "image_reference_ratio_per_image", "video_reference_ratio_per_second", "audio_reference_ratio_per_audio"}
	if len(fields) != len(keys) {
		return nil, fmt.Errorf("H3 pricing requires all seven defined fields and no unknown fields")
	}
	for _, key := range keys {
		v, ok := fields[key]
		if !ok || string(v) == "null" {
			return nil, fmt.Errorf("H3 pricing field %s is required", key)
		}
	}
	var cfg H3PricingConfig
	if err := common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid H3 pricing field type")
	}
	if _, err := types.NewH3PricingSnapshot(cfg.Version, cfg.BaseQuota, cfg.H3PricingRule, types.H3UsageFacts{OutputSeconds: 5, Resolution: "768P"}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ValidateH3PricingRules(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	} // Clearing configuration disables new submissions.
	_, err := ParseH3PricingRules(raw)
	return err
}

func UpdateH3PricingRules(raw string) error {
	if err := ValidateH3PricingRules(raw); err != nil {
		return err
	}
	return config.GlobalConfig.LoadFromDB(map[string]string{H3PricingOption: raw})
}

func GetH3PricingRules() (*H3PricingConfig, error) {
	// ExportAllConfigs holds the same lock used by option loading. Return parsed
	// copies, never a mutable pointer to the live configuration.
	return ParseH3PricingRules(config.GlobalConfig.ExportAllConfigs()[H3PricingOption])
}

func H3Enabled() bool { return common.GetEnvOrDefaultBool("MINIMAX_H3_ENABLED", false) }

func H3CallbackAllowed(raw string) bool {
	var allowed []string
	if common.UnmarshalJsonStr(common.GetEnvOrDefaultString("MINIMAX_H3_CALLBACK_URLS", "[]"), &allowed) != nil {
		return false
	}
	for _, value := range allowed {
		if value == raw {
			return true
		}
	}
	return false
}
