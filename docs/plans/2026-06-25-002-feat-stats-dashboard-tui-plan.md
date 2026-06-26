---
title: "feat: Stats Dashboard TUI"
type: feat
date: 2026-06-25
origin: docs/brainstorms/2026-06-25-stats-dashboard-tui-requirements.md
---

# Stats Dashboard TUI

## Summary

Replace the existing `stats` command text-based table output with an interactive TUI dashboard featuring Counter view (overview metrics) and Bar Chart view (daily spend trends) with keyboard navigation and hover details.

## Problem Frame

The current `stats` command uses ASCII tables to display usage data, making it difficult to quickly grasp overall usage patterns. Users need a more intuitive visual way to view daily spend trends and key metrics.

## Requirements

### R1. Dashboard as Default
- Stats command displays Dashboard TUI by default
- Replaces existing text table output

### R2. Time Range Support
- Retain existing `--period day|week|month` parameter
- Add `--from YYYY-MM-DD --to YYYY-MM-DD` custom date range
- Default: current day (day)

### R3. Counter View (Overview)
Display these metrics:
- Total Spend: total cost for period
- Total Requests: total API requests
- Successful Requests: successful request count
- Failed Requests: failed request count
- Prompt Tokens: prompt tokens used
- Completion Tokens: completion tokens generated
- Total Tokens: prompt + completion tokens
- Avg Cost/Request: Total Spend / Total Requests

### R4. Bar Chart View (Daily Trends)
- X-axis: dates
- Y-axis: daily spend
- Date labels at bar base

### R5. Bar Hover/Focus Details
- Floating detail panel when bar is focused
- Display: Date, Spend, Requests, Successful, Failed, Prompt Tokens, Completion Tokens, Total Tokens
- Rendered with TUI styling (not system tooltip)

### R6. Responsive Layout
- Large screen (>120 cols): side-by-side or stacked layout
- Small screen: view switching via keyboard

### R7. Keyboard Navigation
- `Tab` / `Shift+Tab`: Switch views (Counter ↔ Bar Chart)
- `k`/`↑`: Move focus up in Bar Chart
- `j`/`↓`: Move focus down in Bar Chart
- `q`: Exit Dashboard

### R8. Data Source
- Use existing API: `/user/daily/activity`
- No new API calls required

## Key Technical Decisions

**KTD1: Dashboard replaces text table** — No flag needed; the default behavior changes.

**KTD2: No model distribution view** — Current user has only one model.

**KTD3: TUI-style hover panel** — Use TUI rendering for consistency with the rest of the dashboard.

**KTD4: Reuse existing Bubble Tea patterns** — Follow the `cmd/logs.go` model architecture with `tea.Model` interface (`Init`, `Update`, `View`).

## Scope Boundaries

### Deferred for later
- Model distribution Pie/Donut chart (not needed now)
- Export functionality (CSV/JSON)

### Outside this product
- Team statistics (handled by `team_rank` command)
- Alert/threshold settings

## Implementation Units

### U1. Add custom date range flags

**Goal:** Add `--from` and `--to` parameters to the stats command.

**Files:**
- `cmd/stats.go`

**Approach:**
- Add two new string flags `fromDate` and `toDate`
- Modify `getDateRange()` to check for custom dates first
- Parse dates with `time.Parse("2006-01-02", value)` and validate

**Patterns to follow:**
- Existing flag patterns in `cmd/stats.go:26-29`

**Test scenarios:**
- Happy path: `stats --from 2026-01-01 --to 2026-01-07` uses custom range
- Edge case: Invalid date format shows error
- Edge case: `from` after `to` shows error

---

### U2. Create Stats TUI Model

**Goal:** Implement Bubble Tea model for the stats dashboard.

**Files:**
- `cmd/stats.go` (add TUI model)
- `internal/api/types.go` (reuse existing types)

**Approach:**
- Create `statsModel` struct following `logsModel` pattern from `cmd/logs.go`
- Implement `tea.Model` interface: `Init()`, `Update()`, `View()`
- State: current view (counter/bar), date range, aggregated metrics, daily data slice, selected bar index

**Patterns to follow:**
- `cmd/logs.go` model structure at lines 740-776
- Ticker pattern for auto-refresh (optional, skip for static display)

**Test scenarios:**
- Happy path: Model initializes and renders Counter view
- Edge case: Empty data shows "No data" message

---

### U3. Implement Counter View

**Goal:** Render overview metrics in the dashboard.

**Approach:**
- Calculate totals from daily data
- Use lipgloss for styled output
- Display in card-like layout

**Test scenarios:**
- Happy path: All 6 metrics display correctly
- Edge case: Zero values handled gracefully

---

### U4. Implement Bar Chart View

**Goal:** Render daily spend as a bar chart.

**Approach:**
- Calculate bar width proportional to max daily spend
- Render ASCII bars with date labels at base
- Track `selectedBarIndex` for focus state

**Test scenarios:**
- Happy path: Bars render for multi-day data
- Edge case: Single day shows minimal bar
- Edge case: Max spend is 0 (no bar render)

---

### U5. Implement Bar Detail Panel

**Goal:** Show floating detail panel when bar is focused.

**Approach:**
- When `selectedBarIndex >= 0`, render detail overlay
- Position: right side or bottom based on screen width
- Display all metrics for selected date

**Test scenarios:**
- Happy path: Detail panel shows correct data for selected bar
- Edge case: Navigate off ends of data clears selection

---

### U6. Implement Keyboard Navigation

**Goal:** Handle keyboard input for view switching and bar navigation.

**Approach:**
- `Tab`/`Shift+Tab`: Toggle `viewMode` between "counter" and "bar"
- `j`/`↓`: Increment `selectedBarIndex` (only in bar view, moves down)
- `k`/`↑`: Decrement `selectedBarIndex` (only in bar view, moves up)
- `q`: Set `quitting = true`, return tea.Quit

**Test scenarios:**
- Happy path: Tab switches views
- Happy path: j/k navigates bars
- Happy path: q exits

---

### U7. Integrate TUI into stats command

**Goal:** Replace text output with TUI.

**Files:**
- `cmd/stats.go`

**Approach:**
- Keep existing `--period`, add `--from`, `--to` flags
- Always launch Bubble Tea program instead of text output
- Remove or deprecate text rendering functions

**Test scenarios:**
- Happy path: `stats` launches TUI
- Happy path: `stats --period week` shows week data in TUI

---

## Risks & Dependencies

- **Risk:** Terminal size detection may differ between environments — test on various terminal sizes
- **Dependency:** `/user/daily/activity` API must be working (already used by existing stats)
- **Dependency:** Bubble Tea and lipgloss libraries already in use (`cmd/logs.go`)

## Sources & Research

- Existing TUI patterns: `cmd/logs.go:740-1273` — Bubble Tea model implementation
- API types: `internal/api/types.go:93-141` — `UserDailyActivityResponse` structure
- Styling: `cmd/logs.go:170-185` — lipgloss style definitions