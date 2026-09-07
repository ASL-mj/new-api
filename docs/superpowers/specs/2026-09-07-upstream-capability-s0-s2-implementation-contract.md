# NewAPI 上游能力升级 S0-S2 实施契约

**关联需求：** [上游能力升级需求基线](2026-09-06-upstream-capability-upgrade-requirements.md)
**关联排期：** [上游能力升级实施排期](../plans/2026-09-06-upstream-capability-upgrade-rollout.md)
**适用阶段：** S0 基线与迁移、S1 通用任务状态机、S2 MiniMax-H3
**状态：** 已确认，作为 S0-S2 的编码、测试和发布依据

## 1. 目标与边界

本附录将首个发布包的抽象方案收敛为可实现契约：

```text
现有视频任务 API
  -> MiniMax-H3 /v2 Provider adaptor
  -> Task 调度字段与终态状态机
  -> 终态账务记录
  -> 用户额度、令牌额度、渠道额度、使用日志
```

本附录不修改对外视频路由：继续支持现有 `/v1/video/generations`、`/v1/videos` 与查询接口。H3 专用能力通过现有请求的 `metadata` 传入；未定义的字段不作为首期公开契约。

首期只实现 Go 原生 H3 adaptor，不引入 JS 插件运行时。插件平台后续仅复用本附录定义的任务、usage 和结算边界。

## 2. 已验证的本地边界

| 现有实现 | S0-S2 决策 |
|---|---|
| `TaskStatus` 对外映射只识别 `SUCCESS` 与 `FAILURE` 为终态 | 不新增对外 `POLL_EXHAUSTED` 状态；轮询耗尽落为 `FAILURE`，由失败分类区分 |
| `Task.UpdateWithStatus` 是状态 CAS | 继续用于终态所有权；它不是账务幂等替代品 |
| `TaskPrivateData.BillingContext` 已冻结模型价格、分组倍率和 `OtherRatios` | H3 的请求事实、价格版本和预扣倍率在此快照中持久化 |
| `service/task_billing.go` 当前退款和差额结算会直接修改额度 | S1 在其前增加持久化结算记录和事务边界 |
| `pkg/billingexpr.TokenParams` 只定义 Token、图像/音频 Token 变量 | H3 不把秒数、分辨率或参考素材硬塞入 Token 表达式环境 |

## 3. S0 数据模型与迁移

### 3.1 `tasks` 的加法字段

在 `model.Task` 增加下列字段。时间均使用 Unix 秒并以 `0` 表示未发生；这与现有 `SubmitTime`、`StartTime`、`FinishTime` 保持一致。

| Go 字段 | 列名 | 类型与默认值 | 用途 |
|---|---|---|---|
| `PollFailures` | `poll_failures` | `int NOT NULL DEFAULT 0` | 连续可重试轮询失败次数 |
| `LastPollAt` | `last_poll_at` | `bigint NOT NULL DEFAULT 0` | 最近一次发起轮询的时间 |
| `NextPollAt` | `next_poll_at` | `bigint NOT NULL DEFAULT 0` | 最早允许下次轮询的时间 |
| `LastPollErrorClass` | `last_poll_error_class` | `varchar(40) NOT NULL DEFAULT ''` | 最近错误分类 |
| `LastPollErrorMessage` | `last_poll_error_message` | `text`，可空 | 已脱敏且截断后的错误摘要 |
| `SettlementStatus` | `settlement_status` | `varchar(20) NOT NULL DEFAULT 'pending'` | 任务视图中的账务状态 |
| `SettledAt` | `settled_at` | `bigint NOT NULL DEFAULT 0` | 成功完成账务处理的时间 |

索引采用 GORM 复合索引 `idx_task_poll_ready(status, next_poll_at)`。现有 `status` 单列索引保留，避免影响历史任务查询。

### 3.2 `task_settlements` 终态账务表

Task 字段只记录任务状态；`task_settlements` 是终态账务的唯一事实来源。新模型建议如下：

| 列 | 类型 | 规则 |
|---|---|---|
| `id` | 项目既有主键类型 | 主键 |
| `task_id` | `bigint NOT NULL` | 唯一索引 `uq_task_settlements_task_id` |
| `kind` | `varchar(16)` | `settle` 或 `refund` |
| `state` | `varchar(16)` | `pending`、`applying`、`applied`、`failed` |
| `pre_consumed_quota` | `int` | 创建任务时的预扣额度 |
| `actual_quota` | `int`，可空 | 成功时最终额度；退款为空 |
| `quota_delta` | `int` | 正数补扣、负数退还、退款为负预扣额度 |
| `standard_quota_delta` | `int` | 渠道标准口径差额 |
| `reason_class` | `varchar(40)` | 终态或失败分类 |
| `reason_message` | `text`，可空 | 脱敏摘要 |
| `pricing_snapshot` | JSON | 价格规则版本与实际 usage 事实 |
| `attempt_count` | `int NOT NULL DEFAULT 0` | 账务执行次数 |
| `applied_at` | `bigint NOT NULL DEFAULT 0` | 完成时间 |
| `created_at` / `updated_at` | Unix 秒 | 审计时间 |

`task_id` 只能有一条终态账务记录：成功任务写 `settle`，失败任务写 `refund`。这比在 `tasks` 的字符串键上建立唯一约束可靠，且能记录“已决定终态、账务尚未完成”的恢复状态。

### 3.3 H3 价格快照

在 `TaskBillingContext` 增加可选 `TaskPricingSnapshot`，H3 任务写入该字段；旧任务保持 `nil` 并继续原计费逻辑。建议的可序列化结构如下：

```go
type TaskPricingSnapshot struct {
    Kind        string         `json:"kind"` // "minimax_h3"
    Version     string         `json:"version"`
    RuleHash    string         `json:"rule_hash"`
    BaseQuota   int            `json:"base_quota"`
    H3Rule      H3PricingRule  `json:"h3_rule"`
    Submitted   H3UsageFacts   `json:"submitted"`
}

type H3UsageFacts struct {
    OutputSeconds      float64 `json:"output_seconds"`
    Resolution         string  `json:"resolution"`
    InputImages        int     `json:"input_images"`
    InputVideoSeconds  float64 `json:"input_video_seconds"`
    ReferenceAudios    int     `json:"reference_audios"`
}
```

`H3PricingRule` 记录按输出秒数与分辨率、输入图片、输入视频秒数、音频参考数计算标准额度的完整规则或离散价格表。创建任务时将规则、规则哈希、提交 usage 和预扣金额一起冻结；完成时以同一规则计算实际额度。`OtherRatios` 只保存本次预扣所用的展示和兼容乘数，H3 adaptor 的 `AdjustBillingOnComplete` 返回由 snapshot 算出的最终额度，避免从实时配置重新取价。

### 3.4 迁移与存量规则

1. `model/main.go` 和快速迁移路径均通过 `AutoMigrate(&Task{}, &TaskSettlement{})` 创建加法字段和表。
2. 首次发布仅迁移 schema，不为历史终态任务反填结算记录。历史终态任务将 `settlement_status` 标为 `legacy`，避免后台重新退款或重算。
3. 存量未终态任务初始化为 `pending`、失败计数 `0`、`next_poll_at=0`；首个轮询周期可立即获取。
4. migration 失败即阻止新版本服务接管任务轮询；不执行删列、改列、全表锁定或现有账务回放。
5. SQLite、MySQL、PostgreSQL 各执行一次新库迁移和带存量 Task 的升级迁移。SQLite 使用项目现有 GORM 加列方式，不使用 `ALTER COLUMN`。

### 3.5 提交持久化边界

现有提交路径按“上游提交成功 -> `SettleBilling` -> 使用日志 -> `task.Insert`”处理。若进程在上游已创建 H3 任务后、`task.Insert` 前终止，本地会失去轮询、结果和终态账务锚点。S0 必须将此路径收敛为以下顺序后才能开启 H3：

```text
1. 生成公开 task_id，计算并冻结 TaskPricingSnapshot。
2. 在本地创建 `Status=NOT_START` 的 Task（submission-pending 语义），写入计费会话身份和价格快照。
3. 提交上游 H3。
4. 成功时在事务中绑定 upstream_task_id，置为 SUBMITTED，并提交本次预扣/日志事实。
5. 上游明确拒绝时终止预扣会话，记录受控提交失败审计；不创建可轮询的成功任务。
6. 本地在第 4 步失败时保留该 `NOT_START` 记录，由恢复作业按公开 ID、渠道和提交时间诊断；禁止静默丢弃。
```

该调整仅改变内部持久化顺序，对客户端仍只在上游成功后返回公开 `task_...`。S0 的测试需覆盖第 3、4 步之间进程停止的恢复分支；未完成该项时 `MINIMAX_H3_ENABLED` 保持关闭。

## 4. S1 状态、轮询和结算契约

### 4.1 状态与失败分类

对外 `Task.Status` 只使用既有值：`SUBMITTED`、`QUEUED`、`IN_PROGRESS`、`SUCCESS`、`FAILURE`。新增分类：

| 分类 | 触发条件 | Task 结果 | 下一步 |
|---|---|---|---|
| `transport` | 连接、DNS、超时 | 保持未终态 | 退避重试 |
| `rate_limited` | 上游 429 或明确的稍后重试响应 | 保持未终态 | 按 Retry-After 或退避重试 |
| `server_transient` | 上游 5xx | 保持未终态 | 退避重试 |
| `malformed_response` | 非法 JSON、缺失任务状态、未知结构 | 保持未终态 | 计数后重试 |
| `provider_terminal` | 上游明确 `failed`、`cancelled` 或不可恢复 4xx | `FAILURE` | 退款 |
| `poll_exhausted` | 连续可重试失败达到上限 | `FAILURE` | 退款 |
| `internal` | 本地持久化、配置、解析内部错误 | 保持未终态或告警 | 人工诊断；只在明确不可恢复时终态 |

`TASK_POLL_MAX_FAILURES` 默认 `20`。成功解析 `SUBMITTED`、`QUEUED` 或 `IN_PROGRESS` 后必须将 `poll_failures` 清零。`LastPollErrorMessage` 最多保存 1,024 字节的脱敏文本；原始上游 body 只保留现有受控任务数据，不写入用户视图。

### 4.2 退避规则

```text
base = TASK_POLL_BASE_BACKOFF_SECONDS，默认 10
max  = TASK_POLL_MAX_BACKOFF_SECONDS，默认 300
delay(n) = min(max, base * 2^(min(n - 1, 5)))
```

如上游提供可解析、正数且不超过 300 秒的 `Retry-After`，取 `max(delay(n), Retry-After)`。调度查询只选择未终态且 `next_poll_at <= now` 或为 `0` 的任务。

### 4.3 终态与账务顺序

```text
1. 解析上游结果，得到 SUCCESS 或 FAILURE。
2. 使用 Task.UpdateWithStatus(oldStatus) 取得终态所有权。
3. 在同一数据库事务创建 task_settlements(task_id)；唯一冲突表示已有结算执行者。
4. 成功任务填充 actual_quota、差额和 pricing_snapshot；失败任务填充退款额度和失败分类。
5. 在同一事务内更新钱包/订阅额度、渠道标准额度、结算记录 state=applied 和 Task.settlement_status。
6. 令牌 Redis 缓存更新由已提交的账务事件驱动；失败时可重放，不作为唯一账务事实。
7. 事务提交后记录使用日志和指标；日志失败进入可重试 outbox，不逆转账务。
```

`RefundTaskQuota` 和 `RecalculateTaskQuota` 需要重构为由结算记录驱动的内部操作，禁止在轮询分支直接重复调用。所有账户余额、订阅余额和渠道数据库口径写入必须共用事务；令牌缓存异常通过对账重试处理。

### 4.4 任务详情字段

普通用户只显示任务状态、进度、最终扣费或退款、结果地址和可读失败原因。管理员额外显示 `poll_failures`、错误分类、最近轮询时间、下次轮询时间、结算状态和脱敏上游摘要。root 视图可显示结算记录标识和价格规则版本，不显示渠道原始密钥。

## 5. S2 MiniMax-H3 Provider 契约

### 5.1 模型与上游路径

| 项目 | 固定值 |
|---|---|
| 上游模型 | `MiniMax-H3` |
| 提交 | `POST {base_url}/v2/video_generation` |
| 查询 | `GET {base_url}/v2/query/video_generation/{upstream_task_id}` |
| 鉴权 | `Authorization: Bearer {channel_api_key}` |
| 提交 headers | `Content-Type: application/json`、`Accept: application/json` |
| 查询 headers | `Accept: application/json` |

H3 与当前 Hailuo v1 的差异是：提交使用多模态 `content` 数组、查询任务标识位于 path、查询响应以 `{ "task": { ... } }` 包装。H3 必须使用独立 DTO，不能复用 v1 的 `VideoRequest` / `QueryTaskResponse` 直接反序列化。

### 5.2 对外请求映射

对外继续使用现有 `relaycommon.TaskSubmitReq`：`model`、`prompt`、`duration`、`size`、`image` 和 `metadata`。H3 的新增契约全部位于 `metadata`：

| 输入 | 上游字段 | 规则 |
|---|---|---|
| `model` | `model` | 必须为映射后的 `MiniMax-H3` |
| `prompt` | `content[]` 的 `{type:"text", text}` | 非空文本必需；`metadata.content` 提供文本时优先使用 |
| `duration` | `duration` | 整数 4-15；缺省 5 |
| `size`、`resolution` | `resolution` | 仅 `768P`、`2K`；缺省 `768P` |
| `metadata.ratio` | `ratio` | `adaptive`、`21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16` |
| `image`、`images[]` 或 `metadata.first_frame_image` | `content[].image_url`，role=`first_frame` | 普通图片输入仅表示 frame；最多一个首帧 |
| `metadata.last_frame_image` | `content[].image_url`，role=`last_frame` | 最多一个 |
| `metadata.content[]` 的 `image_url` 项 | `content[].image_url`，role=`reference_image` | 参考图片仅通过该显式多模态项表达；与 frame 不可混用，所有图片最多 9 张 |
| `metadata.reference_video` | `content[].video_url`，role=`reference_video` | 最多 3 个，参考视频总输入时长最多 15 秒 |
| `metadata.reference_audio` | `content[].audio_url`，role=`reference_audio` | 最多 3 个 |
| `metadata.callback_url` | `callback_url` | 仅受控渠道配置或显式白名单 URL |
| `metadata.aigc_watermark` | `aigc_watermark` | 布尔值，显式 `false` 需保留 |

传入 `metadata.content` 时必须是数组；每一项为 `text`、`image_url`、`video_url` 或 `audio_url`。URL 媒体对象使用 `{ "url": "..." }`，例如：

```json
{
  "model": "MiniMax-H3",
  "content": [
    {"type": "text", "text": "夜景城市航拍，镜头平稳推进"},
    {"type": "image_url", "role": "first_frame", "image_url": {"url": "https://ASSET_HOST/frame.jpg"}}
  ],
  "resolution": "768P",
  "duration": 5,
  "ratio": "adaptive"
}
```

校验规则：文本必须非空；`first_frame` 和 `last_frame` 各最多一个；图片总数最多 9；视频、音频各最多 3；frame 图片与 reference 图片/视频/音频不可混用；无视觉输入时 `adaptive` 不可用，默认比例为 `16:9`；有视觉输入且未指定比例时默认 `adaptive`。

所有媒体 URL 必须先经过现有 SSRF、域名/IP 和可访问性策略。对于 Base64 或已上传文件，先转换为受控短时媒体 URL；不将大 Base64 内容持久化到 Task 或日志。

### 5.3 提交、查询与状态映射

提交成功最低要求为 `{ "task_id": "UPSTREAM_TASK_ID" }`；H3 提交错误优先读取 `{ "error": { "message": "...", "http_code": 0 } }`。创建任务后，外部响应继续返回本地公开 `task_...` ID；上游 task ID 只存入 `TaskPrivateData.UpstreamTaskID`。

查询状态映射如下：

| 上游 `task.status` | 本地 Task 状态 | 进度 |
|---|---|---|
| `queued` | `QUEUED` | `30%` |
| `running` | `IN_PROGRESS` | `50%` |
| `succeeded` | `SUCCESS` | `100%` |
| `failed` | `FAILURE(provider_terminal)` | `100%` |
| `cancelled` | `FAILURE(provider_terminal)` | `100%` |
| 其他或缺失 | `malformed_response` | 不终态，计数重试 |

成功时结果地址读取 `task.content.url` 并存入 `TaskPrivateData.ResultURL`。失败信息优先读取 `task.error.message`，无值时使用 `task.status` 生成简短原因。查询 `error.http_code` 或 `error.code` 为 408、429、5xx 时分类为可重试；其余明确 4xx 归为 `provider_terminal`。

### 5.4 usage 与价格事实

提交时冻结事实：

```json
{
  "pricing_version": "h3-v1",
  "requested_seconds": 5,
  "resolution": "2K",
  "input_images": 2,
  "reserved_input_video_seconds": 15,
  "reference_audio_count": 1
}
```

完成时只接受满足边界的上游事实：

| 上游字段 | 本地事实 | 接受范围 | 无效时动作 |
|---|---|---|---|
| `task.usage.output_seconds` | `actual_output_seconds` | 4-15，可为小数 | 保留请求时长 |
| `task.usage.input_image_count` | `actual_input_images` | 0-9，整数 | 保留请求图片数 |
| `task.usage.input_seconds` | `actual_input_video_seconds` | 0-15，可为小数 | 保留预留值 |
| `task.resolution` | `actual_resolution` | `768P` 或 `2K` | 保留请求分辨率 |

上游响应没有可验证的音频参考使用量时，`reference_audio_count` 保持提交快照，不将猜测值写入实际 usage。

### 5.5 首期计费规则

当前 `billingexpr` 只覆盖 Token 语义，因此 H3 首期采用版本化任务价格快照与 `OtherRatios`：

```text
base_quota = configured per-call base quota
precharge  = round(base_quota * requested_duration_ratio * resolution_ratio
                   * image_reference_ratio * reserved_video_ratio * audio_reference_ratio)
actual     = round(base_quota * actual_duration_ratio * actual_resolution_ratio
                   * actual_image_ratio * actual_video_ratio * audio_reference_ratio)
delta      = actual - precharge
```

其中每项 ratio 与 `pricing_version` 一同写入 `TaskBillingContext.TaskPricingSnapshot`。完成时只使用该快照，而非重新读取可变的渠道价格配置。`OtherRatios` 保存本次请求适用的乘数，便于现有使用日志和渠道统计继续显示；snapshot 保存构成乘数的原始事实和规则版本，便于审计与重算解释。

H3 模型未配置完整价格快照时拒绝新任务创建；不以零价格、默认一倍或临时硬编码费率兜底。

## 6. 测试夹具与验收矩阵

### 6.1 固定 HTTP 夹具

| 夹具 | 断言 |
|---|---|
| H3 文生视频提交 | 默认 5 秒、768P、16:9，提交路径和 Authorization 正确 |
| H3 首帧图提交 | `first_frame` 与默认 `adaptive` 正确 |
| H3 参考媒体提交 | 图片/视频/音频边界及不得与 frame 混用 |
| H3 参数错误 | 时长非整数、超范围、2K 以外分辨率、无视觉 `adaptive` 在本地拒绝 |
| H3 `queued` / `running` | 状态、进度与失败计数归零正确 |
| H3 `succeeded` | URL、usage、实际计费事实和差额结算正确 |
| H3 `failed` / `cancelled` | `FAILURE`、原因、单次退款正确 |
| H3 查询 429/5xx | 可重试分类、退避、失败计数正确 |
| H3 查询未知状态/非法 JSON | `malformed_response`、不提前退款 |

### 6.2 账务恢复夹具

1. 两个 worker 同时将同一 Task 推进终态，`task_settlements` 只有一条记录且只有一次余额变化。
2. Task 已变更为 `SUCCESS`，进程在账务提交前停止；重启后从 `pending` 结算记录恢复。
3. 账务数据库事务提交后，日志写入失败；重试只补日志，不改变余额。
4. 连续 20 次可重试失败后写入单条 `refund` 记录；后续轮询不再退款。
5. 存量终态 Task 被扫描时标记为 `legacy`，不创建退款或补扣。

### 6.3 三数据库迁移验证

每个数据库运行以下序列：空库迁移、含旧 `tasks` 表迁移、插入旧终态与未终态任务、创建结算记录、验证唯一约束、执行轮询选择查询。验收时记录实际使用的数据库版本和迁移输出。

## 7. 灰度、观测和停止条件

### 7.1 开关和准入

```text
TASK_POLLING_V2_ENABLED=false
TASK_POLL_MAX_FAILURES=20
MINIMAX_H3_ENABLED=false
```

启用顺序：测试库迁移 -> 单测试渠道开启轮询 V2 -> 管理员测试模型组开启 H3 -> 小范围模型组 -> 常规渠道。H3 开启前必须完成价格快照配置、固定夹具和测试渠道成功/失败链路。

### 7.2 指标与阈值

| 指标 | 灰度观察 | 停止扩容条件 |
|---|---|---|
| `task_poll_failure_total{class}` | 分类分布与趋势 | `malformed_response` 或 `internal` 连续增长 |
| `task_terminal_total{status}` | 成功/失败比例 | H3 失败比例较测试基线显著升高 |
| `task_refund_total` | 退款数与任务失败数对应 | 同一 task 出现多次退款 |
| `task_settlement_delta_total` | 预扣与实际差额 | 超出已配置价格规则的异常大差额 |
| `log_quota_zero_after_settlement_total` | 终态日志额度 | 出现非零账务而日志持续为零 |

发现重复结算、同任务多次退款、余额与结算记录不一致、上游 H3 成功但无结果 URL时，立即关闭 H3 新建入口；已提交任务保留轮询直至终态。

## 8. 责任与完成定义

| 角色 | 负责事项 | 交付物 |
|---|---|---|
| 后端负责人 | Task 迁移、轮询状态机、结算记录、H3 adaptor | Go 代码、迁移、单元与集成测试 |
| 前端负责人 | 任务详情、日志详情、H3 能力提示与多语言 | Semi UI 页面与静态检查 |
| 测试/发布负责人 | 固定夹具、三数据库迁移、测试渠道、灰度监控 | 验收记录、发布与回滚检查表 |

S0-S2 完成的最低条件：迁移三数据库通过；H3 提交、轮询、成功、失败各至少有一条测试渠道验证；任务余额、令牌额度、渠道标准口径和使用日志可按同一 task ID 对账；旧 Hailuo 回归通过；前端保持现有 Skye.7 风格。
