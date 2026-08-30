package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomMessagesExistInEveryBackendLocale(t *testing.T) {
	require.NoError(t, Init())
	keys := []string{
		MsgChannelUsageChannelQuotaReset,
		MsgChannelUsageKeyQuotaReset,
		MsgChannelUsageKeyQuotaUpdated,
		MsgChannelUsageInvalidRequest,
		MsgChannelUsageKeyQuotaNegative,
		MsgChannelUsageChannelIdsRequired,
		MsgChannelUsageMaxBatch,
		MsgChannelUsageChannelIdInvalid,
		MsgChannelUsageNotMultiKey,
		MsgChannelUsageKeyFingerprintInvalid,
		MsgChannelUsageRecordNotFound,
		MsgChannelQuotaLimitNegative,
		MsgChannelQuotaModeInvalid,
		MsgChannelQuotaSingleKeyUnsupported,
		MsgChannelQuotaResetRequired,
		MsgChannelKeyQuotaResetRequired,
		MsgChannelCodexKeyInvalidJSON,
		MsgChannelCodexKeyAccessTokenRequired,
		MsgChannelCodexKeyAccountIDRequired,
		MsgChannelCodexRefreshFailed,
		MsgChannelCodexRefreshSuccess,
		MsgChannelKeysChannelNotFound,
		MsgChannelKeysNotMultiKey,
		MsgChannelKeysDisableIndexRequired,
		MsgChannelKeysEnableIndexRequired,
		MsgChannelKeysDeleteIndexRequired,
		MsgChannelKeysIndexOutOfRange,
		MsgChannelKeysDisabled,
		MsgChannelKeysEnabled,
		MsgChannelKeysEnabledCount,
		MsgChannelKeysNoEnabledKeys,
		MsgChannelKeysDisabledCount,
		MsgChannelKeysLastKeyDeleteForbidden,
		MsgChannelKeysDeleted,
		MsgChannelKeysNoAutoDisabledKeys,
		MsgChannelKeysDeletedAutoCount,
		MsgChannelKeysResetAllCount,
		MsgChannelKeysUnsupportedAction,
		MsgMonitorGroupCreated,
		MsgMonitorGroupUpdated,
		MsgMonitorGroupDeleted,
		MsgMonitorGroupRunning,
		MsgMonitorGroupRunStarted,
		MsgMonitorGroupRunnerNotStarted,
		MsgMonitorGroupIdInvalid,
		MsgMonitorGroupNameInvalid,
		MsgMonitorGroupKeyInvalid,
		MsgMonitorGroupKeyImmutable,
		MsgMonitorGroupModelDescriptionInvalid,
		MsgMonitorGroupIntervalInvalid,
		MsgMonitorGroupTimeoutInvalid,
		MsgMonitorGroupDegradedInvalid,
		MsgMonitorGroupChannelRequired,
		MsgMonitorGroupChannelsMissing,
		MsgMonitorGroupNoProbeModels,
		MsgMonitorGroupNoCommonModels,
		MsgMonitorGroupModelsNotSupported,
		MsgMonitorGroupInvalidStatus,
		MsgMonitorGroupInvalidLimit,
		MsgMonitorGroupInvalidDays,
		MsgMonitorStatusDaysInvalid,
		MsgMonitorStatusNotFound,
		MsgOpsInvalidMetric,
		MsgOpsInvalidStartTimestamp,
		MsgOpsInvalidEndTimestamp,
		MsgOpsEndBeforeStart,
		MsgOpsRangeTooLarge,
		MsgOpsInvalidChannelType,
		MsgOpsInvalidChannelId,
		MsgOpsUnassignedChannel,
		MsgPerfMetricsHoursInvalid,
		MsgPerfMetricsModelRequired,
		MsgPerfMetricsSummaryFailed,
		MsgPerfMetricsQueryFailed,
		MsgSystemEventInvalidStartTimestamp,
		MsgSystemEventInvalidEndTimestamp,
		MsgSystemEventEndBeforeStart,
		MsgSystemEventMonitorProbeStarted,
		MsgSystemEventMonitorChannelFailed,
		MsgSystemEventMonitorSaveFailed,
		MsgSystemEventMonitorFinishFailed,
		MsgSystemEventMonitorNoTargets,
		MsgSystemEventMonitorCompleted,
		MsgSystemEventChannelAutoDisabled,
		MsgSystemEventChannelAutoRestored,
		MsgSystemEventKeyQuotaExhausted,
		MsgSystemEventChannelQuotaExhausted,
		MsgSystemEventOpsFlushRecovered,
		MsgSystemEventOpsFlushFailed,
		MsgSystemEventPerfFlushRecovered,
		MsgSystemEventPerfFlushFailed,
		MsgSystemEventUpstreamFinalFailure,
	}
	args := map[string]any{
		"Max": 200, "GroupId": 1, "GroupName": "Primary",
		"Operational": 1, "Degraded": 2, "Failed": 3,
		"ChannelId": 9, "KeyIndex": 2, "QuotaLimitUsed": 50,
		"QuotaLimit": 100, "ErrorCode": "upstream_error", "Count": 2,
	}

	for _, language := range SupportedLanguages() {
		for _, key := range keys {
			translated := Translate(language, key, args)
			assert.NotEqual(t, key, translated, "%s: %s", language, key)
			assert.NotEmpty(t, translated, "%s: %s", language, key)
		}
	}
}
