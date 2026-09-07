package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGPT6AstraBuiltinBilling(t *testing.T) {
	savedBilling := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if key == "billing_setting.billing_mode" || key == "billing_setting.billing_expr" {
			savedBilling[key] = value
		}
		return nil
	}))
	savedRatios := ratio_setting.ModelRatio2JSONString()
	savedPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedBilling))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": "{}",
		"billing_setting.billing_expr": "{}",
	}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	expression, ok := GetBuiltinBillingExpr("gpt-6-astra")
	require.True(t, ok)
	require.Equal(t, BillingModeTieredExpr, GetBillingMode("gpt-6-astra"))
	require.Equal(t, expression, GetBillingExprCopy()["gpt-6-astra"])

	for _, testCase := range []struct {
		name   string
		params billingexpr.TokenParams
		tier   string
		quota  int
	}{
		{
			name:   "standard",
			params: billingexpr.TokenParams{P: 1000, C: 100, Len: 1000},
			tier:   "standard",
			quota:  7500,
		},
		{
			name:   "standard context with cache read and write",
			params: billingexpr.TokenParams{P: 52000, C: 1000, Len: 272000, CR: 200000, CC: 20000},
			tier:   "standard",
			quota:  510000,
		},
		{
			name:   "long context applies to the whole request",
			params: billingexpr.TokenParams{P: 52001, C: 1000, Len: 272001, CR: 200000, CC: 20000},
			tier:   "long_context",
			quota:  1007510,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := billingexpr.ComputeTieredQuota(&billingexpr.BillingSnapshot{
				ExprString:   expression,
				ExprHash:     billingexpr.ExprHashString(expression),
				GroupRatio:   1,
				QuotaPerUnit: 500000,
			}, testCase.params)
			require.NoError(t, err)
			require.Equal(t, testCase.tier, result.MatchedTier)
			require.Equal(t, testCase.quota, result.ActualQuotaAfterGroup)
		})
	}

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-6-astra":0.1}`))
	require.Equal(t, BillingModeRatio, GetBillingMode("gpt-6-astra"))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"gpt-6-astra":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"gpt-6-astra":"tier(\"custom\", p * 7)"}`,
	}))
	require.Equal(t, "tier(\"custom\", p * 7)", GetBillingExprCopy()["gpt-6-astra"])
}
