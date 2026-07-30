# NewAPI v0.13.2 Model Performance Frontend Design

## 1. Purpose

Port the latest NewAPI model-performance experience to the `v0.13.2` model pricing page without upgrading the rest of the application.

The feature must preserve the existing Semi UI visual language and all current model-detail content while adding:

- Average TPS.
- Average total latency.
- Success rate.
- Per-group TPS, TTFT, total latency, and success rate.
- TTFT trend.
- Availability trend.
- Explicit loading, empty, sparse-sample, and error states.

The implementation remains on `dev/skye-v0.13.2-performance` and does not merge the latest NewAPI frontend or framework stack.

## 2. Approved Decisions

- Use a tabbed model-detail SideSheet.
- Desktop SideSheet width is approximately `760px`; mobile width is `100%`.
- Tabs are `Overview`, `Performance`, and `API`.
- Time ranges are `1h`, `24h`, and `7d`; the default is `24h`.
- Overall metrics are calculated from raw counters across all active groups, not by averaging group percentages or group averages.
- Fewer than 10 effective requests produces a sparse-sample warning while retaining the real values.
- Mobile retains the group table and makes it horizontally scrollable.
- Existing model detail information, dynamic billing, group prices, and endpoint information must not be removed.

## 3. Information Architecture

The model header remains fixed above the tab list and contains the model icon, model name, provider metadata, and close action.

### 3.1 Overview Tab

The Overview tab contains the existing model-detail content:

1. Basic model information.
2. Model capabilities and metadata already supported by `v0.13.2`.
3. Group pricing.
4. Dynamic pricing breakdown when `billing_mode` and `billing_expr` require it.

The migration may reorganize these existing components inside the tab container, but it must not change their calculations or visible data.

### 3.2 Performance Tab

The Performance tab uses this reading order:

1. Page-level time-range selector.
2. Overall metric cards.
3. Per-group performance table.
4. TTFT trend chart.
5. Availability chart.

The three overall cards are:

- Average TPS.
- Average total latency.
- Success rate.

The group table columns are:

- Group.
- TPS.
- Average TTFT.
- Average total latency.
- Success rate.

The latency chart plots average TTFT by time bucket. The availability chart plots request success rate by time bucket. The UI must not label TTFT as total latency or use the two values interchangeably.

### 3.3 API Tab

The API tab contains the existing API endpoint and supported request-path information. The first release does not add a new playground or request builder.

## 4. Component Design

### 4.1 SideSheet Owner

`ModelDetailSideSheet` remains the entry point and owns:

- Visibility and close behavior.
- Current model identity.
- Active tab.
- Desktop and mobile width.
- Reset behavior when the selected model changes.

Opening a different model resets the active tab to Overview and resets the performance time range to `24h`.

### 4.2 Tab Components

The model detail is divided into three bounded views:

- `ModelOverviewTab`: composes existing basic-information and pricing components.
- `ModelPerformancePanel`: owns the performance request lifecycle and state rendering.
- `ModelApiTab`: composes the existing endpoint component.

The performance view is divided further:

- `ModelPerformanceStats`: renders the three overall metric cards.
- `ModelPerformanceGroupTable`: renders one row per active group.
- `ModelPerformanceCharts`: renders the TTFT and availability series.
- `useModelPerformance`: performs requests, cancels stale work, caches successful responses, and normalizes the API result.

Formatting logic belongs in a small performance helper module rather than inside the visual components. It formats throughput, latency, percentage values, and unavailable values consistently.

## 5. Request Lifecycle

The Performance tab is lazy. Opening the SideSheet or viewing Overview does not request performance data. The first switch to Performance requests:

```text
GET /api/perf-metrics?model=<encoded-model-name>&hours=<1|24|168>
```

Request behavior:

- Cache key: model name plus selected hour range.
- Successful-result freshness window: 60 seconds.
- Changing the range requests the corresponding dataset.
- Returning to a previously loaded range within 60 seconds reuses the cached result.
- Changing the model or closing the SideSheet cancels or invalidates in-flight work.
- A stale response must never replace data for the current model and range.
- Retrying an error only reloads performance data and does not reset the other tabs.

## 6. API Contract Required by the Frontend

The frontend requires an overall aggregate calculated by the backend from raw counters. It must not derive the overall metrics by taking a simple average of group results.

```json
{
  "success": true,
  "data": {
    "model_name": "gpt-5.4",
    "hours": 24,
    "series_schema": "schema-version",
    "overall": {
      "request_count": 1264,
      "avg_tps": 42.36,
      "avg_ttft_ms": 820,
      "avg_latency_ms": 5100,
      "success_rate": 98.75,
      "series": [
        {
          "ts": 1785391200,
          "request_count": 58,
          "avg_tps": 44.12,
          "avg_ttft_ms": 790,
          "avg_latency_ms": 4900,
          "success_rate": 100
        }
      ]
    },
    "groups": [
      {
        "group": "default",
        "request_count": 1100,
        "avg_tps": 41.80,
        "avg_ttft_ms": 840,
        "avg_latency_ms": 5200,
        "success_rate": 98.50,
        "series": []
      }
    ]
  }
}
```

Backend formulas:

```text
average total latency = total_latency_ms / request_count
average TTFT          = ttft_sum_ms / ttft_count
success rate          = success_count / request_count * 100
TPS                   = output_tokens / (generation_ms / 1000)
```

The overall values use counters merged across every active group before applying these formulas. This prevents low-volume groups from having the same statistical weight as high-volume groups.

Only currently active groups are returned. The API must not expose user, token, channel, or individual-request data.

## 7. Time Range and Bucket Requirements

The user-facing ranges map as follows:

| Label | API value | Intended chart density |
|---|---:|---:|
| `1h` | `hours=1` | approximately 12 points |
| `24h` | `hours=24` | approximately 24 points |
| `7d` | `hours=168` | up to 168 points |

Supporting a meaningful `1h` chart requires persisted source buckets no larger than five minutes. An hourly source bucket would produce at most one point and does not satisfy this design.

The backend query layer is responsible for range-appropriate rollup:

- `1h`: five-minute points.
- `24h`: hourly points.
- `7d`: hourly points, capped at 168 points.

The frontend renders the returned series and does not perform time-bucket merging.

## 8. Visual Behavior

### 8.1 Desktop

- SideSheet width: approximately `760px`.
- Header and tabs remain visible above the scrollable tab content.
- Overall metrics use three equal columns.
- TTFT and availability charts may share a row when the available content width supports it.
- Cards and panels follow existing Semi UI border, spacing, typography, and radius conventions.

### 8.2 Mobile

- SideSheet width: `100%`.
- Overall metrics remain a stable three-column row with compact labels and values.
- The group table retains desktop columns and scrolls horizontally.
- A right-edge fade or equivalent affordance indicates horizontal scrolling.
- TTFT and availability charts stack vertically.
- No label, value, or table column may resize the overall page width.

## 9. UI States

### 9.1 Loading

Show stable skeleton placeholders matching the final metric and chart dimensions. Loading must not resize the SideSheet or shift the tab bar.

### 9.2 No Data

When `overall.request_count` is zero or the response has no effective groups:

- Hide metric cards, group table, and charts.
- Show `No performance data is available for this time range`.
- Explain that only real Relay requests are collected.
- Do not render misleading zero-valued metrics.

### 9.3 Sparse Sample

When `0 < overall.request_count < 10`:

- Render the real metrics and available charts.
- Show a restrained warning with the current effective request count.
- State that the values may fluctuate because the sample is small.

### 9.4 Error

When the API request fails:

- Show a performance-only error panel.
- Provide a `Retry` action.
- Preserve Overview and API tab usability.
- Do not replace an already displayed successful dataset with a transient refresh error; retain it and show a non-blocking refresh warning instead.

### 9.5 Invalid Values

Non-finite or non-positive TPS and latency values display an em dash. Percentages are clamped to the valid display range of 0 to 100.

## 10. Accessibility and Localization

- Tabs, retry, close, and time-range controls must be keyboard accessible.
- The active tab and active time range must be programmatically identifiable.
- Charts must have visible titles and text summaries; color cannot be the only success/failure signal.
- Success, warning, and error colors use existing Semi UI semantic tokens.
- All new visible strings are added to the localization files already maintained by `v0.13.2`.
- Numeric formatting follows the current locale while API values remain locale-independent numbers.

## 11. Verification

### 11.1 Component and Hook Tests

- Overview is the default tab.
- No performance request occurs before Performance is opened.
- `1h`, `24h`, and `7d` produce `hours=1`, `24`, and `168`.
- Cached data is reused during the 60-second freshness window.
- A stale request cannot overwrite the current model or range.
- Loading, empty, sparse-sample, error, refresh-error, and success states render correctly.
- Retry requests only the active performance dataset.
- Metric formatting handles milliseconds, seconds, percentages, TPS, and invalid values.

### 11.2 Regression Checks

- Basic model information remains visible in Overview.
- Group pricing calculations and display are unchanged.
- Dynamic billing expressions render as before.
- Existing endpoint information remains visible in API.
- Closing and reopening model details preserves existing page behavior.

### 11.3 Responsive and Visual Checks

- Desktop screenshot at a representative 1440px-wide viewport.
- Mobile screenshot at a representative 390px-wide viewport.
- Horizontal group-table scrolling works on mobile.
- Long model names, group names, localized labels, and large numeric values do not overlap.
- Loading and state transitions do not shift the stable panel dimensions.

### 11.4 Engineering Checks

- Targeted frontend tests.
- ESLint.
- Vite production build.
- Browser smoke test against the local `v0.13.2` backend with seeded performance data.

## 12. Out of Scope

- Backfilling performance data from historical logs.
- Migrating the latest React, router, state, or query stack into `v0.13.2`.
- Performance badges or rankings on every model card.
- User-, token-, channel-, or request-level drill-down.
- A performance comparison page across multiple models.
- A new API playground inside model details.

## 13. Acceptance Criteria

The frontend design is complete when:

- All three tabs work without losing existing model-detail functionality.
- Real performance data renders with statistically correct overall metrics.
- `1h`, `24h`, and `7d` produce useful trend series.
- Loading, empty, sparse, error, and success states are distinguishable.
- Desktop and mobile layouts remain readable without incoherent overlap.
- The frontend passes targeted tests, lint, build, and browser smoke checks.
