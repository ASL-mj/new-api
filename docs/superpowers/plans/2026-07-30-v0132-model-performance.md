# NewAPI v0.13.2 模型性能功能实施计划

> **供自动化执行者使用：** 按任务顺序执行，每个任务完成后运行对应验证。推荐使用 subagent-driven-development，并在每个提交后进行规格与代码质量检查。

**目标：** 在官方 `v0.13.2` 基线上移植模型性能采集、聚合 API 和模型详情性能标签页，同时保留旧版现有功能与技术栈。

**架构：** 真实 Relay 请求完成后写入进程内五分钟聚合桶，并在启用 Redis 时同步活跃桶。完成的桶持久化到独立 `perf_metrics` 表；查询层按 `1h/24h/7d` 自适应汇总，并直接返回基于原始计数计算的 `overall` 与分组数据。旧版 React/Semi UI 通过懒加载性能标签页展示指标、表格与图表。

**技术栈：** Go、Gin、GORM、MySQL/SQLite/PostgreSQL、Redis、React 18、Vite、Semi UI、VChart、Bun。

---

## 基线与边界

- 仓库：`/Users/Zhuanz1/Desktop/privateProjects/ssh/workspace/new-api-v0.13.2`
- 分支：`dev/skye-v0.13.2-performance`
- 基线标签：`v0.13.2`
- 个人远程：`origin = git@github.com:ASL-mj/new-api.git`
- 官方远程：`upstream = git@github.com:QuantumNous/new-api.git`
- 不合并最新版主分支，只移植本功能所需代码。
- 不从历史 `logs` 回填数据。
- 不采集渠道测试，只采集真实 Relay 请求。
- 不引入新版路由、状态管理、TanStack Query 或前端测试框架。
- 不修改现有 `/api/performance/*` 系统资源监控接口。

## 文件职责

### 后端新增

- `model/perf_metric.go`：表结构、原子 upsert、时间范围查询与过期清理。
- `model/perf_metric_test.go`：SQLite 下的唯一桶累加、筛选和清理测试。
- `setting/perf_metrics_setting/config.go`：启用开关、刷新周期、固定五分钟桶和保留天数。
- `setting/perf_metrics_setting/config_test.go`：默认值和边界测试。
- `pkg/perf_metrics/types.go`：Sample、计数器和 API 返回结构。
- `pkg/perf_metrics/metrics.go`：记录、合并、加权总体值和时间 rollup。
- `pkg/perf_metrics/flush.go`：周期落库、失败恢复和过期清理。
- `pkg/perf_metrics/metrics_test.go`：公式、分桶、rollup、overall 和并发测试。
- `controller/perf_metrics.go`：公开只读详情接口。
- `controller/perf_metrics_test.go`：参数、错误和活跃分组过滤测试。

### 后端修改

- `model/main.go`：普通与快速迁移均注册 `PerfMetric`。
- `main.go`：资源初始化后启动性能聚合器。
- `controller/relay.go`：最终失败且重试结束后记录一次失败样本。
- `service/text_quota.go`：文本请求成功后记录完成 Token。
- `service/quota.go`：音频等请求成功后记录完成 Token。
- `router/api-router.go`：注册 `/api/perf-metrics`。

### 前端新增

- `web/src/hooks/model-pricing/useModelPerformance.js`：懒加载、取消、60 秒缓存和重试。
- `web/src/helpers/performance.js`：TPS、延迟和成功率格式化。
- `web/src/components/table/model-pricing/modal/components/ModelOverviewTab.jsx`：旧基本信息、价格与动态计费组合。
- `web/src/components/table/model-pricing/modal/components/ModelApiTab.jsx`：旧端点信息组合。
- `web/src/components/table/model-pricing/modal/components/ModelPerformancePanel.jsx`：性能页状态机与时间范围。
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceStats.jsx`：三张整体指标卡。
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceGroupTable.jsx`：分组性能表。
- `web/src/components/table/model-pricing/modal/components/ModelPerformanceCharts.jsx`：TTFT 与可用率图表。

### 前端修改

- `web/src/components/table/model-pricing/modal/ModelDetailSideSheet.jsx`：改为三标签页 owner，桌面约 760px、移动 100%。
- `web/src/pages/Setting/Performance/SettingsPerformance.jsx`：增加模型性能采集配置。
- `web/src/components/settings/PerformanceSetting.jsx`：如现有 options 解析无法覆盖新字段，则增加默认值与类型转换。
- `web/src/i18n/locales/*.json`：新增可见文案。

## Task 1：数据库与配置

- [ ] 新建 `model/perf_metric_test.go`，验证同一 `model_name + group + bucket_ts` 两次写入只产生一行且计数累加。
- [ ] 验证不同分组或不同桶分别产生独立记录。
- [ ] 验证查询只返回时间范围内的数据，过期清理不删除边界内数据。
- [ ] 运行 `go test ./model`，确认测试先失败。
- [ ] 新建 `model/perf_metric.go`，结构至少包含：

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

- [ ] 使用 GORM `clause.OnConflict` 和 `gorm.Expr` 原子累加，不写数据库方言专用 SQL。
- [ ] 新建 `setting/perf_metrics_setting/config.go`，默认：

```text
enabled=true
flush_interval=5
bucket_seconds=300
retention_days=30
```

- [ ] 不暴露 `hour` 源桶配置；所有趋势基于五分钟原始桶。
- [ ] 在 `model/main.go` 的普通和快速迁移中加入 `PerfMetric`。
- [ ] 运行 `go test ./model ./setting/perf_metrics_setting`。
- [ ] 提交：`feat(performance): add metric storage and settings`。

## Task 2：采集与聚合核心

- [ ] 在 `pkg/perf_metrics/metrics_test.go` 先写失败测试：

```text
2 个请求、1 个成功、总延迟 3000ms => 平均总延迟 1500ms，成功率 50%
200 completion tokens、generation 4000ms => TPS 50
TTFT 只统计 HasTtft=true 的样本
空模型名不记录；空分组归为 default
1h 按 5min 汇总；24h 和 7d 按 1h 汇总
overall 先合并原始计数再计算，不平均 group 结果
overall/group/series 都返回 request_count
```

- [ ] `types.go` 定义：

```go
type BucketPoint struct {
    Ts           int64   `json:"ts"`
    RequestCount int64   `json:"request_count"`
    AvgTtftMs    int64   `json:"avg_ttft_ms"`
    AvgLatencyMs int64   `json:"avg_latency_ms"`
    SuccessRate  float64 `json:"success_rate"`
    AvgTps       float64 `json:"avg_tps"`
}

type AggregateResult struct {
    RequestCount int64         `json:"request_count"`
    AvgTtftMs    int64         `json:"avg_ttft_ms"`
    AvgLatencyMs int64         `json:"avg_latency_ms"`
    SuccessRate  float64       `json:"success_rate"`
    AvgTps       float64       `json:"avg_tps"`
    Series       []BucketPoint `json:"series"`
}

type QueryResult struct {
    ModelName    string        `json:"model_name"`
    Hours        int           `json:"hours"`
    SeriesSchema string        `json:"series_schema"`
    Overall      AggregateResult `json:"overall"`
    Groups       []GroupResult `json:"groups"`
}
```

- [ ] 采集桶键固定为 `model + group + aligned_5min_timestamp`。
- [ ] 查询先合并数据库和当前进程热桶，再按目标桶 `300s` 或 `3600s` rollup。
- [ ] `overall` 从所有允许分组的原始 counters 合并后计算。
- [ ] Redis key 使用 `perf:<model>:<group>:<bucket_ts>`，超时 1 秒，TTL 至少覆盖未落库窗口。
- [ ] `flush.go` 只落库已完成桶；失败时恢复已 drain 的计数器。
- [ ] 运行 `go test ./pkg/perf_metrics` 和 `go test -race ./pkg/perf_metrics`。
- [ ] 提交：`feat(performance): collect and aggregate relay metrics`。

## Task 3：真实 Relay 接入

- [ ] 在 `main.go` 的数据库和 Redis 初始化完成后调用 `perfmetrics.Init()`。
- [ ] `controller/relay.go` 仅在重试全部结束且最终失败时异步记录一次失败。
- [ ] `service/text_quota.go` 成功结算后异步记录 `summary.CompletionTokens`。
- [ ] `service/quota.go` 的音频等成功结算路径异步记录 `usage.CompletionTokens`。
- [ ] 记录字段：

```text
model = relayInfo.OriginModelName
group = relayInfo.UsingGroup
latency = now - relayInfo.StartTime
TTFT = relayInfo.FirstResponseTime - relayInfo.StartTime（仅流式且已首字响应）
generation = 流式时 now - FirstResponseTime，否则等于总延迟
```

- [ ] 使用现有 `gopool.Go`，采集不得阻塞 Relay 响应。
- [ ] 运行 `go test ./controller ./service`。
- [ ] 提交：`feat(performance): record relay outcomes`。

## Task 4：公开只读 API

- [ ] 在 `controller/perf_metrics_test.go` 先写缺少 model、非法 hours、默认 24h、活跃分组过滤和空数据测试。
- [ ] 注册：

```text
GET /api/perf-metrics?model=<model>&hours=<1|24|168>&group=<optional>
```

- [ ] 仅允许 `hours=1|24|168`；缺省为 24，其他值返回 400。
- [ ] 使用 `middleware.TryUserAuth()`，与 `/api/pricing` 的公开行为一致。
- [ ] 返回 `overall`、`groups` 和 `request_count`；不返回用户、Token、渠道、请求 ID 或内容。
- [ ] 过滤掉未激活分组，保留特殊 `auto` 分组。
- [ ] 运行 `go test ./controller ./router`。
- [ ] 提交：`feat(performance): expose model performance API`。

## Task 5：性能设置页面

- [ ] 在 `SettingsPerformance.jsx` 增加独立“模型性能采集”区块。
- [ ] 控件只包含：启用开关、落库刷新分钟数、保留天数。
- [ ] 明确显示源数据固定为五分钟桶，不提供会破坏 `1h` 趋势的小时选项。
- [ ] 关闭采集时禁用刷新与保留天数输入。
- [ ] 保持现有系统性能、磁盘缓存和日志清理设置不变。
- [ ] 在 `PerformanceSetting.jsx` 正确解析 bool 和 number，并按 `/api/option/` 要求提交字符串值。
- [ ] 更新已维护语言文件。
- [ ] 运行 `cd web && bun run i18n:lint && bun run eslint && bun run build`。
- [ ] 提交：`feat(performance): add collection settings UI`。

## Task 6：模型详情三标签页与性能 UI

- [ ] 将 `ModelDetailSideSheet.jsx` 改为受控 `Tabs`，默认 `overview`。
- [ ] 模型变化时重置 `activeTab=overview`；性能组件重置 `hours=24`。
- [ ] 桌面宽度约 760px，移动宽度 100%。
- [ ] `ModelOverviewTab` 复用基础信息、分组价格和动态计费。
- [ ] `ModelApiTab` 复用现有 `ModelEndpoints`。
- [ ] `useModelPerformance(modelName, enabled, hours)` 仅在性能标签激活时请求，支持 AbortController 和 60 秒缓存。
- [ ] `ModelPerformancePanel` 状态：

```text
loading                         => 稳定骨架屏
overall.request_count == 0      => 无数据，隐藏指标和图表
0 < overall.request_count < 10  => 保留真实值并显示样本不足
首次请求失败                    => 错误面板和重试
已有成功数据刷新失败            => 保留旧数据并显示非阻塞警告
```

- [ ] 三张卡只读取后端 `overall`：TPS、平均总延迟、成功率。
- [ ] 分组表列：分组、TPS、平均 TTFT、平均总延迟、成功率。
- [ ] 图表直接使用 `overall.series`：TTFT 趋势与成功率/可用率。
- [ ] 移动端表格水平滚动并显示右侧滚动提示，图表单列。
- [ ] 更新语言文件，所有可见文字走 `t()`。
- [ ] 运行 `cd web && bun run i18n:lint && bun run eslint && bun run build`。
- [ ] 提交：`feat(pricing): add model performance tabs`。

## Task 7：集成验证与发布准备

- [ ] 运行：

```bash
go test ./model ./setting/perf_metrics_setting ./pkg/perf_metrics ./controller ./service ./router
go test -race ./pkg/perf_metrics
go test ./...
go vet ./...
cd web
bun run i18n:lint
bun run eslint
bun run build
```

- [ ] 使用隔离的本地 `new-api-v0132` 数据库启动后端，确认只新增 `perf_metrics` 表。
- [ ] 发送真实成功 Relay 请求和受控失败 Relay 请求，确认各计数只增加一次。
- [ ] 用 `curl` 检查 `hours=1/24/168`，确认点数、overall、request_count 和活跃分组正确。
- [ ] 在桌面 1440px 和移动 390px 下检查标签页、横向表格和图表像素非空。
- [ ] 确认 `.env`、数据库导出、密钥、日志和 `.superpowers/` 未被提交。
- [ ] 检查 `git diff --check` 和完整提交历史。
- [ ] 不自动打标签或部署生产；完成后给出迁移、配置和回滚说明。

## 验收标准

- 数据库迁移仅新增独立表，不修改现有业务数据。
- 禁用采集后不再产生新样本。
- 每个真实成功或最终失败请求只记录一次。
- `overall` 使用原始计数合并，统计不受小流量分组放大影响。
- `1h/24h/7d` 都有有意义的趋势点。
- 模型详情三标签页保留旧版全部功能。
- 空数据、稀疏样本、错误和成功状态可区分。
- 后端测试、race、vet、前端 i18n、eslint、build 和浏览器检查通过。
