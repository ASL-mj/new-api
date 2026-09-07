# NewAPI 上游能力升级 AI 交接文档

**交接日期：** 2026-09-07
**当前阶段：** 设计与排期已完成，尚未开始业务代码实施
**下一阶段：** S0 - 基线、迁移和提交持久化边界
**本次交接目标：** 让下一位 AI 从 S0 开始实施，不重做需求分析，不跳过账务和迁移验证。

## 1. 仓库与工作树

| 项目 | 当前值 |
|---|---|
| 仓库 | `/Users/Zhuanz1/Desktop/privateProjects/ssh/workspace/new-api-v0.13.2` |
| 分支 | `dev/skye-v0.13.2-performance` |
| HEAD | `feec1e74b1961286cdfd53e6a5397b442c9fa4c3` |
| HEAD 描述 | `chore: bump version to v0.13.2-skye.7-52405375` |
| 远端 | `origin ssh://git@ssh.github.com:443/ASL-mj/new-api.git` |
| 技术栈 | Go 1.22+、Gin、GORM、SQLite/MySQL/PostgreSQL、React 18、Vite、Semi UI、Bun |

当前工作树只有文档修改，尚未提交：

```text
M  .gitignore
?? docs/superpowers/plans/2026-09-06-upstream-capability-upgrade-rollout.md
?? docs/superpowers/specs/2026-09-06-upstream-capability-upgrade-requirements.md
?? docs/superpowers/specs/2026-09-07-upstream-capability-s0-s2-implementation-contract.md
?? docs/superpowers/handoffs/2026-09-07-upstream-capability-upgrade-ai-handoff.md
```

`.gitignore` 的改动仅为将当前排期文件从通用 `plans` 忽略规则中精确放行。保留该例外，确保排期可纳入 Git；不要移除其他 `plans` 忽略策略。

## 2. 先读这些文档

阅读顺序固定如下：

1. [`AGENTS.md`](../../../AGENTS.md)：项目约定，尤其是三数据库兼容、`common/json.go`、可选请求字段和计费表达式规则。
2. [上游能力升级需求基线](../specs/2026-09-06-upstream-capability-upgrade-requirements.md)：全量能力范围 A-F 与边界。
3. [上游能力升级实施排期](../plans/2026-09-06-upstream-capability-upgrade-rollout.md)：阶段顺序、发布门和回滚。
4. [S0-S2 实施契约](../specs/2026-09-07-upstream-capability-s0-s2-implementation-contract.md)：S0-S2 的字段、H3 协议、结算、测试和灰度细节；与前两份文档有歧义时，以该文件为准。
5. `pkg/billingexpr/expr.md`：任何涉及计费的代码修改前必须阅读。H3 首期不扩展其 Token 参数环境。

## 3. 已确认需求与交付顺序

```text
S0 基线、迁移、提交持久化
  -> S1 轮询失败状态机与终态账务幂等
  -> S2 MiniMax-H3 视频任务闭环
  -> S3 模型解析和 gpt-6-astra 计费
  -> S4 GLM/Ollama Responses 与流式终态
  -> S5 使用日志终态额度和角色投影
  -> S6 JS 插件平台
```

首个可发布包只包含 S0-S2，预计 11 个工作日。当前下一位 AI 的实施范围只到 **S0**；S1/S2 代码在 S0 发布门通过后再开始。

## 4. 已确认的设计决策

### 4.1 UI 与本地定制

- 前端保持现有 React 18 + Vite + Semi UI 和 Skye.7 风格；官方新版前端不是母版。
- 不回退本地渠道 affinity、性能设置、使用日志详情、菜单和其他 fork 定制。
- 首期只在现有任务详情、使用日志详情中增补字段，不建设新视频工作台。

### 4.2 任务状态与轮询

- 对外 `TaskStatus` 保持既有 `SUBMITTED`、`QUEUED`、`IN_PROGRESS`、`SUCCESS`、`FAILURE`。
- `poll_exhausted` 是失败分类，不是新对外状态；第 20 次连续可重试失败应写为 `FAILURE + poll_exhausted`。
- 使用 `Task.UpdateWithStatus` 取得终态所有权，但 CAS 不能单独保证账户余额、渠道账本和日志的幂等。
- 默认配置：`TASK_POLL_MAX_FAILURES=20`、基础退避 10 秒、最大退避 300 秒。

### 4.3 终态账务

- S1 增加 `task_settlements`，以 `task_id` 唯一索引保证每个任务只有一个终态账务意图。
- Task 的新字段服务于轮询和展示；结算表才是账务恢复锚点。
- 不在 `tasks` 的字符串空值字段上添加唯一 `settlement_key`。
- 账户余额、订阅余额、渠道标准口径和结算状态需要处于同一个数据库事务；令牌 Redis 缓存属于派生状态，需要可重放对账。
- 日志失败不逆转已经提交的账务，应通过 outbox 或重试机制补写。

### 4.4 H3 与视频计费

- 首期在现有 Go `relay/channel/task/hailuo/` adaptor 内实现 H3；完整 JS 插件平台后置。
- H3 提交：`POST {base_url}/v2/video_generation`；查询：`GET {base_url}/v2/query/video_generation/{upstream_task_id}`。
- H3 使用多模态 `content`；普通 `image/images` 表示首尾帧，参考图片只通过 `metadata.content[]` 中 `role=reference_image` 表达。
- H3 支持整数 4-15 秒、`768P`/`2K`、图片/视频/音频参考；完整字段映射、边界和状态映射见 S0-S2 实施契约。
- H3 首期走 `TaskBillingContext.TaskPricingSnapshot` + `OtherRatios`，冻结提交时价格规则并在完成时按快照重算。
- `pkg/billingexpr.TokenParams` 是 Token 计费模型，首期不新增视频秒数、分辨率或媒体参考变量。

### 4.5 提交持久化

当前代码在上游提交成功后才调用 `task.Insert`。H3 启用前必须消除以下窗口：

```text
上游已创建任务 -> 本地进程停止 -> 本地没有 Task -> 无法轮询、结算或退款
```

S0 的目标顺序：先生成公开 `task_...`、冻结价格快照、写入 `TaskStatusNotStart` 任务记录；再提交上游；成功后绑定 `UpstreamTaskID` 并推进为 `SUBMITTED`。第 3、4 步之间中断时，保留 `NOT_START` 记录供恢复作业诊断。

## 5. 已验证的现有代码位置

| 责任 | 文件与符号 |
|---|---|
| Task 模型、私有账务快照、CAS | `model/task.go`：`Task`、`TaskPrivateData`、`TaskBillingContext`、`UpdateWithStatus` |
| Task 迁移 | `model/main.go`：`migrateDB`、`migrateDBFast` |
| 轮询、终态推进 | `service/task_polling.go`：`TaskPollingLoop`、`updateVideoSingleTask`、`settleTaskBillingOnComplete` |
| 退款与差额 | `service/task_billing.go`：`RefundTaskQuota`、`RecalculateTaskQuota`、`RecalculateTaskQuotaByTokens` |
| 提交后的结算、日志、Task 插入 | `controller/relay.go`：成功分支约第 663 行 |
| 对外任务请求/查询数据 | `relay/common/relay_info.go`：`TaskSubmitReq`、`TaskInfo` |
| Hailuo 旧版 adaptor | `relay/channel/task/hailuo/adaptor.go`、`models.go`、`constants.go` |
| Task 计费预扣与 `OtherRatios` | `relay/relay_task.go`、`relay/channel/adapter.go` |
| 现有任务测试 | `model/task_cas_test.go`、`service/task_polling_test.go`、`service/task_billing_test.go` |

上游 H3 行为已从 `v1.0.0-rc.33` 的 `plugins/tasks/hailuo/plugin.js` 对照。若本地临时参考检出仍存在，其路径为 `/tmp/new-api-rc33.qimETO/src`；实现前仍应按当前上游版本和 MiniMax 文档核对 H3 的字段与限制。

## 6. S0 工作清单

按以下顺序执行；每一步先读相关现有实现和测试，再最小化修改。

1. 为 Task 写回归测试，锁定旧 Hailuo 成功、失败、超时、退款和 CAS 行为。
2. 设计并创建 `TaskSettlement` GORM 模型和测试，验证 `task_id` 唯一性、存量 Task 不自动创建结算记录。
3. 为 Task 添加轮询字段及 `SettlementStatus`，同时接入普通与快速迁移路径。
4. 为 SQLite、MySQL、PostgreSQL 完成新库与存量表升级测试。
5. 增加关闭状态的 `TASK_POLLING_V2_ENABLED` 和轮询参数读取，开关关闭时完全保留旧扫描行为。
6. 重构提交持久化顺序：至少让 H3 可以在上游提交前写入 `NOT_START` Task、渠道身份和价格快照。
7. 为“上游成功后、本地绑定 upstream task ID 前中断”编写恢复测试或可复现的持久化夹具。
8. 完成 S0 发布门后，再开始 S1；不要在 S0 同时实现完整 H3 请求转换或 JS 插件运行时。

## 7. S0 明确不做的事项

- 不实现 H3 `/v2` 请求 DTO、视频参考、轮询解析或价格计算细节。
- 不修改 GLM/Ollama Responses、RelayKit、模型大小写或 `gpt-6-astra`。
- 不建设插件表、JS 沙箱、动态路由或插件管理页面。
- 不重构现有前端或替换 Semi UI。
- 不对历史终态 Task 执行退款、差额重算或结算记录回填。

## 8. 验证与完成标准

S0 完成前，至少运行并阅读结果：

```bash
go test ./model -run 'Task|Settlement' -v
go test ./service -run 'Task|Billing|Poll' -v
go test ./relay/helper -v
git diff --check
```

数据库测试需覆盖 SQLite，并在可用的 MySQL、PostgreSQL 环境各执行一次。构建或测试因依赖网络、数据库服务未启动等原因未运行时，交接中必须记录实际阻塞原因，不将静态检查表述为运行时验证。

S0 完成定义：

1. 加法迁移在三种数据库成立，存量任务保持可读。
2. `task_settlements` 可以唯一锁定一条终态账务意图。
3. 新提交持久化顺序关闭了上游任务创建后本地 Task 丢失的恢复空洞。
4. 新开关关闭时，旧任务轮询与账务行为回归通过。
5. 未改动任何既有前端样式或非 S0 功能。

## 9. 后续待办与风险

- S1 的最大风险是现有余额、订阅、令牌和渠道口径跨存储系统的原子性；先做事务边界设计与故障恢复测试，再改退款入口。
- S2 的最大风险是 H3 上游媒体 URL 可访问性、实际 usage 缺失或超出约束、结果 URL 时效性；必须使用固定 HTTP fixture 和测试渠道双重验证。
- S5 仍需完成 `Log.Ip -> API -> UI` 的完整链路核对；不要仅改前端展示掩盖日志写入缺失。
- 当前文档与 `.gitignore` 均未提交。提交时将文档和 `.gitignore` 作为独立 docs 提交，不与 S0 代码混合。

## 10. 可直接交给下一位 AI 的启动提示

```text
在 /Users/Zhuanz1/Desktop/privateProjects/ssh/workspace/new-api-v0.13.2 的
dev/skye-v0.13.2-performance 分支执行 NewAPI 上游能力升级 S0。

先阅读：AGENTS.md、docs/superpowers/specs/2026-09-06-upstream-capability-upgrade-requirements.md、
docs/superpowers/plans/2026-09-06-upstream-capability-upgrade-rollout.md、
docs/superpowers/specs/2026-09-07-upstream-capability-s0-s2-implementation-contract.md、
pkg/billingexpr/expr.md。

只实施 S0：Task 加法迁移、task_settlements 幂等记录、特性开关、旧任务回归夹具、
H3 提交前 Task 持久化和恢复边界。保持现有 React/Semi UI 风格，暂不做 S1/S2/S3-S6。
不回退工作树中现有文档或本地 fork 定制。每项改动先写/补测试，完成后运行目标 Go 测试和 git diff --check，
报告未运行验证的真实原因。不要提交 Git，除非用户明确要求。
```
