# Channel Quota Card Design

## Goal

Present channel quota configuration with the same money-first interaction and visual hierarchy as token quota configuration.

## Scope

- Keep `quota_limit_mode`, `quota_limit`, `quota_limit_used`, and reset behavior unchanged in the API.
- Move quota configuration out of the core configuration card into a dedicated card.
- Default to display-currency input and convert through the existing quota helpers.
- Provide an expandable raw quota input and an unlimited toggle.

## Interaction

1. The new card shows a green credit-card icon, title, and short description.
2. The quota-mode selector remains visible. Single-key channels offer `none` and `channel`; multi-key channels also offer `key` and `both`.
3. A display-currency amount input is the default entry point. It writes the converted integer quota to `quota_limit`.
4. The raw quota input is collapsed initially. Editing it updates the display-currency value.
5. Enabling unlimited sets `quota_limit_mode` to `none` and `quota_limit` to `0`; both inputs are disabled. Disabling it restores the previously selected finite mode, defaulting to `channel`.
6. Existing used-quota and reset controls remain in the card. Reset still only clears usage and never enables a channel.

## Validation

- Currency amount has six decimal places and cannot be negative.
- Raw quota is a non-negative integer.
- Building the web app must succeed.
