package model

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OpsMetricBucket struct {
	Id                   int    `json:"id"`
	BucketTs             int64  `json:"bucket_ts" gorm:"bigint;not null;uniqueIndex:idx_ops_dimension_bucket,priority:1;index"`
	ModelName            string `json:"model_name" gorm:"size:128;not null;uniqueIndex:idx_ops_dimension_bucket,priority:2"`
	Group                string `json:"group" gorm:"column:group;size:64;not null;uniqueIndex:idx_ops_dimension_bucket,priority:3"`
	ChannelId            int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_ops_dimension_bucket,priority:4;index"`
	ChannelType          int    `json:"channel_type" gorm:"not null;uniqueIndex:idx_ops_dimension_bucket,priority:5;index"`
	RequestCount         int64  `json:"request_count" gorm:"not null;default:0"`
	SuccessCount         int64  `json:"success_count" gorm:"not null;default:0"`
	BusinessLimitedCount int64  `json:"business_limited_count" gorm:"not null;default:0"`
	UpstreamErrorCount   int64  `json:"upstream_error_count" gorm:"not null;default:0"`
	Upstream429Count     int64  `json:"upstream_429_count" gorm:"column:upstream_429_count;not null;default:0"`
	Upstream529Count     int64  `json:"upstream_529_count" gorm:"column:upstream_529_count;not null;default:0"`
	TotalLatencyMs       int64  `json:"total_latency_ms" gorm:"not null;default:0"`
	TtftSumMs            int64  `json:"ttft_sum_ms" gorm:"not null;default:0"`
	TtftCount            int64  `json:"ttft_count" gorm:"not null;default:0"`
	OutputTokens         int64  `json:"output_tokens" gorm:"not null;default:0"`
	GenerationMs         int64  `json:"generation_ms" gorm:"not null;default:0"`
}

type OpsMetricHistogram struct {
	Id           int    `json:"id"`
	BucketTs     int64  `json:"bucket_ts" gorm:"bigint;not null;uniqueIndex:idx_ops_histogram,priority:1;index"`
	Metric       string `json:"metric" gorm:"size:16;not null;uniqueIndex:idx_ops_histogram,priority:2"`
	Group        string `json:"group" gorm:"column:group;size:64;not null;uniqueIndex:idx_ops_histogram,priority:3"`
	ChannelId    int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_ops_histogram,priority:4"`
	ChannelType  int    `json:"channel_type" gorm:"not null;uniqueIndex:idx_ops_histogram,priority:5"`
	UpperBoundMs int64  `json:"upper_bound_ms" gorm:"not null;uniqueIndex:idx_ops_histogram,priority:6"`
	Count        int64  `json:"count" gorm:"not null;default:0"`
}

type OpsMetricQuery struct {
	StartBucketTs int64
	EndBucketTs   int64
	Group         string
	ChannelType   *int
	ChannelIDs    []int
	ModelName     string
}

func (OpsMetricBucket) TableName() string { return "ops_metric_buckets" }

func (OpsMetricHistogram) TableName() string { return "ops_metric_histograms" }

// UpsertOpsMetrics persists one bucket and all of its histograms atomically.
// Keeping the writes together prevents a failed histogram write from making a
// retry duplicate the corresponding bucket counters.
func UpsertOpsMetrics(bucket OpsMetricBucket, histograms []OpsMetricHistogram) error {
	if err := ValidateOpsMetricBucket(&bucket); err != nil {
		return err
	}
	for index := range histograms {
		if err := ValidateOpsMetricHistogram(&histograms[index]); err != nil {
			return err
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := upsertOpsMetricBucket(tx, &bucket); err != nil {
			return err
		}
		for index := range histograms {
			if err := upsertOpsMetricHistogram(tx, &histograms[index]); err != nil {
				return err
			}
		}
		return nil
	})
}

func UpsertOpsMetricBuckets(metrics []OpsMetricBucket) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for index := range metrics {
			if err := ValidateOpsMetricBucket(&metrics[index]); err != nil {
				return err
			}
			if err := upsertOpsMetricBucket(tx, &metrics[index]); err != nil {
				return err
			}
		}
		return nil
	})
}

func UpsertOpsMetricHistograms(metrics []OpsMetricHistogram) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for index := range metrics {
			if err := ValidateOpsMetricHistogram(&metrics[index]); err != nil {
				return err
			}
			if err := upsertOpsMetricHistogram(tx, &metrics[index]); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertOpsMetricBucket(tx *gorm.DB, metric *OpsMetricBucket) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bucket_ts"}, {Name: "model_name"}, {Name: "group"}, {Name: "channel_id"}, {Name: "channel_type"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":          gorm.Expr("request_count + ?", metric.RequestCount),
			"success_count":          gorm.Expr("success_count + ?", metric.SuccessCount),
			"business_limited_count": gorm.Expr("business_limited_count + ?", metric.BusinessLimitedCount),
			"upstream_error_count":   gorm.Expr("upstream_error_count + ?", metric.UpstreamErrorCount),
			"upstream_429_count":     gorm.Expr("upstream_429_count + ?", metric.Upstream429Count),
			"upstream_529_count":     gorm.Expr("upstream_529_count + ?", metric.Upstream529Count),
			"total_latency_ms":       gorm.Expr("total_latency_ms + ?", metric.TotalLatencyMs),
			"ttft_sum_ms":            gorm.Expr("ttft_sum_ms + ?", metric.TtftSumMs),
			"ttft_count":             gorm.Expr("ttft_count + ?", metric.TtftCount),
			"output_tokens":          gorm.Expr("output_tokens + ?", metric.OutputTokens),
			"generation_ms":          gorm.Expr("generation_ms + ?", metric.GenerationMs),
		}),
	}).Create(metric).Error
}

func upsertOpsMetricHistogram(tx *gorm.DB, metric *OpsMetricHistogram) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bucket_ts"}, {Name: "metric"}, {Name: "group"}, {Name: "channel_id"}, {Name: "channel_type"}, {Name: "upper_bound_ms"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"count": gorm.Expr("count + ?", metric.Count),
		}),
	}).Create(metric).Error
}

func GetOpsMetricBucketsByRange(startBucketTs, endBucketTs int64, channelIds []int) ([]OpsMetricBucket, error) {
	return GetOpsMetricBuckets(OpsMetricQuery{
		StartBucketTs: startBucketTs,
		EndBucketTs:   endBucketTs,
		ChannelIDs:    channelIds,
	})
}

func GetOpsMetricBuckets(query OpsMetricQuery) ([]OpsMetricBucket, error) {
	tx := DB.Model(&OpsMetricBucket{}).Order("bucket_ts ASC, id ASC")
	if query.StartBucketTs != 0 {
		tx = tx.Where("bucket_ts >= ?", query.StartBucketTs)
	}
	if query.EndBucketTs != 0 {
		tx = tx.Where("bucket_ts <= ?", query.EndBucketTs)
	}
	if query.Group != "" {
		groupColumn := commonGroupCol
		if groupColumn == "" {
			groupColumn = "`group`"
		}
		tx = tx.Where(groupColumn+" = ?", query.Group)
	}
	if query.ChannelType != nil {
		tx = tx.Where("channel_type = ?", *query.ChannelType)
	}
	if len(query.ChannelIDs) > 0 {
		tx = tx.Where("channel_id IN (?)", query.ChannelIDs)
	}
	if query.ModelName != "" {
		tx = tx.Where("model_name = ?", query.ModelName)
	}
	var rows []OpsMetricBucket
	return rows, tx.Find(&rows).Error
}

func GetOpsMetricHistograms(query OpsMetricQuery) ([]OpsMetricHistogram, error) {
	tx := DB.Model(&OpsMetricHistogram{}).Order("bucket_ts ASC, id ASC")
	if query.StartBucketTs != 0 {
		tx = tx.Where("bucket_ts >= ?", query.StartBucketTs)
	}
	if query.EndBucketTs != 0 {
		tx = tx.Where("bucket_ts <= ?", query.EndBucketTs)
	}
	if query.Group != "" {
		groupColumn := commonGroupCol
		if groupColumn == "" {
			groupColumn = "`group`"
		}
		tx = tx.Where(groupColumn+" = ?", query.Group)
	}
	if query.ChannelType != nil {
		tx = tx.Where("channel_type = ?", *query.ChannelType)
	}
	if len(query.ChannelIDs) > 0 {
		tx = tx.Where("channel_id IN (?)", query.ChannelIDs)
	}
	var rows []OpsMetricHistogram
	return rows, tx.Find(&rows).Error
}

func DeleteExpiredOpsMetrics(expireBefore int64) (int64, error) {
	result := DB.Where("bucket_ts < ?", expireBefore).Delete(&OpsMetricBucket{})
	if result.Error != nil {
		return result.RowsAffected, result.Error
	}
	histogramResult := DB.Where("bucket_ts < ?", expireBefore).Delete(&OpsMetricHistogram{})
	return result.RowsAffected + histogramResult.RowsAffected, histogramResult.Error
}

func ValidateOpsMetricBucket(metric *OpsMetricBucket) error {
	if metric == nil {
		return fmt.Errorf("ops metric is nil")
	}
	if metric.ModelName == "" {
		return fmt.Errorf("ops metric model name is empty")
	}
	return nil
}

func ValidateOpsMetricHistogram(metric *OpsMetricHistogram) error {
	if metric == nil {
		return fmt.Errorf("ops metric histogram is nil")
	}
	if metric.Metric != "duration" && metric.Metric != "ttft" {
		return fmt.Errorf("invalid ops metric histogram metric: %s", metric.Metric)
	}
	if metric.Count < 0 {
		return fmt.Errorf("ops metric histogram count must not be negative")
	}
	return nil
}
