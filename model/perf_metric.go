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

type PerfMetricSummaryBucket struct {
	ModelName      string `json:"model_name"`
	BucketTs       int64  `json:"bucket_ts"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
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

func GetPerfMetricSummaryBucketsByRange(startBucketTs int64, endBucketTs int64, groups []string) ([]PerfMetricSummaryBucket, error) {
	if groups != nil && len(groups) == 0 {
		return []PerfMetricSummaryBucket{}, nil
	}

	tx := DB.Model(&PerfMetric{}).Select([]string{
		"model_name",
		"bucket_ts",
		"SUM(request_count) AS request_count",
		"SUM(success_count) AS success_count",
		"SUM(total_latency_ms) AS total_latency_ms",
		"SUM(output_tokens) AS output_tokens",
		"SUM(generation_ms) AS generation_ms",
	})

	if startBucketTs != 0 {
		tx = tx.Where(clause.Gte{Column: clause.Column{Name: "bucket_ts"}, Value: startBucketTs})
	}
	if endBucketTs != 0 {
		tx = tx.Where(clause.Lte{Column: clause.Column{Name: "bucket_ts"}, Value: endBucketTs})
	}
	if groups != nil {
		tx = tx.Where(clause.Eq{Column: clause.Column{Name: "group"}, Value: groups})
	}

	var summaries []PerfMetricSummaryBucket
	err := tx.Clauses(
		clause.GroupBy{
			Columns: []clause.Column{
				{Name: "model_name"},
				{Name: "bucket_ts"},
			},
			Having: []clause.Expression{
				clause.Gt{Column: gorm.Expr("SUM(?)", clause.Column{Name: "request_count"}), Value: 0},
			},
		},
		clause.OrderBy{
			Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "model_name"}},
				{Column: clause.Column{Name: "bucket_ts"}},
			},
		},
	).Scan(&summaries).Error
	return summaries, err
}

func DeleteExpiredPerfMetrics(expireBeforeBucketTs int64) (int64, error) {
	result := DB.Where(clause.Lt{Column: clause.Column{Name: "bucket_ts"}, Value: expireBeforeBucketTs}).Delete(&PerfMetric{})
	return result.RowsAffected, result.Error
}
