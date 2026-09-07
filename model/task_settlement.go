package model

import "encoding/json"

type TaskSettlementKind string

const (
	TaskSettlementKindSettle TaskSettlementKind = "settle"
	TaskSettlementKindRefund TaskSettlementKind = "refund"
)

type TaskSettlementState string

const (
	TaskSettlementStatePending  TaskSettlementState = "pending"
	TaskSettlementStateApplying TaskSettlementState = "applying"
	TaskSettlementStateApplied  TaskSettlementState = "applied"
	TaskSettlementStateFailed   TaskSettlementState = "failed"
)

// TaskSettlement is the durable, one-per-task record of a terminal billing
// decision, including the durable outbox for post-commit effects.
type TaskSettlement struct {
	ID                 int64               `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt          int64               `json:"created_at" gorm:"index"`
	UpdatedAt          int64               `json:"updated_at"`
	TaskID             int64               `json:"task_id" gorm:"not null;uniqueIndex:uq_task_settlements_task_id"`
	Kind               TaskSettlementKind  `json:"kind" gorm:"type:varchar(16);not null"`
	State              TaskSettlementState `json:"state" gorm:"type:varchar(16);not null;default:'pending';index"`
	PreConsumedQuota   int                 `json:"pre_consumed_quota"`
	ActualQuota        *int                `json:"actual_quota"`
	QuotaDelta         int                 `json:"quota_delta"`
	StandardQuotaDelta int                 `json:"standard_quota_delta"`
	ReasonClass        string              `json:"reason_class" gorm:"type:varchar(40)"`
	ReasonMessage      string              `json:"reason_message" gorm:"type:text"`
	PricingSnapshot    json.RawMessage     `json:"pricing_snapshot" gorm:"type:json"`
	AttemptCount       int                 `json:"attempt_count" gorm:"not null;default:0"`
	AppliedAt          int64               `json:"applied_at" gorm:"not null;default:0"`
	EffectsPayload     string              `json:"-" gorm:"type:text"`
	EffectsAppliedAt   int64               `json:"-" gorm:"not null;default:0;index"`
	EffectsAttempts    int                 `json:"-" gorm:"not null;default:0"`
	EffectsNextAt      int64               `json:"-" gorm:"not null;default:0;index"`
}
