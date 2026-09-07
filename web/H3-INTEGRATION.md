# S1/S2 Dashboard Integration

Frontend ownership: `web/**`. Parent accepted this dashboard contract for the
parallel backend implementation; it is not evidence of a deployed API.

## Submission

- `POST /api/task/video`, existing session auth and `New-API-User` header.
- No channel key, upstream task ID, or raw token is accepted or stored by the UI.
- Body: `{token_id:123,model:"MiniMax-H3",prompt,duration:5,size:"768P",metadata:{ratio:"16:9",content:[...]}}`.
- Required `token_id` is selected from the current user's paginated
  `GET /api/token/?p=1&size=20`. UI retains only ID/name/disabled state,
  never fetches the token-key endpoint. Backend must verify token ownership,
  status, group, limits, and apply normal token billing without exposing its key.
- Model choices use `GET /api/task/video/models?token_id=...`; the backend
  resolves the selected token's group, model limits, enabled abilities, and
  enabled MiniMax video channels. The UI refreshes this list after every token
  change.
- Content starts with `{type:"text",text:prompt}`. Media use
  `{type:"image_url",role:"first_frame"|"last_frame"|"reference_image",image_url:{url}}`,
  `{type:"video_url",role:"reference_video",video_url:{url}}`, and
  `{type:"audio_url",role:"reference_audio",audio_url:{url}}`.
- Success: `{success:true,message:"",data:{task_id:"task_..."}}`.
- Failure: `{success:false,message:"sanitized explanation"}` or HTTP error with
  that envelope. Enforce feature gate, full pricing, access group, SSRF/media
  validation, and normal task billing on the server. Unknown submission outcome
  is not automatically retried by the UI.

## Task Views

Existing `GET /api/task/self` and admin `GET /api/task/` pagination remain unchanged.
Add role-filtered fields to each item; numeric quota fields are integer quota
units, not money strings. Missing/null means unavailable, never zero.

- User: existing task ID/status/progress/result URL/failure reason and
  `model`, `pre_consumed_quota`, `actual_quota`, `quota_delta`,
  `settlement_status` (`pending|applying|applied|failed|legacy`),
  `settlement_kind` (`settle|refund`), `settled_at`.
- Admin additionally: `poll_failures`, `last_poll_error_class`,
  `last_poll_error_message` (sanitized), `last_poll_at`, `next_poll_at`.
- Root additionally: `settlement_id`, `pricing_version`.
- Applied refund final amount is zero; refunded amount is `-quota_delta`.
  Pending/failed settlement must not be shown as completed billing.
- UI selects an allowlist of fields, never displays a raw Task/private data blob.
- Usage-log `other.task_billing` should contain the same billing fields and
  public `task_id`; admin/root additions must be filtered server-side as well.

## Pricing Configuration

Existing root `GET/PUT /api/option/`, key `minimax_h3_pricing.rules`.
Value is a JSON object (serialized string in the option API):

```json
{
  "version": "h3-v1",
  "base_quota": 1000,
  "duration_base_seconds": 5,
  "resolution_ratios": { "768P": 1, "2K": 2 },
  "image_reference_ratio_per_image": 0.1,
  "video_reference_ratio_per_second": 0.1,
  "audio_reference_ratio_per_audio": 0.1
}
```

Numbers above are schema examples, NOT default prices. UI starts blank and
requires every value. Proposed ratios: duration/base_seconds; selected resolution;
`1 + count * per_image`; `1 + seconds * per_second`; `1 + count * per_audio`.
Backend must validate, freeze the rule/version/hash, and apply group ratios and
actual usage. Parent accepted this schema. Missing option
key disables save, preventing apparent success against an unregistered option.

## Manual Acceptance

Use only the local integrated backend with fixtures; no production submissions.
In existing Tasks, open Generate Video, submit text, first/last frame, and reference
media cases. Check validation, submission error/double-click, task ID, refresh,
success result preview, pending billing, applied delta/refund, and user/admin/root
field separation. In existing pricing settings, verify required values, load/save
failure, and reloaded persisted rules. Recheck legacy Hailuo and Suno previews.

Static checks do not establish live provider or browser acceptance.
