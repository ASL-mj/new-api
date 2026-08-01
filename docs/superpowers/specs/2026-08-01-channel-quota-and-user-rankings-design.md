# 渠道限额、渠道用量与用户排行设计

## 1. 目标

在 NewAPI v0.13.2 的现有渠道管理和数据看板基础上增加两组能力：

1. 渠道用量与限额：支持渠道整体限额、多密钥独立限额、自动禁用、手动重置和三列用量展示。
2. 用户排行：在现有用户额度消耗排行上增加 Token 消耗排行和调用次数排行。

本设计不包含周期自动重置。任何达到限额的渠道或密钥，重置用量后仍保持禁用，必须由管理员再次手动启用。

## 2. 已确认的产品规则

### 2.1 限额模式

渠道限额模式固定为：

- `none`：不限额。
- `channel`：仅渠道整体限额。
- `key`：仅多密钥独立限额。
- `both`：渠道整体限额和多密钥独立限额同时生效。

单密钥渠道只展示 `none` 和 `channel`。多密钥渠道展示全部四种模式。

限额为 `0` 表示无限，不使用 `NULL` 表示业务语义。接口可以接受空值，但保存时统一归一化为 `0`。

### 2.2 越限与启用规则

- 渠道整体本轮用量达到渠道限额后，渠道状态改为自动禁用，并从渠道调度能力中移除。
- 单个密钥达到密钥限额后，只自动禁用该密钥。
- 多密钥渠道全部密钥不可用时，渠道状态改为自动禁用。
- `both` 模式下任一层先达到限额，就执行对应层级的禁用。
- 管理员重置渠道用量或密钥用量时只清零计数，不修改禁用状态。
- 管理员尝试启用未重置且仍然超限的渠道或密钥时，接口返回明确错误，不允许绕过限额。
- 管理员将限额改为 `0` 时，不自动启用；仍需手动点击启用。
- 限额自动禁用不受现有 `AutoBan` 开关控制。`AutoBan` 继续只控制上游请求错误造成的自动禁用。

### 2.3 渠道列表展示

渠道列表将现有用量相关列调整为三列：

| 列名 | 内容 |
|---|---|
| 今日 / 30 日 | 渠道今日费用与最近 30 天费用，例如 `$12.36 / $286.50` |
| 已用 / 限额 | 本轮渠道限额用量与渠道整体限额，例如 `$426.80 / $500`；不限额显示 `$426.80 / ∞` |
| 上游余额 | 现有渠道余额，例如 `$63.28`；无法查询时显示 `—` |

“已用”使用新的可重置 `quota_limit_used`，不是历史累计 `used_quota`。原字段 `used_quota` 保留，用于兼容现有统计和历史累计语义。

多密钥的独立用量、限额、状态和重置操作放入现有多密钥管理弹窗，不扩展渠道主表宽度。

### 2.4 用户排行

现有“用户消耗排行”升级为“用户排行”，在图表标题区域提供三个指标切换：

- 额度消耗
- Token 消耗
- 调用次数

模型调用次数排行保持独立，不改变。用户消耗趋势保持现状，本期不增加 Token 趋势和调用次数趋势。

## 3. Sub2API 调研结论

### 3.1 值得复用的设计

Sub2API 的限额可靠性来自请求结算热路径，而不是定时扫描：

1. 使用数据库原子递增用量，避免并发请求产生丢失更新。
2. 在同一次数据库更新中返回递增后的用量、限额和状态。
3. 使用 `new_used >= limit && previous_used < limit` 判断首次跨越限额，只触发一次状态和缓存更新。
4. API Key 达限时在同一条 SQL 中原子修改状态，下一次调度立即排除。
5. 调度候选过滤再次检查限额，即使缓存同步有短暂延迟，也不会继续分配已超限对象。
6. 今日统计使用批量接口，不为列表每一行单独发请求。
7. 前端只在相关列可见时加载统计，并用请求序号避免旧响应覆盖新页数据。

### 3.2 NewAPI 不能照搬的部分

- Sub2API 使用 PostgreSQL `RETURNING`、JSONB 和专属 SQL；NewAPI 必须兼容 MySQL、SQLite、PostgreSQL。
- Sub2API 支持日、周、周期重置；本项目明确不做周期重置。
- Sub2API 重置后会重新进入调度；本项目必须保持禁用，等待管理员手动启用。
- NewAPI 多密钥当前状态按数组下标保存，密钥重排会改变下标；限额账本不能只用下标作为身份。

## 4. 数据模型

### 4.1 `channels` 新字段

在 `model.Channel` 增加：

```go
QuotaLimitMode    string `json:"quota_limit_mode" gorm:"type:varchar(16);default:'none';index"`
QuotaLimit        int64  `json:"quota_limit" gorm:"bigint;default:0"`
QuotaLimitUsed    int64  `json:"quota_limit_used" gorm:"bigint;default:0"`
QuotaLimitResetAt int64  `json:"quota_limit_reset_at" gorm:"bigint;default:0"`
```

金额继续使用 NewAPI 的内部 quota 整数单位，避免浮点累计误差。前端通过现有 `renderQuota` 转为美元展示。

### 4.2 `channel_key_usages`

新增模型 `ChannelKeyUsage`：

```go
type ChannelKeyUsage struct {
    Id                int    `json:"id"`
    ChannelId         int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_key_fingerprint,priority:1;index"`
    KeyFingerprint    string `json:"key_fingerprint" gorm:"size:64;uniqueIndex:idx_channel_key_fingerprint,priority:2"`
    KeyIndex          int    `json:"key_index" gorm:"default:0"`
    KeyMask           string `json:"key_mask" gorm:"size:64"`
    QuotaLimit        int64  `json:"quota_limit" gorm:"bigint;default:0"`
    QuotaLimitUsed    int64  `json:"quota_limit_used" gorm:"bigint;default:0"`
    QuotaLimitResetAt int64  `json:"quota_limit_reset_at" gorm:"bigint;default:0"`
    Status            int    `json:"status" gorm:"default:1;index"`
    DisabledReason    string `json:"disabled_reason" gorm:"size:255"`
    DisabledAt        int64  `json:"disabled_at" gorm:"bigint;default:0"`
    CreatedAt         int64  `json:"created_at" gorm:"bigint"`
    UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}
```

`KeyFingerprint` 使用服务端固定盐值生成 HMAC-SHA256，不保存明文密钥，也不使用可被离线枚举的裸 SHA256。固定盐值来源优先使用现有加密密钥配置；若项目没有稳定密钥，则新增安装级随机 secret 并持久化到 Option。

`KeyIndex` 只用于展示和兼容现有多密钥状态映射，不能作为唯一身份。保存或读取渠道时按指纹重新映射当前下标，从而支持密钥重排。

### 4.3 `channel_usage_daily`

新增每日聚合表：

```go
type ChannelUsageDaily struct {
    Id             int    `json:"id"`
    ChannelId      int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_usage_day,priority:1;index"`
    KeyFingerprint string `json:"key_fingerprint" gorm:"size:64;uniqueIndex:idx_channel_usage_day,priority:2"`
    UsageDate      string `json:"usage_date" gorm:"size:10;uniqueIndex:idx_channel_usage_day,priority:3;index"`
    Quota          int64  `json:"quota" gorm:"bigint;default:0"`
    TokenUsed      int64  `json:"token_used" gorm:"bigint;default:0"`
    RequestCount   int64  `json:"request_count" gorm:"bigint;default:0"`
    UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}
```

渠道汇总行使用空字符串 `key_fingerprint=''`。每次成功结算同时更新渠道汇总行和实际密钥行。这样主列表只查询渠道汇总，密钥弹窗再查询密钥维度。

日期使用系统数据看板时区计算后的 `YYYY-MM-DD`，不直接依赖数据库服务器时区。

## 5. 结算与并发架构

### 5.1 统一结算入口

新增 `service.RecordChannelUsage(relayInfo, quota, tokenUsed, requestCount)`，替换当前各结算路径直接调用 `model.UpdateChannelUsedQuota()` 的方式。

该服务负责：

1. 更新历史累计 `channels.used_quota`。
2. 更新渠道本轮 `quota_limit_used`。
3. 更新实际密钥的 `quota_limit_used`。
4. 增量写入 `channel_usage_daily` 的渠道汇总和密钥明细。
5. 识别首次跨越渠道或密钥限额。
6. 禁用越限对象、写系统事件并同步缓存。

所有文本、实时流、任务、违规费和 Midjourney 等现有计费入口都必须接入，避免出现部分请求不计入限额。

### 5.2 跨数据库原子性

不使用数据库方言专属 `RETURNING`。统一采用 GORM 事务内的两步原子操作：

```sql
UPDATE channels
SET used_quota = used_quota + ?,
    quota_limit_used = quota_limit_used + ?
WHERE id = ?;

UPDATE channels
SET status = auto_disabled_status
WHERE id = ?
  AND status = enabled_status
  AND quota_limit > 0
  AND quota_limit_used >= quota_limit;
```

第二条状态更新只有一个并发事务能得到 `RowsAffected == 1`，该结果就是唯一的首次越限标志。事务内再读取最新用量和状态用于响应。密钥表采用同样的累加与条件禁用流程。

为了兼容 SQLite，测试数据库需要开启 busy timeout；生产推荐 MySQL 或 PostgreSQL。所有涉及同一渠道的更新保持固定顺序：渠道行、密钥行、渠道日汇总、密钥日汇总，降低死锁概率。

### 5.3 请求超额边界

本功能采用“结算后禁用”语义：导致额度跨越的当前请求允许完成，后续请求不再调度到该渠道或密钥。这与现有 NewAPI 后结算模型一致，也避免预估 Token 与实际 Token 不一致。

并发情况下可能有多个已在途请求同时完成，最终用量可以超过限额，但原子累计不会丢失；首次跨越事件只记录一次。要实现绝对硬截断必须引入预占额度和回滚，超出本期范围。

### 5.4 调度前双重校验

除状态禁用外，在以下位置增加限额过滤：

- 渠道候选选择：渠道模式为 `channel` 或 `both` 且 `quota_limit_used >= quota_limit` 时不可选。
- 多密钥选择：密钥模式为 `key` 或 `both` 且密钥用量达到限额时不可选。

该校验必须同时覆盖数据库模式和内存缓存模式。达到限额后立即更新当前节点缓存，并调用现有能力状态更新；其他节点通过现有定时 `InitChannelCache()` 最终同步。若部署多节点且需要秒级一致性，后续可增加 Redis Pub/Sub，本期不引入新依赖。

## 6. 管理接口

新增管理员接口：

```text
GET  /api/channel/usage/batch?ids=1,2,3
GET  /api/channel/:id/key-usages
POST /api/channel/:id/quota/reset
POST /api/channel/:id/keys/:fingerprint/quota/reset
PUT  /api/channel/:id/keys/:fingerprint/quota
```

现有渠道新增/编辑接口增加：

```json
{
  "quota_limit_mode": "both",
  "quota_limit": 500000
}
```

批量用量接口返回当前页全部渠道的数据：

```json
{
  "1": {
    "today_quota": 123600,
    "last_30d_quota": 2865000,
    "quota_limit_used": 4268000,
    "quota_limit": 5000000,
    "balance": 63.28
  }
}
```

启用渠道和启用密钥的现有操作增加限额前置校验。错误消息必须区分：

- `渠道本轮用量已达到限额，请先重置额度`
- `该密钥本轮用量已达到限额，请先重置额度`

## 7. 前端设计

### 7.1 渠道编辑

在 `EditChannelModal` 增加“用量限额”区域：

- 限额模式下拉框。
- 渠道整体限额输入框，仅 `channel`、`both` 展示。
- 说明文本：`0 表示无限；达到限额后自动禁用，重置后仍需手动启用。`

### 7.2 多密钥管理

`MultiKeyManageModal` 每行增加：

- 本轮已用。
- 密钥限额，`0` 显示 `∞`。
- 使用率进度条。
- 编辑限额。
- 重置用量。
- 状态和禁用原因。

密钥被重新排序时，前端继续提交当前 key index；后端将其解析为指纹并操作稳定记录。接口响应只暴露指纹和掩码，不返回完整密钥。

### 7.3 渠道列表加载

列表主体加载完成后，用当前页渠道 ID 发一次批量统计请求。仅当三列中至少一列可见时请求。使用递增请求序号，翻页或搜索后忽略旧响应。

统计请求失败不影响渠道主列表，只在单元格显示 `—` 并允许手动刷新。

### 7.4 用户排行

`ChartsPanel` 的用户排行页签内增加 Semi UI 分段控件。选择状态保存在组件状态，不持久化到服务器。默认选择“额度消耗”。

`processUserData` 同时输出每个用户的 `Quota`、`TokenUsed`、`Count` 聚合。图表根据当前指标排序、格式化坐标轴和总计：

- 额度：使用 `renderQuota`。
- Token：使用 `renderNumber`。
- 调用次数：使用 `renderNumber`。

## 8. 历史数据与上线

- `quota_limit_used` 上线初始值为 `0`，不会用历史 `used_quota` 自动填充。
- `channel_usage_daily` 上线后实时积累。
- 可提供一次性后台命令，从 `logs` 按渠道回填最近 30 天渠道汇总数据。
- 不回填历史密钥级数据，因为旧日志中的 `multi_key_index` 可能在密钥重排后失真。
- 回填过程分批执行，不能阻塞服务启动，也不能在自动迁移期间扫描大表。

## 9. 测试与验收

### 9.1 后端

- 10 个并发结算不会丢失渠道或密钥用量。
- 只有首次跨越限额产生自动禁用事件。
- 单密钥越限只禁用该密钥。
- 所有密钥禁用后渠道自动禁用。
- 渠道整体越限直接禁用渠道。
- 重置只清零，不启用。
- 未重置前启用返回明确错误。
- 限额改为无限后仍不自动启用。
- MySQL、SQLite、PostgreSQL 方言不使用专属 SQL。
- 批量统计在无数据时返回零值，在未知 ID 时不报整批错误。

### 9.2 前端

- 三列正确显示今日、30 日、本轮用量、无限限额和余额。
- 隐藏统计列时不请求批量统计。
- 快速翻页时旧响应不会覆盖当前页。
- 多密钥弹窗不会泄露完整密钥。
- 用户排行三个指标排序和总计正确。

## 10. 明确不做

- 日、周、月或自定义周期自动重置。
- 达限前额度预占和失败回滚。
- 重置后自动启用。
- 将多个密钥的上游余额盲目相加。
- 历史密钥级用量回填。
- 本期新增 Redis Pub/Sub 多节点失效通知。
