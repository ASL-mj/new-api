# NewAPI 上游能力升级实施排期

**关联需求：** `docs/superpowers/specs/2026-09-06-upstream-capability-upgrade-requirements.md`
**排期口径：** 1 名后端、1 名前端、1 名测试可并行；D1 为排期启动后的第一个工作日。
**首个发布目标：** S0-S2，形成 MiniMax-H3 的任务、轮询、结算与日志闭环。
**前端基线：** React 18 + Vite + Semi UI，维持当前 Skye.7 风格。

字段、上游协议、计费和测试夹具细则见
[`../specs/2026-09-07-upstream-capability-s0-s2-implementation-contract.md`](../specs/2026-09-07-upstream-capability-s0-s2-implementation-contract.md)。

## 1. 总览

| 阶段 | 工作日 | 可并行项 | 主要产出 | 发布门 |
|---|---:|---|---|---|
| S0 基线与迁移准备 | D1-D2，2 天 | 测试夹具、特性开关 | 任务字段迁移与旧链路基线 | 迁移三数据库通过 |
| S1 通用任务状态机 | D3-D6，4 天 | 前端详情字段 | 失败分类、退避、20 次阈值、幂等结算 | 并发和重启测试通过 |
| S2 MiniMax-H3 | D7-D11，5 天 | H3 设置与详情 UI | H3 `/v2`、媒体参考、usage 与计费 | H3 端到端验收 |
| S3 模型解析与计费身份 | D12-D14，3 天 | 模型配置展示 | 大小写、modifier、reasoning、astra | 表驱动计费回归 |
| S4 协议转换 | D15-D19，5 天 | 协议夹具与渠道测试 UI | GLM/Ollama Responses、流式终态 | golden fixtures 通过 |
| S5 日志视图 | D20-D22，3 天 | 角色详情前端 | 最终额度、角色投影、IP 链路 | 权限矩阵通过 |
| S6 插件平台 | D23-D34，12 天 | 插件管理端、审计 | JS 运行时、版本、动态路由 | 插件生命周期回归 |

总排期为 34 个工作日；S0-S2 为首个发布包，预计 11 个工作日。排期以每阶段发布门通过为前提，下一阶段在上一阶段稳定后启动生产灰度。

## 2. S0：基线、迁移与测试夹具（D1-D2）

### 目标

建立现有异步任务、旧版 Hailuo、任务账务和流式 usage 行为的回归基线，并准备加法数据库迁移与特性开关。

### 文件落点

- `model/task.go`
- `model/main.go`
- `model/task_cas_test.go`
- `service/task_polling.go`
- `service/task_polling_test.go`
- `service/task_billing.go`
- `service/task_billing_test.go`
- 任务配置所在 setting 模块及环境变量加载位置

### 实施项

1. 确认 Task 当前字段与迁移方式，新增 `poll_failures`、`last_poll_at`、`next_poll_at`、错误分类和结算状态。
2. 新增 `task_settlements` 终态账务记录模型，以 `task_id` 唯一索引承载幂等；为 `tasks(status, next_poll_at)` 补跨数据库兼容索引。
3. 加入关闭状态的 `TASK_POLLING_V2_ENABLED` 与默认 `TASK_POLL_MAX_FAILURES=20`。
4. 为旧 Hailuo 成功、上游失败、已预扣任务退款、客户端流中断后的 usage 结算建立固定测试夹具。
5. 在 SQLite、MySQL、PostgreSQL 测试环境验证迁移；SQLite 按项目既有加列模式实施。
6. 调整任务提交持久化顺序：H3 上游提交前写入 `NOT_START` Task 和价格快照，避免上游成功后本地 Task 丢失。

### 发布门

- `go test ./model ./service ./relay/helper` 通过。
- 迁移为纯加法，存量任务仍可读取和轮询。
- 关闭所有新开关时，旧行为与基线一致。
- H3 提交后、本地绑定上游 task ID 前的进程中断具备保留记录和恢复测试。

## 3. S1：通用任务轮询与结算状态机（D3-D6）

### 目标

让所有异步任务拥有统一的失败分类、退避、最大失败次数、CAS 终态转换和幂等账务。

### 文件落点

- `service/task_polling.go`
- `service/task_billing.go`
- `service/task.go`
- `model/task.go`
- `model/task_cas_test.go`
- `service/task_polling_test.go`
- `service/task_billing_test.go`
- `web/src/` 中现有任务详情和使用日志详情组件

### 实施项

1. 实现 `transport`、`rate_limited`、`server_transient`、`malformed_response`、`provider_terminal`、`internal` 分类器。
2. 实现可配置的指数退避与 `next_poll_at` 调度。
3. 实现连续失败归零、达到 20 次后以 `FAILURE + poll_exhausted` 分类终态转换。
4. 将成功结算和失败退款收敛到单一入口，以 `task:{id}:settle` 和 `task:{id}:refund` 作为账务幂等键。
5. 任务详情增加只读轮询诊断、终态与结算字段；沿用既有 Semi UI 表格、描述列表和抽屉布局。

### 测试矩阵

| 场景 | 预期 |
|---|---|
| 连续网络错误 19 次 | 继续按退避轮询 |
| 连续网络错误 20 次 | `FAILURE`，失败分类为 `poll_exhausted`，仅一次退款 |
| 成功读取 `running` | 失败计数归零 |
| 上游明确 `failed` | 立即失败并退款 |
| 两个 worker 同时取得终态 | 一方结算，另一方读取既有结果 |
| 结算服务重启 | 使用幂等键恢复既有账务结论 |

### 发布门

- `go test ./model ./service -run 'Task|Billing|Poll'` 通过。
- 旧 Hailuo、Suno、Kling 任务回归通过。
- 本地测试环境中开关灰度到单个测试渠道，账务与任务记录一一对应。

## 4. S2：MiniMax-H3 视频 Provider（D7-D11）

### 目标

基于既有 Hailuo 通道交付 MiniMax-H3/Hailuo-03 的 `/v2/video_generation` 任务闭环。

### 文件落点

- `relay/channel/task/hailuo/adaptor.go`
- `relay/channel/task/hailuo/models.go`
- `relay/channel/task/hailuo/constants.go`
- 新增 `relay/channel/task/hailuo/h3_dto.go`
- 新增 `relay/channel/task/hailuo/h3_adaptor.go`
- 新增 `relay/channel/task/hailuo/h3_adaptor_test.go`
- `relay/relay_task.go`
- `relay/channel/task/taskcommon/helpers.go`
- H3 task pricing snapshot 与 `OtherRatios` 的预扣/结算测试
- 现有渠道模型配置、任务详情和使用日志详情前端组件

### 实施项

1. 定义 H3 提交、查询和终态回包 DTO；JSON 编解码使用项目 `common/json.go`。
2. 按 `AsyncTaskProvider` 责任拆分校验、提交、查询和 `TaskUsage` 标准化。
3. 支持 4-15 秒、768P、2K、图片/视频/音频参考与 ratio 参数。
4. 保存 provider task id、结果 URL、封面、实际时长、有效期和诊断标识。
5. 以 H3 task pricing snapshot 和 `OtherRatios` 计算时长、分辨率、参考素材的预扣与完成后差额；不扩展 Token 专用 `billingexpr.TokenParams`。
6. 以 `MINIMAX_H3_ENABLED` 控制新模型分发；旧 Hailuo 路径维持原逻辑。
7. 前端在现有配置和详情区域增加 H3 能力、计费维度、结果状态和退款状态展示。

### 验收与灰度

1. 完成图片、视频、音频参考的单项与组合请求夹具。
2. 对 4、8、10、15 秒和两种分辨率验证使用的计费变量。
3. 使用测试渠道完成 `queued -> running -> succeeded` 与 `failed` 两条端到端链路。
4. 灰度仅开放管理员测试模型组；观察任务数、轮询失败类别、退款数和结算差额。
5. 已提交 H3 任务始终持续轮询至终态，即使新建任务入口在灰度期间关闭。

### 首个发布包完成定义

- S0-S2 所有发布门均通过。
- H3 可以提交、轮询、保存结果、产出标准化 usage 并完成单次结算或退款。
- 旧视频任务和现有账务测试保持通过。
- 使用日志及任务详情展示最终金额、任务状态和诊断摘要。

## 5. S3：模型解析与计费身份（D12-D14）

### 目标

建立从原始模型名到路由、上游请求与计费规则的一致身份链路。

### 文件落点

- 新增 `relay/helper/model_identity.go`
- 新增 `relay/helper/model_identity_test.go`
- `relay/helper/billing_expr_request.go`
- `setting/billing_setting/`
- 各依赖模型名的 provider adaptor
- 对应模型配置与多语言显示文件

### 实施项

1. 定义 `ModelIdentity`，含 requested、routing、provider、billing、reasoning 和 provider options。
2. 实现 ASCII 大小写规范化、modifier 解析和 canonical billing identity。
3. 将 `max`、`xhigh` 作为独立值，保留 Provider 原生 reasoning 参数。
4. 加入 `gpt-6-astra` 上下文阈值和长上下文表达式规则。
5. 用表驱动测试覆盖别名、变体、未知模型和价格边界。

### 发布门

- `go test ./relay/helper ./setting/billing_setting ./pkg/billingexpr` 通过。
- 现有模型的路由名、上游模型名与账务金额无意外漂移。

## 6. S4：协议转换与 Provider Responses（D15-D19）

### 目标

补齐 GLM Responses、Ollama Claude Messages/Responses，并强化公共协议转换和流式终态 usage。

### 文件落点

- `relay/channel/zhipu_4v/adaptor.go`
- `relay/channel/zhipu_4v/dto.go`
- `relay/channel/zhipu_4v/relay-zhipu_v4.go`
- `relay/channel/ollama/adaptor.go`
- `relay/channel/ollama/dto.go`
- `relay/channel/ollama/relay-ollama.go`
- `relay/channel/ollama/stream.go`
- `relay/chat_completions_via_responses.go`
- `relay/responses_handler.go`
- `service/openaicompat/chat_to_responses.go`
- `service/openaicompat/responses_to_chat.go`
- `relay/helper/stream_scanner.go`

### 实施项

1. 为 GLM 构建 `/v1/responses` 请求与回包映射。
2. 为 Ollama 构建 Claude Messages 和 OpenAI Responses 转换及回包链路。
3. 编写 Chat、Responses、Claude、Gemini 的 golden fixtures，校验文本、工具调用、错误、finish reason、usage 和事件顺序。
4. 保留本地客户端断开后的 usage drain 行为，基于基线测试补强终态合并。
5. 按 `GLM_RESPONSES_ENABLED` 与 `OLLAMA_RESPONSES_ENABLED` 分渠道灰度。

### 发布门

- 指定协议 fixtures 通过，现有 OpenAI Responses 兼容测试通过。
- 流式连接中断后，已有的成功 usage 仍可进入结算和日志。

## 7. S5：使用日志终态额度与角色视图（D20-D22）

### 目标

将使用日志展示建立在最终结算事实上，并按角色输出受控字段集合。

### 文件落点

- `service/log_info_generate.go`
- `model/log.go`
- `service/billing.go`
- `service/billing_session.go`
- 相关 controller 与日志 DTO
- 使用日志、详情抽屉和多语言文件

### 实施项

1. 定义结算事实到用户、管理员、root、审计视图的投影器。
2. 异步任务成功、补扣、差额退款和失败退款后写入最终账务字段。
3. 追踪 `Log.Ip -> API -> UI`，为使用日志详情补端到端覆盖。
4. 保持 `admin_info` 与现有日志详情布局，新增字段按权限矩阵输出。

### 发布门

- 四种角色视图的金额一致，字段范围符合权限设计。
- 完成后异步任务日志不再显示零额度覆盖最终结算事实。
- 日志详情 IP 的写入、接口输出和 UI 展示具备验证记录。

## 8. S6：托管插件平台（D23-D34）

### 目标

在任务、账务和协议接口稳定后，提供插件版本、生命周期和动态能力注册。

### 文件落点

- 新增 `pkg/jsplugin/`
- 新增 `plugins/`
- 新增插件 model、service、controller、router 与迁移文件
- 新增插件管理 React/Semi UI 页面或现有设置页子模块
- 新增插件审计与多语言资源

### 实施项

1. 定义 manifest、安装包校验、运行时资源限制与宿主 API。
2. 建立 `plugins`、`plugin_versions`、`plugin_activations`、`plugin_audit_logs`。
3. 将插件 capability 桥接到 protocol handler 和 `AsyncTaskProvider`。
4. 实现安装、启停、版本激活、升级、回滚与审计。
5. 以 H3 task plugin fixture 验证动态任务 Provider 能复用 C 包的轮询和账务边界。

### 发布门

- 插件生命周期测试覆盖安装、启停、升级、回滚、失败诊断和审计。
- 插件任务保持单次结算/退款语义。
- 插件管理端延续现有后台样式与权限控制。

## 9. 发布策略与回滚

### 9.1 灰度顺序

1. 测试库执行加法迁移，跑 S0 基线。
2. S1 限定一个低风险测试渠道启用 `TASK_POLLING_V2_ENABLED`。
3. S2 限定管理员测试模型组启用 H3。
4. 观察任务状态分布、轮询错误分类、退款次数、差额金额、日志零额度记录。
5. S3-S5 分别按模型、Provider 和管理员视图开关扩大范围。

### 9.2 回滚边界

| 功能 | 回滚动作 | 已创建数据处理 |
|---|---|---|
| H3 新建入口 | 关闭 `MINIMAX_H3_ENABLED` | 已提交任务继续轮询至终态 |
| 轮询 V2 | 关闭 `TASK_POLLING_V2_ENABLED` | 结算键保留，避免账务重复 |
| 模型身份 V2 | 关闭 `MODEL_IDENTITY_V2_ENABLED` | 原模型映射继续可读 |
| GLM/Ollama Responses | 关闭 Provider 开关 | 现有任务与请求记录保留 |
| 日志投影 V2 | 关闭 `LOG_VIEW_PROJECTION_V2_ENABLED` | 结算事实持续保留 |

## 10. 验证清单

每个阶段完成前执行对应的最小验证集，并在发布门前补全：

```bash
go test ./model ./service
go test ./relay/...
go test ./pkg/billingexpr
cd web && bun run lint
cd web && bun run build
git diff --check
```

涉及数据库迁移的阶段额外执行 SQLite、MySQL、PostgreSQL 的迁移和回归。涉及 Provider 的阶段使用固定 HTTP fixture 与测试渠道各验证一次，固定夹具覆盖结构，测试渠道验证请求与终态联通。

## 11. 提交拆分

```text
docs: add upstream capability upgrade requirements and rollout plan
feat(task): add durable polling failure state and idempotent settlement
feat(hailuo): add MiniMax H3 v2 video task support
feat(billing): introduce canonical model identity and astra pricing
feat(relay): add GLM and Ollama responses compatibility
feat(log): project usage logs by role and reconcile final quota
feat(plugin): add managed task plugin runtime
```

每个提交只包含一个可测试责任边界；迁移、实现、测试和对应多语言资源与所属能力包一起提交。
