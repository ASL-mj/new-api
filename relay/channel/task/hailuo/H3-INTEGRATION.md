# MiniMax H3 Integration

## Verified Protocol Source

Read on 2026-09-07:

- https://platform.minimaxi.com/docs/llms.txt
- https://platform.minimaxi.com/docs/api-reference/video/generation/api/v2-video-generation.json
- Local reference: `/tmp/new-api-rc33.qimETO/src/plugins/tasks/hailuo/plugin.js`

The official schema reports OpenAPI 3.1.0, API version 2.0.0 and confirms
`POST /v2/video_generation`, `GET /v2/query/video_generation/{task_id}`, bearer
authentication, content URL objects, task status wrapper and usage fields.
It is a mutable document, already containing H3-Max and additional token/audio
usage fields. Its description still says Hailuo-03; the local reference has no
Git metadata and an empty VERSION. Neither is a pinned release guarantee.
Only the approved MiniMax-H3 subset is implemented. H3-Max, regeneration,
Context-IR, token pricing and aliases without explicit model mapping are excluded.
No authenticated provider requests or paid generation were used for verification.

## Integration API

- `TaskAdaptor.PrepareTaskBilling(c, info) (bool, error)` runs after channel model
  mapping, before precharge. A handled H3 request supplies the exact final
  `PriceData.Quota`, display ratios and `TaskPricingSnapshot`; do not multiply
  those ratios into the quota a second time.
- Shared snapshot type: `types.TaskPricingSnapshot`, referenced from
  `TaskRelayInfo` and `TaskBillingContext`. The persistence owner deep-copies it.
- Poll usage: `TaskInfo.TaskUsage *types.H3UsageOverrides`. Merge it into the
  submitted facts before calling snapshot `Quota` and `StandardQuota`.
- `OtherRatios() (map[string]float64, error)` is display/compatibility data only.
- H3 `DoResponse` is parse-only. Write the public task ID response only after
  durable upstream binding and submission bookkeeping commit.
- `H3SubmissionOutcomeUnknownError.SubmissionOutcomeUnknown()` and task error
  code `h3_submit_outcome_unknown` identify ambiguous outcomes. Preserve the
  local pending task and reconcile; no blind retry or refund. Explicit 4xx
  rejections use sanitized `h3_upstream_error`. Pass H3 non-2xx responses to
  `DoResponse` instead of the generic raw-body error handler.
- Poll embedded HTTP errors surface as `TaskInfo.Code`; missing/unknown task
  structures return errors. Failed/cancelled tasks are explicit terminal facts.

## Configuration

`minimax_h3_pricing.rules` is registered, blank by default. The option controller
must invoke `task_setting.ValidateH3PricingRules` before persistence. Read and
update helpers are `GetH3PricingRules` and `UpdateH3PricingRules`.

Required JSON fields: `version`, positive integer `base_quota`, positive
`duration_base_seconds`, `resolution_ratios` with exactly `768P` and `2K`, and
explicit non-negative `image_reference_ratio_per_image`,
`video_reference_ratio_per_second`, `audio_reference_ratio_per_audio`.
There are no built-in prices. Test numbers are arithmetic fixtures only.

Pricing uses duration/base_seconds, the resolution ratio and `1 + quantity*rate`
for each reference category, rounding once after the group ratio. The snapshot
freezes rules, hash, version, base quota and submitted facts. Fractional actual
seconds are supported. Explicit zero group ratios are supported. Invalid rules,
overflow, missing rates, and zero-rounded standard prices fail closed.

New submission requires `MINIMAX_H3_ENABLED=true` and the parent's
`TaskPollingV2EnabledForChannel(channel_id)` gate. Neither gate applies to fetch
or settlement of previously submitted tasks. Optional callbacks require an
exact URL in the JSON-array environment variable `MINIMAX_H3_CALLBACK_URLS` and
must pass the fetch policy. Callback verification/receiver behavior is not added.

## Validation And Remaining Boundaries

Requests are limited to 1 MiB; text to 7000 characters. Unknown metadata/types,
null fields, conflicting media sources, invalid counts/roles, unsupported
resolutions, mixed frame/reference content and invalid ratios are rejected.
The generic image aliases represent one first frame; last frames are explicit.
Media must be HTTP(S), without credentials or fragments. Base64/mm_file inputs
need a separate controlled short-lived URL conversion; this adaptor rejects them.

Downloads reuse existing SSRF/fetch settings and revalidate redirects. They send
no provider credentials, with 10-second request and 30-second aggregate deadlines,
three redirect hops, per-media limits (30/50/15 MiB image/video/audio) and a
128 MiB aggregate budget. JPEG/PNG/WEBP dimensions, MP4/MOV header duration and
dimensions, and WAV/MP3 durations are checked. Video and audio reference totals
are each limited to 15 seconds; billing reserves 15 video seconds until completion.

HEIC/HEIF are not accepted by available image decoders. Video codec/frame-rate
and full media decodability are not verified by the metadata probe. Remote media
can change after validation; upstream fetch reachability is not proven by a local
probe. DNS rebinding protection remains that of the existing fetch helper/client.
These restrictions/gaps must remain visible in release acceptance. Fixture tests
do not prove paid API compatibility, production operation, or cross-DB acceptance.
