---
date: 2026-06-25
topic: stats-dashboard-tui
focus: Stats command dashboard with visualizations
mode: repo-grounded
---

# Stats Dashboard TUI

## Summary

用可视化 TUI 替换现有的 stats 命令文本表格，展示个人使用数据的 Dashboard，支持 Counter（总览统计）和 Bar Chart（每日花费趋势）两种视图，键盘导航切换，Bar 支持 hover/聚焦显示详细数据。

## Problem Frame

当前 `stats` 命令使用纯文本 + ASCII 表格展示数据，信息密度低，难以快速理解整体使用情况。用户需要更直观的可视化方式来查看每日花费趋势和关键指标。

## Requirements

### R1. Dashboard 替换
- Stats 命令的默认行为改为展示 Dashboard TUI
- 替换现有的纯文本表格输出

### R2. 时间范围支持
- 保留现有 `--period day|week|month` 参数
- 新增 `--from YYYY-MM-DD --to YYYY-MM-DD` 自定义日期范围参数
- 默认行为：当天数据 (day)

### R3. Counter 视图 (总览)
显示以下指标的 Counter：
- Total Spend: 指定区间的总花费
- Total Requests: 总请求数
- Successful Requests: 成功请求数
- Failed Requests: 失败请求数
- Total Tokens: 总 Token 数 (prompt + completion)
- Avg Cost/Request: 平均每次请求成本 (Total Spend / Total Requests)

### R4. Bar Chart 视图 (每日趋势)
- X 轴：日期
- Y 轴：每日花费 (spend)
- 每个 Bar 底部显示日期标签

### R5. Bar Hover/聚焦详情
- 当 Bar 获得焦点时，显示浮动详情面板
- 显示内容：Date, Spend, Requests, Successful, Failed, Total Tokens
- 详情面板使用 TUI 样式渲染（不是系统 tooltip）

### R6. 响应式布局
- 大屏（宽 > 120列）：Counter 和 Bar 并排或上下布局
- 小屏：切换视图展示（通过键盘切换）

### R7. 键盘导航
- `Tab` / `Shift+Tab`: 切换视图（Counter ↔ Bar）
- `j`/`k` 或 `↓`/`↑`: 在 Bar Chart 中移动焦点
- `q`: 退出 Dashboard

### R8. 数据来源
- 使用现有 API: `/user/daily/activity`
- 不需要新增 API 调用

## Key Decisions

1. **Dashboard 替换文本表格** — 保持交互简洁，不需要 flag 区分
2. **不显示模型分布** — 当前用户只有一个模型，无需此视图
3. **TUI 样式 hover** — 不使用系统 tooltip，用 TUI 渲染保持一致性

## Acceptance Examples

### AE1. 基本展示
- 运行 `litellm-cli stats` 显示 Counter 视图
- 显示当天的花费和请求统计

### AE2. 时间范围切换
- `litellm-cli stats --period week` 显示最近7天数据
- `litellm-cli stats --from 2026-01-01 --to 2026-01-07` 显示自定义范围

### AE3. 视图切换
- 按 `Tab` 从 Counter 切换到 Bar Chart
- Bar Chart 显示每日花费柱状图

### AE4. Bar 详情
- 用 `j`/`k` 移动焦点到某个 Bar
- 详情面板显示该日期的详细数据

### AE5. 退出
- 按 `q` 退出 Dashboard

## Scope Boundaries

### Deferred for later
- 按模型分布的 Pie/Donut 图（当前不需要）
- 导出功能（CSV/JSON）

### Outside this product
- 团队统计（team_rank 命令负责）
- 告警/阈值设置

## Dependencies

- 现有 `/user/daily/activity` API 正常工作
- Bubble Tea TUI 框架已用于 logs 命令

## Outstanding Questions

- **Resolve before planning:** None

- **Deferred to planning:**
  - 具体的大屏/小屏断点选择（需要测试不同终端尺寸）
  - Bar 详情面板的具体位置和样式
  - 每日花费柱状图的颜色主题