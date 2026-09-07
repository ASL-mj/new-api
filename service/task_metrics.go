package service

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/model"
)

var taskPollCounters = struct {
	sync.Mutex
	byClass map[string]int64
}{byClass: make(map[string]int64)}

var taskZeroSettlementLogs atomic.Int64

func countTaskPollFailure(class string) {
	switch class {
	case "transport", "rate_limited", "server_transient", "malformed_response", "provider_terminal", "internal":
	default:
		class = "internal"
	}
	taskPollCounters.Lock()
	taskPollCounters.byClass[class]++
	taskPollCounters.Unlock()
}

type TaskMetricsSnapshot struct {
	PollFailureTotal      map[string]int64 `json:"task_poll_failure_total"`
	TerminalTotal         map[string]int64 `json:"task_terminal_total"`
	RefundTotal           int64            `json:"task_refund_total"`
	SettlementDeltaTotal  int64            `json:"task_settlement_delta_total"`
	SettlementDebitTotal  int64            `json:"task_settlement_debit_total"`
	SettlementCreditTotal int64            `json:"task_settlement_credit_total"`
	ZeroLogTotal          int64            `json:"log_quota_zero_after_settlement_total"`
	CounterScope          string           `json:"poll_and_log_counter_scope"`
}

// Terminal metrics use the unique durable ledger, so retries and process
// restarts do not count refunds twice. Poll/log observations are per process.
func GetTaskMetricsSnapshot(ctx context.Context) (*TaskMetricsSnapshot, error) {
	var totals struct {
		Settled  int64
		Refunded int64
		Delta    int64
		Debit    int64
		Credit   int64
	}
	err := model.DB.WithContext(ctx).Model(&model.TaskSettlement{}).
		Where("state = ?", model.TaskSettlementStateApplied).
		Select(`COALESCE(SUM(CASE WHEN kind = 'settle' THEN 1 ELSE 0 END), 0) AS settled,
		COALESCE(SUM(CASE WHEN kind = 'refund' THEN 1 ELSE 0 END), 0) AS refunded,
		COALESCE(SUM(quota_delta), 0) AS delta,
		COALESCE(SUM(CASE WHEN quota_delta > 0 THEN quota_delta ELSE 0 END), 0) AS debit,
		COALESCE(SUM(CASE WHEN quota_delta < 0 THEN -quota_delta ELSE 0 END), 0) AS credit`).Scan(&totals).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	taskPollCounters.Lock()
	for class, value := range taskPollCounters.byClass {
		counts[class] = value
	}
	taskPollCounters.Unlock()
	return &TaskMetricsSnapshot{
		PollFailureTotal: counts, TerminalTotal: map[string]int64{"SUCCESS": totals.Settled, "FAILURE": totals.Refunded},
		RefundTotal: totals.Refunded, SettlementDeltaTotal: totals.Delta, SettlementDebitTotal: totals.Debit,
		SettlementCreditTotal: totals.Credit, ZeroLogTotal: taskZeroSettlementLogs.Load(), CounterScope: "process_since_start",
	}, nil
}
