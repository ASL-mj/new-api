package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type relayMetricSample struct {
	info         *relaycommon.RelayInfo
	success      bool
	outputTokens int64
}

type quotaMetricsTestBilling struct {
	err         error
	settleCalls int
}

func (b *quotaMetricsTestBilling) Settle(int) error {
	b.settleCalls++
	return b.err
}

func (b *quotaMetricsTestBilling) Refund(*gin.Context)      {}
func (b *quotaMetricsTestBilling) NeedsRefund() bool        { return false }
func (b *quotaMetricsTestBilling) GetPreConsumedQuota() int { return 0 }
func (b *quotaMetricsTestBilling) Reserve(int) error        { return nil }

func captureQuotaMetrics(t *testing.T) chan relayMetricSample {
	t.Helper()

	originalRecord := recordRelayQuotaSample
	originalDB := model.DB
	originalLogDB := model.LOG_DB

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := testDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, testDB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Log{}))
	model.DB = testDB
	model.LOG_DB = testDB
	recorded := make(chan relayMetricSample, 1)

	recordRelayQuotaSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		recorded <- relayMetricSample{info: info, success: success, outputTokens: outputTokens}
	}
	t.Cleanup(func() {
		recordRelayQuotaSample = originalRecord
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		_ = sqlDB.Close()
	})

	return recorded
}

func newQuotaMetricsRelayInfo(billing relaycommon.BillingSettler) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime:       time.Now().Add(-time.Second),
		OriginModelName: "gpt-4.1",
		UsingGroup:      "default",
		IsPlayground:    true,
		Billing:         billing,
		ChannelMeta:     &relaycommon.ChannelMeta{},
		PriceData: types.PriceData{
			ModelRatio:      0,
			CompletionRatio: 1,
			CacheRatio:      1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
	}
}

func newQuotaMetricsContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func requireRecordedRelayMetric(t *testing.T, recorded <-chan relayMetricSample, info *relaycommon.RelayInfo, outputTokens int64) {
	t.Helper()

	select {
	case sample := <-recorded:
		require.Same(t, info, sample.info)
		require.True(t, sample.success)
		require.EqualValues(t, outputTokens, sample.outputTokens)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for settled relay metric")
	}

	select {
	case sample := <-recorded:
		t.Fatalf("unexpected duplicate relay metric: %+v", sample)
	case <-time.After(50 * time.Millisecond):
	}
}

func requireNoRecordedRelayMetric(t *testing.T, recorded <-chan relayMetricSample) {
	t.Helper()

	select {
	case sample := <-recorded:
		t.Fatalf("unexpected relay metric after failed settlement: %+v", sample)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPostTextConsumeQuotaRecordsOnlySettledMetrics(t *testing.T) {
	t.Run("successful settlement records completion tokens once", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{}
		info := newQuotaMetricsRelayInfo(billing)

		PostTextConsumeQuota(newQuotaMetricsContext(), info, &dto.Usage{PromptTokens: 1, CompletionTokens: 37}, nil)

		require.Equal(t, 1, billing.settleCalls)
		requireRecordedRelayMetric(t, recorded, info, 37)
	})

	t.Run("failed settlement does not record a success metric", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{err: errors.New("settlement failed")}

		PostTextConsumeQuota(newQuotaMetricsContext(), newQuotaMetricsRelayInfo(billing), &dto.Usage{PromptTokens: 1, CompletionTokens: 37}, nil)

		require.Equal(t, 1, billing.settleCalls)
		requireNoRecordedRelayMetric(t, recorded)
	})
}

func TestPostAudioConsumeQuotaRecordsOnlySettledMetrics(t *testing.T) {
	t.Run("successful settlement records completion tokens once", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{}
		info := newQuotaMetricsRelayInfo(billing)

		PostAudioConsumeQuota(newQuotaMetricsContext(), info, &dto.Usage{PromptTokens: 1, CompletionTokens: 19, TotalTokens: 20}, "")

		require.Equal(t, 1, billing.settleCalls)
		requireRecordedRelayMetric(t, recorded, info, 19)
	})

	t.Run("failed settlement does not record a success metric", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{err: errors.New("settlement failed")}

		PostAudioConsumeQuota(newQuotaMetricsContext(), newQuotaMetricsRelayInfo(billing), &dto.Usage{PromptTokens: 1, CompletionTokens: 19, TotalTokens: 20}, "")

		require.Equal(t, 1, billing.settleCalls)
		requireNoRecordedRelayMetric(t, recorded)
	})
}

func TestPostWssConsumeQuotaRecordsOnlySettledMetrics(t *testing.T) {
	t.Run("successful settlement records output tokens once", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{}
		info := newQuotaMetricsRelayInfo(billing)

		PostWssConsumeQuota(newQuotaMetricsContext(), info, "gpt-realtime", &dto.RealtimeUsage{InputTokens: 1, OutputTokens: 23, TotalTokens: 24}, "")

		require.Equal(t, 1, billing.settleCalls)
		requireRecordedRelayMetric(t, recorded, info, 23)
	})

	t.Run("failed settlement does not record a success metric", func(t *testing.T) {
		recorded := captureQuotaMetrics(t)
		billing := &quotaMetricsTestBilling{err: errors.New("settlement failed")}

		PostWssConsumeQuota(newQuotaMetricsContext(), newQuotaMetricsRelayInfo(billing), "gpt-realtime", &dto.RealtimeUsage{InputTokens: 1, OutputTokens: 23, TotalTokens: 24}, "")

		require.Equal(t, 1, billing.settleCalls)
		requireNoRecordedRelayMetric(t, recorded)
	})
}

func TestRecordSuccessfulRelayQuotaAlsoRecordsOpsMetrics(t *testing.T) {
	originalPerformance := recordRelayQuotaSample
	originalOps := recordRelayOpsSuccess
	t.Cleanup(func() {
		recordRelayQuotaSample = originalPerformance
		recordRelayOpsSuccess = originalOps
	})

	performanceRecorded := make(chan relayMetricSample, 1)
	opsRecorded := make(chan relayMetricSample, 1)
	recordRelayQuotaSample = func(info *relaycommon.RelayInfo, success bool, outputTokens int64) {
		performanceRecorded <- relayMetricSample{info: info, success: success, outputTokens: outputTokens}
	}
	recordRelayOpsSuccess = func(info *relaycommon.RelayInfo, outputTokens int64) {
		opsRecorded <- relayMetricSample{info: info, success: true, outputTokens: outputTokens}
	}

	info := newQuotaMetricsRelayInfo(&quotaMetricsTestBilling{})
	recordSuccessfulRelayQuota(info, 21)

	for name, recorded := range map[string]<-chan relayMetricSample{
		"performance": performanceRecorded,
		"ops":         opsRecorded,
	} {
		select {
		case sample := <-recorded:
			require.Same(t, info, sample.info, name)
			require.True(t, sample.success, name)
			require.EqualValues(t, 21, sample.outputTokens, name)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s metric", name)
		}
	}
}
