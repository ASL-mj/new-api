# NewAPI v0.13.2 Model Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在官方 `v0.13.2` 基线上移植最新版的模型性能采集与模型详情展示能力，并通过个人分支持续维护、提交到 `ASL-mj/new-api`。

**Architecture:** Relay 请求完成后，将成功/失败、总延迟、TTFT、输出 Token 和生成耗时写入进程内聚合桶，并在启用 Redis 时同步累加 Redis 计数器；完成的时间桶定期落入独立的 `perf_metrics` 表。模型广场通过只读聚合 API 获取最近 24 小时数据，旧版 React/Semi UI 详情侧栏展示 TPS、平均延迟、成功率、分组性能、延迟趋势和可用率。

**Tech Stack:** Go 1.22+、Gin、GORM、MySQL/SQLite/PostgreSQL、Redis、React 18、Vite、Semi UI、VChart、Bun。

---

## 1. Current Baseline

- Repository: `/Users/Zhuanz1/Desktop/privateProjects/ssh/workspace/new-api-v0.13.2`
- Branch: `dev/skye-v0.13.2-performance`
- Base tag: `v0.13.2`
- Base commit: `bee339d279ccecbf8c8a89e14ddbbd902f78bd5d`
- Current remote: official `git@github.com:QuantumNous/new-api.git`
- GitHub identity: `ASL-mj`
- Personal repository status: `ASL-mj/new-api` does not exist yet
- Existing local-only change: `web/vite.config.js` supports `VITE_PROXY_TARGET`

The implementation must stay on `dev/skye-v0.13.2-performance`. Do not merge the latest `main` branch into it; port only the model-performance feature and the minimum dependencies required by that feature.

## 2. Scope

### Included

- Performance collection switch and bucket configuration.
- Real Relay success and failure samples.
- Per-model and per-group aggregation.
- Average TTFT, average total latency, TPS and success rate.
- Recent latency and availability series.
- Read-only model performance APIs.
- Model-detail performance UI in the existing Semi UI side sheet.
- MySQL, SQLite and PostgreSQL-compatible schema and queries.
- Focused backend tests, frontend static checks and an end-to-end smoke test.

### Excluded

- Historical `logs` backfill. Only requests processed after this feature is deployed are collected.
- Channel test requests and admin-only test traffic. Only real `/v1/...` Relay traffic is counted.
- Latest-version authentication/session changes.
- Latest frontend framework, router, Zustand or TanStack Query migration.
- Dashboard-wide performance cards and model-card ranking in the first release.
- User-, token-, channel- or request-level data in public responses.

## 3. Data Contract

Create one row per `model_name + group + bucket_ts`:

```go
type PerfMetric struct {
    Id             int
    ModelName      string
    Group          string
    BucketTs       int64
    RequestCount   int64
    SuccessCount   int64
    TotalLatencyMs int64
    TtftSumMs      int64
    TtftCount      int64
    OutputTokens   int64
    GenerationMs   int64
}
```

Calculated fields:

```text
average latency = total_latency_ms / request_count
average TTFT    = ttft_sum_ms / ttft_count
success rate    = success_count / request_count * 100
TPS             = output_tokens / (generation_ms / 1000)
```

Public API response:

```json
{
  "success": true,
  "data": {
    "model_name": "gpt-5.4",
    "series_schema": "dbcd0a3c01b55203",
    "groups": [
      {
        "group": "default",
        "avg_ttft_ms": 820,
        "avg_latency_ms": 5100,
        "success_rate": 98.75,
        "avg_tps": 42.36,
        "series": [
          {
            "ts": 1785391200,
            "avg_ttft_ms": 790,
            "avg_latency_ms": 4900,
            "success_rate": 100,
            "avg_tps": 44.12
          }
        ]
      }
    ]
  }
}
```

## 4. File Map

### Backend files to create

- `model/perf_metric.go`: GORM schema, upsert, query and retention deletion.
- `model/perf_metric_test.go`: cross-dialect-safe SQLite aggregation tests.
- `setting/perf_metrics_setting/config.go`: collection configuration and defaults.
- `setting/perf_metrics_setting/config_test.go`: bucket and minimum flush interval tests.
- `pkg/perf_metrics/types.go`: samples, counters and API result structures.
- `pkg/perf_metrics/metrics.go`: recording, aggregation, query and Redis active-bucket support.
- `pkg/perf_metrics/flush.go`: periodic persistence and retention cleanup.
- `pkg/perf_metrics/metrics_test.go`: formula, bucketing, empty-data and group-series tests.
- `controller/perf_metrics.go`: detail and summary handlers.
- `controller/perf_metrics_test.go`: request validation and active-group filtering tests.

### Backend files to modify

- `model/main.go`: migrate `PerfMetric` in normal and fast migration paths.
- `main.go`: initialize the performance collector after database/Redis setup.
- `controller/relay.go`: record failed Relay requests once final retry handling ends.
- `service/quota.go`: record successful non-text/audio Relay usage.
- `service/text_quota.go`: record successful text Relay usage.
- `router/api-router.go`: expose `/api/perf-metrics` and `/api/perf-metrics/summary`.

### Frontend files to create

- `web/src/hooks/model-pricing/useModelPerformance.js`: request lifecycle, cancellation and data normalization.
- `web/src/components/table/model-pricing/modal/components/ModelPerformancePanel.jsx`: loading/error/empty/success state owner.
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceStats.jsx`: TPS, latency and success-rate summary.
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceGroupTable.jsx`: per-group metrics table.
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceCharts.jsx`: VChart latency and availability trends.
- `web/src/helpers/performance.js`: formatting and success-rate color grading.

### Frontend files to modify

- `web/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx`: append performance section without replacing existing model metadata/pricing.
- `web/src/pages/Setting/Operation/SettingsMonitoring.jsx`: add collection switch, flush interval, bucket size and retention fields.
- `web/src/components/settings/OperationSetting.jsx`: define defaults and correctly parse option types.
- `web/src/i18n/locales/zh-CN.json`
- `web/src/i18n/locales/zh-TW.json`
- `web/src/i18n/locales/en.json`
- `web/src/i18n/locales/ja.json`
- `web/src/i18n/locales/fr.json`
- `web/src/i18n/locales/ru.json`
- `web/src/i18n/locales/vi.json`

## 5. Implementation Tasks

### Task 0: Personal Repository and Branch Hygiene

**Files:**
- Existing local branch only; no source changes.

- [ ] Create the GitHub fork/repository `ASL-mj/new-api` from `QuantumNous/new-api`.
- [ ] Rename the official remote so accidental pushes cannot target upstream:

```bash
git remote rename origin upstream
git remote add origin git@github.com:ASL-mj/new-api.git
git remote -v
```

Expected:

```text
origin   git@github.com:ASL-mj/new-api.git
upstream git@github.com:QuantumNous/new-api.git
```

- [ ] Confirm the feature branch is based on `v0.13.2`:

```bash
git switch dev/skye-v0.13.2-performance
test "$(git merge-base HEAD 'v0.13.2^{}')" = "$(git rev-parse 'v0.13.2^{}')"
```

- [ ] Commit the existing configurable Vite proxy separately so it cannot be mixed with feature code:

```bash
git add web/vite.config.js
git commit -m "chore(dev): allow configurable Vite proxy target"
```

- [ ] Commit this implementation plan separately:

```bash
git add docs/superpowers/implementation/2026-07-30-v0132-model-performance.md
git commit -m "docs: add v0.13.2 model performance plan"
```

- [ ] Publish the baseline feature branch to the personal repository:

```bash
git push -u origin dev/skye-v0.13.2-performance
```

### Task 1: Schema and Configuration

**Files:**
- Create: `model/perf_metric.go`
- Create: `model/perf_metric_test.go`
- Create: `setting/perf_metrics_setting/config.go`
- Create: `setting/perf_metrics_setting/config_test.go`
- Modify: `model/main.go`

- [ ] Write failing tests proving that two inserts with the same model/group/bucket are accumulated instead of duplicated.
- [ ] Test that another group or bucket produces a separate row.
- [ ] Test summary filtering by time range and group.
- [ ] Test configuration defaults:

```text
enabled=true
flush_interval=5
bucket_time=hour
retention_days=0
```

- [ ] Implement `PerfMetric` with the composite unique index `idx_perf_model_group_bucket` and the time index `idx_perf_bucket_ts`.
- [ ] Use GORM `clause.OnConflict` and `gorm.Expr` for atomic increments; do not add dialect-specific SQL.
- [ ] Register `perf_metrics_setting` with the existing `setting/config.GlobalConfig` manager.
- [ ] Add `&PerfMetric{}` to both `migrateDB()` and `migrateDBFast()`.
- [ ] Run:

```bash
go test ./model ./setting/perf_metrics_setting
```

Expected: PASS.

- [ ] Commit:

```bash
git add model/perf_metric.go model/perf_metric_test.go model/main.go \
  setting/perf_metrics_setting/config.go \
  setting/perf_metrics_setting/config_test.go
git commit -m "feat(performance): add model metric storage and settings"
```

### Task 2: Collection and Aggregation Core

**Files:**
- Create: `pkg/perf_metrics/types.go`
- Create: `pkg/perf_metrics/metrics.go`
- Create: `pkg/perf_metrics/flush.go`
- Create: `pkg/perf_metrics/metrics_test.go`

- [ ] Write failing tests for:

```text
2 requests, 1 success, 3000ms total latency => 1500ms average, 50% success
200 output tokens, 4000ms generation       => 50 TPS
TTFT average uses only samples with HasTtft=true
empty model names are ignored
empty groups become "default"
hours <= 0 becomes 24; hours > 720 becomes 720
```

- [ ] Implement lock-free per-bucket counters with `sync.Map` and `atomic.Int64`.
- [ ] Keep one bucket key as `model + group + aligned timestamp`.
- [ ] Write active-bucket counters to Redis keys using this exact format:

```text
perf:<model>:<group>:<bucket_ts>
```

- [ ] Apply a one-hour Redis TTL and a one-second Redis operation timeout.
- [ ] Query persisted completed buckets plus the current process hot bucket without double-counting.
- [ ] Flush only completed buckets; failed database writes must restore drained counters.
- [ ] Delete expired persisted buckets only when `retention_days > 0`.
- [ ] Run:

```bash
go test ./pkg/perf_metrics
go test -race ./pkg/perf_metrics
```

Expected: PASS with no race reports.

- [ ] Commit:

```bash
git add pkg/perf_metrics
git commit -m "feat(performance): collect and aggregate relay metrics"
```

### Task 3: Wire Real Relay Requests

**Files:**
- Modify: `main.go`
- Modify: `controller/relay.go`
- Modify: `service/quota.go`
- Modify: `service/text_quota.go`

- [ ] Call `perfmetrics.Init()` after database and Redis initialization.
- [ ] Record one failed sample only after retries are exhausted in `controller/relay.go`.
- [ ] Record successful text requests in `PostTextConsumeQuota` with `summary.CompletionTokens`.
- [ ] Record successful audio/other requests in the corresponding post-consume path with completion tokens.
- [ ] Use the existing goroutine pool so metrics never delay the Relay response.
- [ ] Confirm the sample uses:

```text
model         = relayInfo.OriginModelName
group         = relayInfo.UsingGroup
latency       = now - relayInfo.StartTime
TTFT          = relayInfo.FirstResponseTime - relayInfo.StartTime for streamed responses
generation_ms = now - first response for streamed responses, otherwise total latency
```

- [ ] Run focused existing quota and relay tests:

```bash
go test ./service ./controller
```

Expected: PASS.

- [ ] Commit:

```bash
git add main.go controller/relay.go service/quota.go service/text_quota.go
git commit -m "feat(performance): record relay success and failure samples"
```

### Task 4: Read-only Performance API

**Files:**
- Create: `controller/perf_metrics.go`
- Create: `controller/perf_metrics_test.go`
- Modify: `router/api-router.go`

- [ ] Write failing handler tests for missing model, default 24-hour window, 30-day maximum, empty data and active-group filtering.
- [ ] Add endpoints:

```text
GET /api/perf-metrics?model=<model>&hours=24&group=<optional>
GET /api/perf-metrics/summary?hours=24
```

- [ ] Use `middleware.TryUserAuth()` to match the public/private behavior already used by `/api/pricing` in `v0.13.2`.
- [ ] Return aggregate model/group data only. Never expose user ID, token ID, channel ID, request ID or prompt content.
- [ ] Filter groups against `ratio_setting.GetGroupRatioCopy()` and preserve the special `auto` group.
- [ ] Run:

```bash
go test ./controller ./router
```

Expected: PASS.

- [ ] Commit:

```bash
git add controller/perf_metrics.go controller/perf_metrics_test.go router/api-router.go
git commit -m "feat(performance): expose model metric APIs"
```

### Task 5: Admin Collection Settings

**Files:**
- Modify: `web/src/pages/Setting/Operation/SettingsMonitoring.jsx`
- Modify: `web/src/components/settings/OperationSetting.jsx`
- Modify: frontend locale JSON files.

- [ ] Add these values to `OperationSetting` defaults:

```js
'perf_metrics_setting.enabled': true,
'perf_metrics_setting.flush_interval': 5,
'perf_metrics_setting.bucket_time': 'hour',
'perf_metrics_setting.retention_days': 0,
```

- [ ] Ensure boolean fields pass through `toBoolean`, numeric fields become numbers for Semi inputs, and PUT payload values remain strings as required by `/api/option/`.
- [ ] Add controls to the existing monitoring card:

```text
Enable model performance metrics: Switch
Flush interval: InputNumber, minimum 1
Aggregation bucket: Select(minute, 5min, hour)
Retention days: InputNumber, minimum 0; 0 means unlimited
```

- [ ] Disable interval/bucket/retention controls when collection is off.
- [ ] Preserve all existing channel-monitoring fields and save behavior.
- [ ] Run:

```bash
cd web
bun run i18n:lint
bun run eslint -- src/pages/Setting/Operation/SettingsMonitoring.jsx \
  src/components/settings/OperationSetting.jsx
bun run build
```

Expected: all commands succeed.

- [ ] Commit:

```bash
git add web/src/pages/Setting/Operation/SettingsMonitoring.jsx \
  web/src/components/settings/OperationSetting.jsx web/src/i18n/locales
git commit -m "feat(performance): add collection settings UI"
```

### Task 6: Model Detail Performance Panel

**Files:**
- Create: `web/src/hooks/model-pricing/useModelPerformance.js`
- Create: `web/src/helpers/performance.js`
- Create: `web/src/components/table/model-pricing/modal/components/ModelPerformancePanel.jsx`
- Create: `web/src/components/table/model-pricing/modal/components/ModelPerformanceStats.jsx`
- Create: `web/src/components/table/model-pricing/modal/components/ModelPerformanceGroupTable.jsx`
- Create: `web/src/components/table/model-pricing/modal/components/ModelPerformanceCharts.jsx`
- Modify: `web/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx`
- Modify: frontend locale JSON files.

- [ ] Implement `useModelPerformance(modelName, visible)` with Axios cancellation, loading/error/empty states and a 60-second in-memory freshness window.
- [ ] Do not request metrics while the side sheet is closed or no model is selected.
- [ ] Add summary values using these display rules:

```text
invalid or <= 0 TPS/latency => "—"
TPS < 10                    => 2 decimal places
TPS >= 10                   => 1 decimal place
latency >= 1000ms           => seconds with 2 decimals
success rate                => 2 decimal places + "%"
```

- [ ] Add a Semi `Table` with one row per group and columns for group, TPS, TTFT, latency and success rate.
- [ ] Add VChart line charts for latency and success-rate/availability series using the existing Semi chart theme.
- [ ] Keep the side sheet width responsive: full width on mobile and at least 720px on desktop so charts and tables fit without overlap.
- [ ] Append the performance panel after pricing; do not remove or rewrite `ModelBasicInfo`, `ModelEndpoints`, dynamic billing or `ModelPricingTable`.
- [ ] Empty state text must explicitly say that only new real Relay requests are collected.
- [ ] Run:

```bash
cd web
bun run i18n:lint
bun run eslint -- src/hooks/model-pricing/useModelPerformance.js \
  src/helpers/performance.js \
  src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx \
  src/components/table/model-pricing/modal/components/ModelPerformancePanel.jsx \
  src/components/table/model-pricing/modal/components/ModelPerformanceStats.jsx \
  src/components/table/model-pricing/modal/components/ModelPerformanceGroupTable.jsx \
  src/components/table/model-pricing/modal/components/ModelPerformanceCharts.jsx
bun run build
```

Expected: all commands succeed.

- [ ] Commit:

```bash
git add web/src/hooks/model-pricing/useModelPerformance.js \
  web/src/helpers/performance.js \
  web/src/components/table/model-pricing/modal \
  web/src/i18n/locales
git commit -m "feat(pricing): show model performance details"
```

### Task 7: Local Migration and End-to-End Verification

**Files:**
- No new source files unless a verified defect is found.

- [ ] Back up the isolated local `new-api-v0132` database before migration.
- [ ] Start the feature backend against the isolated database and Redis DB 1.
- [ ] Confirm `perf_metrics` is created and existing business tables/row counts are unchanged.
- [ ] For a fast smoke test, set `bucket_time=minute` and `flush_interval=1` from the admin UI.
- [ ] Send one real successful Relay request with a valid API token through the `v0.13.2` feature backend. Do not use channel test.
- [ ] Send one controlled failed Relay request using a valid token and an invalid/disabled target model.
- [ ] Verify:

```bash
curl 'http://127.0.0.1:3002/api/perf-metrics?model=gpt-5.4&hours=24'
curl 'http://127.0.0.1:3002/api/perf-metrics/summary?hours=24'
```

Expected:

```text
groups contains at least one active group
request_count reflects both real samples
success_rate is below 100 after the controlled failure
latency is positive
TPS is positive for the successful response when completion tokens are available
```

- [ ] Wait for the current minute bucket to complete and verify one persisted row exists.
- [ ] Restart the backend and verify the completed bucket still appears.
- [ ] Open the model detail panel at desktop and mobile widths and verify no overlap, clipping or blank chart canvas.
- [ ] Restore production-like settings after the smoke test: `bucket_time=hour`, `flush_interval=5`, `retention_days=30`.

### Task 8: Final Regression and Publish

**Files:**
- All files changed by Tasks 1-7.

- [ ] Run backend checks:

```bash
gofmt -w model/perf_metric.go model/perf_metric_test.go \
  setting/perf_metrics_setting pkg/perf_metrics \
  controller/perf_metrics.go controller/perf_metrics_test.go
go test ./model ./setting/perf_metrics_setting ./pkg/perf_metrics ./controller ./service ./router
go test -race ./pkg/perf_metrics
go test ./...
go vet ./...
```

- [ ] Run frontend checks:

```bash
cd web
bun run i18n:lint
bun run lint
bun run eslint
bun run build
```

- [ ] Inspect repository state and ensure no database dump, token, `.env`, log or generated binary is tracked:

```bash
git status --short
git diff --check
git log --oneline 'v0.13.2^{}'..HEAD
```

- [ ] Tag the first reusable personal release only after the smoke test:

```bash
git tag -a skye-v0.13.2-perf.1 -m "NewAPI v0.13.2 with model performance metrics"
git push origin dev/skye-v0.13.2-performance
git push origin skye-v0.13.2-perf.1
```

## 6. Acceptance Criteria

- A fresh `v0.13.2` database starts and creates `perf_metrics` automatically.
- Existing databases migrate without destructive schema changes.
- Disabling collection prevents new samples from being recorded.
- Real successful and failed Relay requests are counted exactly once.
- Channel tests do not affect public model performance.
- The detail API returns only active groups and no sensitive identifiers.
- The model side sheet renders loading, empty, error and data states correctly.
- TPS, average latency, success rate, group table, latency trend and availability trend are visible when samples exist.
- Existing pricing, model metadata and dynamic billing displays remain intact.
- Backend tests, race test, frontend lint/i18n/build and manual smoke checks pass.
- The complete history is pushed only to `ASL-mj/new-api` on `dev/skye-v0.13.2-performance`.

## 7. Rollback

Application rollback is code-only: deploy the previous `v0.13.2` build. The added `perf_metrics` table is isolated and does not change existing rows, so it may remain unused. Drop it only after a database backup and only when permanently abandoning the feature:

```sql
DROP TABLE perf_metrics;
```

Do not delete or rewrite existing `logs`; they remain the source of billing and audit history and are intentionally independent from performance aggregates.
