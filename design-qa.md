# Customer Billing Option 2 — Design QA

- source visual truth path: `/Users/randy/.codex/generated_images/019fe4a2-869a-7741-8b87-8fb1890a16ad/exec-bf240ab5-3c3e-45c0-970a-eb35fbc84609.png`
- implementation screenshot path: `/Users/randy/.codex/visualizations/2026/08/09/019fe4a2-869a-7741-8b87-8fb1890a16ad/customer-billing-option-2.png`
- full-view comparison evidence: `/Users/randy/.codex/visualizations/2026/08/09/019fe4a2-869a-7741-8b87-8fb1890a16ad/customer-billing-comparison.png`
- focused region comparison evidence: `/Users/randy/.codex/visualizations/2026/08/09/019fe4a2-869a-7741-8b87-8fb1890a16ad/customer-billing-focused-comparison.png`
- viewport: desktop, 1920 × 873 CSS px, light theme, authenticated platform administrator, August 2026 customer statements
- source pixels: 1487 × 1058
- implementation pixels: 1920 × 873; browser `devicePixelRatio` was 2, while the captured file was normalized by the browser tool to one output pixel per CSS pixel
- density normalization: the implementation was proportionally scaled to 1487 px width for the combined full-view comparison; focused content regions were cropped independently and the implementation region was proportionally scaled to the source-region width without aspect-ratio distortion

## Findings

No actionable P0, P1, or P2 differences remain.

- Fonts and typography: the implementation uses the product's existing Public Sans stack and preserves the reference hierarchy: compact page title, restrained helper copy, medium summary figures, and dense table typography. Customer identity uses a two-line treatment to keep the stable ID visible without crowding the primary name.
- Spacing and layout rhythm: the implementation retains the reference sequence of filters, compact total strip, and customer table. Summary dividers and consistent card radii restore the dense operational-ledger rhythm while staying inside the existing page shell.
- Colors and visual tokens: neutral surfaces and borders map to the existing shadcn tokens. Discount values use the product success token, while incomplete database facts use the warning badge instead of a misleading green status.
- Image and asset fidelity: the target contains no bespoke product imagery. The implementation reuses the existing TokenAI logo and Hugeicons family; no placeholder, CSS-drawn, inline-SVG, or generated substitute was introduced.
- Copy and content: labels use database-backed billing terms and seven-locale i18n. Runtime values reflect the local database, including partial data quality and the configured display currency, instead of copying mock values.
- Accessibility and behavior: search has a visible label, sort controls are semantic buttons, pagination exposes current/disabled state, the list remains horizontally scrollable at constrained widths, and customer drill-down/back controls are keyboard-reachable.

Expected product-constrained differences from the concept image:

- The concept's statement-status filter and export button are omitted because neither has a persisted backend fact or implemented export contract. Adding either now would conflict with the database-authoritative rule.
- Billing month remains in the shared page header rather than being duplicated in the customer-list filter card.
- The concept's sample CNY values and all-complete statuses are replaced by the environment's configured currency and actual database quality states.
- A small TanStack Router badge appears only in the development screenshot; it is absent from the production build.

## Interaction Evidence

- customer search reduced the table to one matching customer and updated the URL
- keyboard clearing removed `customerSearch` from the URL and restored all five database customers
- request sorting updated the URL and placed the highest-request customer first
- `查看账单` opened the selected customer's channel/API Key drill-down
- `返回客户账单` removed `userId` and restored the list while preserving list sort state
- page reload produced no observed console event, and no React error boundary, hydration warning, failed request state, or backend 5xx was present

## Comparison History

### Iteration 1

- [P1] Query-state cleanup needed an explicit navigation reducer so list filters and `userId` could be removed without losing the rest of the deep-link state.
- [P2] The summary bar lacked the reference's dividers and the discount figures did not carry the positive semantic accent.
- Fixes: changed billing search updates to a functional search reducer; added summary dividers; applied the existing success token to discount values.

### Iteration 2

- Post-fix evidence: `customer-billing-option-2.png`, `customer-billing-comparison.png`, and `customer-billing-focused-comparison.png`.
- Search clear, sorting, drill-down, and back navigation were re-run successfully.
- No actionable P0/P1/P2 findings remain.

## Follow-up Polish

- [P3] Repeat the responsive visual pass in a signed-in tablet/mobile browser session; the current pass verified desktop behavior and code-level responsive fallbacks only.

final result: passed

---

# Upstream Cost Statement — Selected Design QA

- source visual truth path: `/Users/randy/.codex/generated_images/019fe5ef-832e-7a40-b0b1-1d5aa224b4a2/exec-57e336d5-69c0-4617-8a22-87d4c5f4d5aa.png`
- implementation screenshot path: `/Users/randy/code/refeiner/yuan-gateway/design-qa-upstream-cost-statement.jpg`
- full-view comparison evidence: `/Users/randy/code/refeiner/yuan-gateway/design-qa-upstream-cost-statement-comparison.png`
- viewport: desktop, 1789 × 879 CSS px, light theme, authenticated platform administrator, August 2026 upstream statement
- source pixels: 1789 × 879
- implementation pixels: 1789 × 813; the Chrome viewport excludes browser chrome from the captured page surface

## Findings

No actionable P0, P1, or P2 differences remain.

- Information architecture: the implemented statement preserves the selected design's channel parent row → model child row hierarchy. Each model appears once per billing mode beneath its channel, with explicit expand/collapse controls and tree connectors.
- Metric priority: input, cache-read, cache-write, output Token usage and billable calls remain the dominant columns. Reference amount is visually secondary and stays unavailable when authoritative historical pricing facts are incomplete.
- Numeric density: large Chinese-locale usage values use `万`/`亿` compaction while small values remain exact, matching the selected design's scan pattern without changing CSV precision.
- Export behavior: `导出我方账单` exports one CSV row per visible model, applies the active channel/model filters, preserves exact numeric values, and neutralizes spreadsheet formulas.
- Accessibility: month and filter fields have visible labels, channel disclosure buttons expose expanded state, model detail links are keyboard-reachable, and the dense table remains horizontally scrollable.

Expected product-shell differences from the concept image:

- The live implementation remains inside the existing authenticated TokenAI shell and keeps the customer/upstream statement tabs; the concept isolates only the page content.
- The live database currently reports partial data and missing historical reference-price snapshots, so the warning and unavailable amounts are shown instead of copying the concept's sample completeness and CNY values.
- A TanStack Router development badge appears only in local development and is absent from the production build.

## Interaction Evidence

- channel filtering reduced the table to Channel #23 while preserving its authoritative channel total
- model filtering reduced the table to the two `gpt-5.6-sol` billing-mode rows under their channel groups
- channel rows collapsed and expanded without duplicating model rows
- export produced the `账单 CSV 已导出。` success feedback
- the final fixed reload rendered two default-expanded channels, displayed compact usage values, and added no new browser warning or error events

## Comparison History

### Iteration 1

- [P1] The runtime language identifier `zhCN` was not a valid `Intl.NumberFormat` locale and caused the statement route to hit its error boundary.
- [P2] Large Token values were shown as long raw integers and child rows relied on indentation alone, making the channel/model hierarchy less scannable than the selected design.
- Fixes: normalized the supported Chinese locale identifiers, added locale-aware `万`/`亿` display compaction, and added model-tree connector lines.

### Iteration 2

- Post-fix evidence: `design-qa-upstream-cost-statement.jpg` and `design-qa-upstream-cost-statement-comparison.png`.
- Channel/model filters, disclosure behavior, CSV export feedback, default expansion, and post-reload console state were re-run successfully.
- No actionable P0/P1/P2 findings remain.

## Follow-up Polish

- [P3] Add a persisted historical provider-price snapshot for channels that currently show unavailable reference amounts; this is a data-contract follow-up, not a UI fallback.

final result: passed
