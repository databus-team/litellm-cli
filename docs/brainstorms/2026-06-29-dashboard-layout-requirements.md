# Dashboard Layout 统一设计

**Date:** 2026-06-29
**Status:** Draft

## 目标

为 litellm-cli TUI dashboard 建立跨视图、跨层级的稳定 layout 结构。

## 当前问题

- **一级视图**（Dashboard）：`header(tabs) + content + footer`
- **二级视图**（Log Detail）：`header(tabs) + header(title+help) + content + footer`（中间多了一层，且 help 与 footer 重复）

## 方案

### Layout 结构

所有视图统一为三层结构：

```
┌─────────────────────────────┐
│         header             │  ← 导航层
├─────────────────────────────┤
│         content            │  ← 数据层
├─────────────────────────────┤
│         footer             │  ← 帮助层
└─────────────────────────────┘
```

### header 行为

| 视图层级 | header 内容 | 交互性 |
|----------|-------------|--------|
| 一级视图 | tabs（Stats / Models / Teams / Keys / Logs） | 可交互 |
| 二级视图 | breadcrumb（如 `logs > message[2]`） | 不可交互，导航用途 |
| N级视图 | breadcrumb | 不可交互 |

### footer 行为

- **左侧**：空（预留未来使用）
- **右侧**：help 按键提示（如 `←/→: 切换 | q: 退出`）

### breadcrumb 格式

- 二级：`logs > message`
- 带索引：`logs > message[2]`
- 多级时显示最近 2-3 级，避免过长

## 适用范围

- `internal/tui/dashboard/model.go` — Dashboard 主视图
- `internal/tui/logs/model.go` — Log 详情视图
- 其他视图按相同模式扩展

## 待确认

- 三级及以上视图的 breadcrumb 截断策略
- 各视图的 help 文本内容统一规范