# NewAPI 上游能力升级需求基线

**状态：** 已确认，待实施
**建立日期：** 2026-09-06
**本地基线：** `v0.13.2-skye.7-52405375`，分支 `dev/skye-v0.13.2-performance`
**上游参考：** `QuantumNous/new-api v1.0.0-rc.33`
**前端约束：** 延续当前 React 18、Vite、Semi UI 与 Skye.7 信息密度；上游新版前端仅用于行为核对。

S0-S2 的字段、上游协议、计费、测试和灰度细则以
[`2026-09-07-upstream-capability-s0-s2-implementation-contract.md`](2026-09-07-upstream-capability-s0-s2-implementation-contract.md)
为准；该附录对本文件中较早的抽象描述具有优先解释权。

## 1. 目标

在保留本地渠道 affinity、性能设置、日志详情与现有管理端风格的前提下，分阶段吸收上游的任务、视频、模型计费、协议转换、日志视图和插件能力。

首个可发布闭环为：

```text
MiniMax-H3 视频任务提交
  -> 异步轮询
  -> 标准化 usage
  -> 成功差额结算或失败退款
  -> 使用日志与任务详情展示
```

完整 JS 插件平台位于该闭环之后，作为可运行时扩展 Provider 和任务类型的能力建设。

## 2. 当前基线与设计原则

### 2.1 已有能力

本地已具备以下基础能力，可作为本次升级的复用点：

- OpenAI `/v1/responses` 与 `/v1/responses/compact`。
- Chat Completions 与 Responses 的部分转换。
- 旧版 Hailuo 异步视频任务。
- 任务持久化、后台轮询、失败退款、计费快照与完成后差额结算。
- 客户端中断后持续读取上游流并合并 usage。
- `pkg/billingexpr` 的表达式计费框架。
- Gemini `thinking_budget`、部分 OpenRouter/DeepSeek reasoning 处理。
- 使用日志详情定制和 `admin_info` 边界。

### 2.2 设计原则

1. Provider 只负责协议构建、上游调用、状态映射和 usage 事实；预扣、差额结算、退款、日志由共享服务处理。
2. 异步任务的轮询、失败分类、终态转换和退款保持通用，Provider 内只输出上游状态与错误事实。
3. 模型路由名、上游模型名和计费模型名可分离，通过统一身份对象贯通。
4. 数据库变更采用加法迁移，兼容 SQLite、MySQL 与 PostgreSQL。
5. 所有新链路由特性开关控制，已提交任务始终允许走到最终结算状态。
6. 前端以现有 Semi UI 组件、表格、抽屉与设置页为基础；新增字段优先进入现有任务详情和使用日志详情。

## 3. 能力分包与依赖

| 包 | 名称 | 内容 | 依赖 | 交付优先级 |
|---|---|---|---|---|
| A | 模型解析与计费身份 | 大小写模型名、modifier、reasoning、计费身份、`gpt-6-astra` | 现有 relay、`billingexpr` | P2 |
| B | 公共协议转换 | Chat/Responses、Claude/Gemini/OpenAI 转换、GLM 与 Ollama Responses、流式终态 | relay 生命周期 | P3 |
| C | 通用异步任务基础设施 | 轮询失败分类、失败阈值、终态状态机、幂等结算 | Task、Billing Session | P0 |
| D | 视频 Provider | MiniMax-H3/Hailuo-03、媒体参考、结果解析、usage | C、现有 Hailuo adaptor | P1 |
| E | 使用日志与角色视图 | 额度终态展示、用户/管理员/root 视图、审计字段 | C、结算事实 | P4 |
| F | 托管插件平台 | JS 运行时、版本、启停、动态路由、审计 | A/B/C/D 接口稳定 | P5 |

```text
C -> D -> E
A -> B
C + D + A + B -> F
```

视频计费与失败退款属于同一结算链路的两个阶段；视频 Provider 与任务轮询属于具体实现和共享基础设施的关系；插件平台负责加载与注册，视频计费继续归属共享账务能力。

## 4. C 包：通用异步任务基础设施

### 4.1 生命周期

```text
CREATED -> SUBMITTED -> QUEUED -> RUNNING -> SUCCEEDED -> SETTLING -> SETTLED
                         |             |
                         +-------------+-> FAILURE(provider_terminal) -> REFUNDING -> REFUNDED
                                       |
                                       +-> FAILURE(poll_exhausted) -> REFUNDING -> REFUNDED
                                       |
                                       +-> FAILURE(cancelled) -> REFUNDING -> REFUNDED
```

对外 `TaskStatus` 保持现有 `SUCCESS` / `FAILURE` 语义，`poll_exhausted`、取消和 Provider 明确失败通过失败分类表达，避免新增状态破坏现有视频 API 映射。终态转换使用现有 Task CAS/条件更新语义；账务幂等由独立结算记录表保证，不能只依赖 Task 状态或一次 CAS。

### 4.2 数据字段

在 `model/task.go` 的现有任务模型上增加或确认以下字段：

| 字段 | 含义 |
|---|---|
| `poll_failures` | 连续且可重试的轮询失败次数 |
| `last_poll_at` | 最近轮询时间 |
| `next_poll_at` | 下次轮询时间 |
| `last_poll_error_class` | `transport`、`rate_limited`、`server_transient`、`malformed_response`、`provider_terminal`、`internal` |
| `last_poll_error_message` | 已脱敏的错误摘要 |
| `settlement_status` | 任务视图使用的 `pending`、`processing`、`settled`、`refunded`、`legacy` |
| `settled_at` | 账务终态时间 |

新增索引：`(status, next_poll_at)` 支持调度扫描。财务幂等键存于独立 `task_settlements` 表并对 `task_id` 建唯一索引；不在 `tasks` 表的非空字符串列上建立唯一 `settlement_key`，避免存量空值和崩溃恢复带来错误语义。

### 4.3 失败处理

新增配置：

```text
TASK_POLL_MAX_FAILURES=20
TASK_POLL_BASE_BACKOFF_SECONDS=10
TASK_POLL_MAX_BACKOFF_SECONDS=300
```

规则如下：

1. 上游成功返回 `queued` 或 `running` 时，`poll_failures` 归零并计算下一次轮询时间。
2. 网络、临时服务端错误、限流和回包格式异常进入可重试分类，计数递增并按退避策略调度。
3. 上游明确业务失败、取消或资源失效直接进入任务失败终态。
4. 连续可重试失败达到 20 次时，任务进入 `FAILURE`，失败分类为 `poll_exhausted`，记录最后一次错误摘要并发起退款。
5. 任务完成结算与失败退款均以任务幂等键执行，重复轮询仅读取已完成结果。

### 4.4 结算契约

```go
type TaskUsage struct {
    Provider              string
    ProviderModel         string
    CanonicalBillingModel string
    TaskType              string
    DurationSeconds       int
    Resolution            string
    ReferenceImageCount   int
    ReferenceVideoCount   int
    ReferenceAudioCount   int
    ProviderUsage         map[string]any
}
```

账务顺序：预扣成功后创建任务；成功终态使用 `TaskUsage` 计算最终金额并补扣或退差额；失败终态退款。`task_settlements` 以 `task_id` 唯一记录终态账务意图、实际金额和处理状态，`service/task_billing.go` 是任务终态账务唯一入口。

### 4.5 验收标准

1. 每个任务在并发轮询场景中只产生一次成功结算或一次退款。
2. 第 20 次连续可重试失败后，任务进入可观测失败终态且自动退款。
3. 有效的非终态上游状态将连续失败计数归零。
4. worker 重启后可根据已持久化状态继续轮询和结算。
5. 原有 Hailuo、Suno、Kling 等任务类型的状态与账务回归通过。

## 5. D 包：MiniMax-H3 视频 Provider

### 5.1 支持范围

首期在现有 `relay/channel/task/hailuo/` Go adaptor 中新增 H3 `/v2/video_generation` 能力，覆盖：

- MiniMax-H3、Hailuo-03 对应模型路由。
- 图片参考、视频参考和音频参考。
- 768P、2K。
- 4 至 15 秒视频任务。
- 异步任务提交、查询、结果 URL 保存与终态映射。
- 将时长、分辨率与参考素材写入 `TaskUsage`。

### 5.2 Provider 接口

```go
type AsyncTaskProvider interface {
    Validate(ctx context.Context, request TaskRequest) error
    Submit(ctx context.Context, request TaskRequest) (ProviderTask, error)
    Poll(ctx context.Context, task ProviderTask) (ProviderTaskStatus, error)
    NormalizeUsage(ctx context.Context, task ProviderTask) (TaskUsage, error)
}
```

H3 adaptor 负责校验与转换，通用任务服务负责持久化、调度和结算。该接口将作为后续插件运行时接入任务能力的稳定边界。

### 5.3 请求与结果

内部任务请求支持：

```text
model
prompt
negative_prompt
duration_seconds: 4-15
resolution: 768P | 2K
aspect_ratio
metadata.content[] 中 role=`reference_image` 的图片
metadata.reference_video
metadata.reference_audio
seed / provider_options
```

适配器根据 Provider 能力矩阵校验模型、时长、分辨率、比例和媒体引用。任务结果保存上游任务标识、视频 URL、封面 URL、实际时长、结果过期时间与 provider 诊断标识。

### 5.4 计费

H3 只输出标准化计费事实。首期沿用任务按次计费的 `OtherRatios` 和冻结的 `TaskBillingContext`，由 H3 task pricing snapshot 决定预扣与完成后的实际差额；不向仅定义 Token 变量的 `pkg/billingexpr.TokenParams` 注入视频维度。渠道成本与单位价格不写入 adaptor。参考图片的精确对外字段、媒体约束和价格快照结构见 S0-S2 实施契约。

### 5.5 验收标准

1. 图片、视频、音频参考的独立和组合请求可完成 schema 校验与上游请求构建。
2. 4、8、10、15 秒以及 768P、2K 进入对应计费表达式分支。
3. 上游 `queued`、`running`、`succeeded`、`failed` 映射到本地状态机。
4. 提交错误、上游失败和轮询耗尽均进入统一退款链路。
5. 已完成任务保存可展示的结果信息和最终账务记录。

## 6. A 包：模型解析与计费身份

### 6.1 统一流程

```text
raw_model
  -> ASCII 大小写归一化
  -> modifier / suffix 解析
  -> provider-native reasoning 参数保留
  -> provider model
  -> canonical billing identity
  -> 上下文规则和价格表达式
```

```go
type ModelIdentity struct {
    RequestedModel  string
    RoutingModel    string
    ProviderModel   string
    BillingModel    string
    ReasoningEffort string
    ProviderOptions map[string]any
}
```

### 6.2 需求

- 支持 ASCII 模型名大小写变体，路由和模型计费遵循同一规范化规则。
- 统一解析尾部参数，分别保留路由行为与上游原生参数。
- `max` 与 `xhigh` 作为独立推理等级；Provider 原生 reasoning 字段原样进入对应 adaptor。
- `gpt-6-astra` 录入默认上下文、长上下文阈值和表达式计费规则。
- 使用 canonical billing model 匹配价格，避免别名和 modifier 导致的计费漂移。

### 6.3 验收标准

表驱动测试覆盖大小写、别名、suffix、`max`、`xhigh`、原生 reasoning、长上下文阈值和未知模型的现有通用规则。

## 7. B 包：公共协议转换与 Provider 兼容

### 7.1 GLM

在 `relay/channel/zhipu_4v/` 中补充 `/v1/responses` 请求构建、流式与非流式回包映射、usage 提取、错误映射与渠道验证测试。

### 7.2 Ollama

在 `relay/channel/ollama/` 中补充 Claude Messages 与 OpenAI Responses 透传，统一最终 usage 和工具调用事件。

### 7.3 公共转换边界

复用并增强 `relay/chat_completions_via_responses.go`、`relay/responses_handler.go`、`service/openaicompat/` 和 `relay/helper/stream_scanner.go`。采用协议 golden fixtures 验证文本块、工具调用、错误结构、finish reason、usage 和流式终态。客户端断开后继续 drain 上游 usage 的本地既有行为作为回归基线。

## 8. E 包：使用日志与角色视图

结算事实与视图投影分离：

```text
结算事实：预扣、最终费用、差额、退款、usage、task id、provider request id
  ->
日志视图：用户 / 管理员 / root / 审计
```

### 8.1 需求

- 使用日志额度采用最终结算事实，异步任务完成或退款后更新最终账务字段。
- 用户视图展示模型、用量、扣费或退款、任务状态和结果地址。
- 管理员视图增加渠道、分组、倍率、错误分类和上游摘要。
- root 视图提供结算键、内部关联标识和审计关联。
- 使用日志详情中的 `Log.Ip -> API -> UI` 链路单独完成端到端校验。
- 现有 `admin_info` 和本地日志详情布局继续保留。

### 8.2 验收标准

同一结算事实在四种角色投影中金额一致，字段集合满足权限矩阵；异步任务成功、差额退款和失败退款均展示最终金额；日志详情 IP 链路有对应测试或端到端验证记录。

## 9. F 包：托管插件平台（后置）

插件平台用于动态加载和管理能力，覆盖：

```text
JS sandbox
  -> manifest 校验
  -> install / enable / disable / upgrade
  -> provider、route、task capability 注册
  -> 审计与版本回滚
```

最小数据模型包括 `plugins`、`plugin_versions`、`plugin_activations` 和 `plugin_audit_logs`。运行时桥接层将 JS Provider 映射到 `AsyncTaskProvider`、协议 handler 和 `TaskUsage`。

准入条件：C 的终态与账务幂等稳定，D 的 H3 闭环稳定，B 的协议 fixtures 稳定。插件管理端延续现有设置页面、表格和抽屉模式。

## 10. 前端需求

1. 任务详情新增轮询诊断、结算状态、退款状态和视频计费维度。
2. H3 在现有渠道/模型配置页提供能力说明，展示可用时长、分辨率和参考素材支持。
3. 使用日志详情展示终态金额、差额、退款和任务结果状态。
4. 插件平台阶段增加插件列表、版本、启停、配置与审计抽屉。
5. 新增可见文字、状态和错误码补齐现有多语言。

## 11. 配置、观测与灰度

```text
TASK_POLLING_V2_ENABLED=false
TASK_POLL_MAX_FAILURES=20
MINIMAX_H3_ENABLED=false
MODEL_IDENTITY_V2_ENABLED=false
GLM_RESPONSES_ENABLED=false
OLLAMA_RESPONSES_ENABLED=false
LOG_VIEW_PROJECTION_V2_ENABLED=false
```

核心指标：

```text
task_poll_attempt_total
task_poll_failure_total{class}
task_terminal_total{status}
task_refund_total
task_settlement_delta_total
video_task_duration_seconds
relay_stream_usage_reconciled_total
log_quota_zero_after_settlement_total
```

## 12. 范围外事项

- 本轮不以官方新版前端重构本地管理端。
- 首个 H3 交付不要求用户上传或在线升级 JS 插件。
- 首期不变更现有渠道 affinity、性能设置、用户排行和本地菜单能力。
- Provider 商务单价由当前渠道成本和计费配置维护，文档不固化临时价格。
