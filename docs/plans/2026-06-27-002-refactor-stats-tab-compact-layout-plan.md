---
date: 2026-06-27
topic: stats-tab-compact-layout
origin: docs/brainstorms/2026-06-27-stats-tab-compact-layout-requirements.md
type: refactor
---

# Stats Tab 紧凑布局优化

## Summary

优化 dashboard 统计 tab 的布局，将聚合指标卡片紧凑横向排列在顶部（2x3 或 3x2 布局），底部使用水平柱状图展示每日用量，替代原来占用空间大的大方块垂直布局和垂直柱状图。

## Problem Frame

当前 stats tab 使用大尺寸卡片垂直排列（每行 1-2 个），空间利用率低；垂直柱状图（每天一行）在屏幕较窄时展示效果不佳。需要更紧凑的布局。

## Requirements (from origin)

- **R1**: 顶部聚合指标区域 — 6 个核心指标紧凑卡片，横向排列
- **R2**: 底部水平柱状图 — 横向展示每日花费
- **R3**: 详情面板 — 选中 bar 时显示详细指标
- **R4**: 响应式适配 — 根据屏幕宽度调整布局
- **R5**: 交互保持 — j/k 导航，q 退出

## Key Technical Decisions

1. **水平柱状图替代垂直柱状图** — 每日数据横向展示，左侧日期+金额，右侧进度条
2. **紧凑卡片布局** — 2 行 x 3 列（大屏）或 3 行 x 2 列（中屏）
3. **复用现有数据模型** — 不修改数据结构，只改渲染逻辑

## Implementation Units

### U1. 重构 Counter 卡片渲染为紧凑横向布局

**Goal:** 将原来的大卡片垂直布局改为紧凑的横向卡片网格

**Requirements:** R1, R4

**Files:**
- `internal/tui/stats/model.go` — 修改 `renderCounterContent` 方法

**Approach:**
- 保持 6 个核心指标不变：总花费、总请求、成功数、失败数、总 Tokens、平均费用
- 减小卡片尺寸：宽约 25-30 字符，高 2-3 行
- 使用动态列数：宽度 >= 100 时 3 列，60-99 时 2 列，< 60 时 1 列
- 卡片内容：图标 + 标签 + 数值（单行或紧凑换行）

**Patterns to follow:**
- 参考现有的 lipgloss 样式模式
- 使用现有的颜色变量（mutedStyle, valueStyle）

**Test scenarios:**
- 大屏显示 3 列卡片
- 中屏显示 2 列卡片
- 小屏显示 1 列卡片
- 数值正确显示（货币格式、Token 格式）

---

### U2. 实现水平柱状图

**Goal:** 将每日用量从垂直柱状图改为水平横向进度条

**Requirements:** R2, R5

**Files:**
- `internal/tui/stats/model.go` — 修改或替换 `renderBarContent` 方法

**Approach:**
- 每行显示：[日期] [████████░░░░] $XX.XX
- 日期左对齐，花费右对齐，中间用进度条填充
- 进度条宽度根据相对花费比例计算（最大花费 = 100% 宽度）
- 选中项高亮显示（使用选中样式）

**Patterns to follow:**
- 参考现有 bar 渲染的颜色和样式
- 保持选中状态的视觉区分

**Test scenarios:**
- 显示正确数量的日期行
- 进度条比例与花费成正比
- 选中项高亮显示
- 无数据时显示空状态

---

### U3. 添加/优化详情面板

**Goal:** 选中水平 bar 时显示该日详细指标

**Requirements:** R3

**Files:**
- `internal/tui/stats/model.go` — 更新 `renderBarContent` 或添加新方法

**Approach:**
- 当 `selectedBarIndex >= 0` 时，在水平 bar 下方或右侧显示详情
- 详情内容：日期、花费、请求数、成功数、失败数、Prompt Tokens、Completion Tokens、Total Tokens
- 紧凑布局，与水平 bar 协调

**Test scenarios:**
- 选中某天时显示详情
- 详情内容正确
- 取消选中时隐藏详情

---

### U4. 响应式适配

**Goal:** 根据屏幕宽度自动调整布局

**Requirements:** R4

**Files:**
- `internal/tui/stats/model.go` — 更新 View 方法中的响应式逻辑

**Approach:**
- 大屏 (>= 100 列)：2 行 x 3 列卡片
- 中屏 (60-99 列)：3 行 x 2 列卡片
- 小屏 (< 60 列)：保持现有 fallback 或简化显示
- 水平 bar 宽度自适应：最大宽度限制，最小宽度保护

**Test scenarios:**
- 调整窗口大小时布局正确变化
- 水平 bar 不会溢出屏幕
- 卡片不会重叠或截断

---

## Verification

1. 运行 `litellm-cli dashboard`，切换到 Stats tab
2. 验证顶部显示 6 个紧凑卡片（2x3 布局）
3. 验证底部显示水平柱状图
4. 使用 j/k 键导航，验证选中高亮和详情显示
5. 调整终端窗口大小，验证响应式布局
6. 切换到其他 tab 再切回，确认状态正确

## Scope Boundaries

### Deferred for later
- 团队维度统计（使用 team_rank tab）
- 其他 Tab 的布局优化

### Outside this product
- 导出功能
- 数据过滤/排序