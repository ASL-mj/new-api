package model

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerfMetric struct {
	Id             int    `json:"id"`
	ModelName      string `json:"model_name" gorm:"size:128;not null;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
	Group          string `json:"group" gorm:"column:group;size:64;not null;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
	BucketTs       int64  `json:"bucket_ts" gorm:"bigint;not null;uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
	RequestCount   int64  `json:"request_count" gorm:"not null;default:0"`
	SuccessCount   int64  `json:"success_count" gorm:"not null;default:0"`
	TotalLatencyMs int64  `json:"total_latency_ms" gorm:"not null;default:0"`
	TtftSumMs      int64  `json:"ttft_sum_ms" gorm:"not null;default:0"`
	TtftCount      int64  `json:"ttft_count" gorm:"not null;default:0"`
	OutputTokens   int64  `json:"output_tokens" gorm:"not null;default:0"`
	GenerationMs   int64  `json:"generation_ms" gorm:"not null;default:0"`
}

func (PerfMetric) TableName() string {
	return "perf_metrics"
}

func UpsertPerfMetric(metric *PerfMetric) error {
	if metric == nil {
		return fmt.Errorf("perf metric is nil")
	}

	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "model_name"},
			{Name: "group"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":    gorm.Expr("request_count + ?", metric.RequestCount),
			"success_count":    gorm.Expr("success_count + ?", metric.SuccessCount),
			"total_latency_ms": gorm.Expr("total_latency_ms + ?", metric.TotalLatencyMs),
			"ttft_sum_ms":      gorm.Expr("ttft_sum_ms + ?", metric.TtftSumMs),
			"ttft_count":       gorm.Expr("ttft_count + ?", metric.TtftCount),
			"output_tokens":    gorm.Expr("output_tokens + ?", metric.OutputTokens),
			"generation_ms":    gorm.Expr("generation_ms + ?", metric.GenerationMs),
		}),
	}).Create(metric).Error
}

func GetPerfMetricsByRange(modelName string, group string, startBucketTs int64, endBucketTs int64) ([]*PerfMetric, error) {
	tx := DB.Model(&PerfMetric{}).Order("bucket_ts ASC").Order("id ASC")

	if modelName != "" {
		tx = tx.Where(clause.Eq{Column: clause.Column{Name: "model_name"}, Value: modelName})
	}
	if group != "" {
		tx = tx.Where(clause.Eq{Column: clause.Column{Name: "group"}, Value: group})
	}
	if startBucketTs != 0 {
		tx = tx.Where(clause.Gte{Column: clause.Column{Name: "bucket_ts"}, Value: startBucketTs})
	}
	if endBucketTs != 0 {
		tx = tx.Where(clause.Lte{Column: clause.Column{Name: "bucket_ts"}, Value: endBucketTs})
	}

	var metrics []*PerfMetric
	err := tx.Find(&metrics).Error
	return metrics, err
}

func DeleteExpiredPerfMetrics(expireBeforeBucketTs int64) (int64, error) {
	result := DB.Where(clause.Lt{Column: clause.Column{Name: "bucket_ts"}, Value: expireBeforeBucketTs}).Delete(&PerfMetric{})
	return result.RowsAffected, result.Error
}
