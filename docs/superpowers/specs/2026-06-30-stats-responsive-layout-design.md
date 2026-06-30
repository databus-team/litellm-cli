# TUI 响应式布局修复设计

## 问题描述

在极宽但扁的窗口下（如笔记本垂直分屏），stats 视图和 logs 详情视图的顶部 header 和 counter 会被挤出屏幕之外。

## 影响范围

- **stats 视图**：顶部 header、counter 卡片区域
- **logs 详情视图**：顶部 header（需验证）

## 问题根因

### stats/model.go

```go
// 硬编码的固定行数估算，与实际渲染行数不匹配
fixedLines := 1 + 2 + 1  // 时间选择器(1) + 卡片(2) + 分隔线(1)
if m.showHeader {
    fixedLines += 3  // header(2) + footer(1)
}
maxBarLines := m.height - fixedLines
```

问题：
1. `renderCounterContent()` 固定渲染 2 行，不接收高度限制
2. 当窗口很扁时，`m.height < fixedLines`，导致 `maxBarLines` 为负数或过小
3. 实际渲染行数超过估算，导致 header/counter 被挤出

### logs/detail view

代码使用 `m.height - 15` 计算可见区域，需验证是否充分。

## 修复方案

### 核心原则

采用**动态高度传递**：每个渲染函数返回实际占用的行数，上游函数根据剩余高度动态调整。

### 修改点 1：stats/model.go - View()

```go
func (m *Model) View() string {
    // 计算可用高度
    availableHeight := m.height
    if m.showHeader {
        availableHeight -= 3  // header(2) + footer(1)
    }
    
    // 时间选择器固定 1 行
    timeSelectorHeight := 1
    
    // 计算 counter 和 bar 的可用高度
    // 先估算 counter 最大行数（基于列数和指标数）
    counterMaxRows := (len(metrics) + cols - 1) / cols  // 向上取整
    counterMaxHeight := availableHeight - timeSelectorHeight - 1 - 3  // -1 分隔线 -3 最小bar
    
    // 限制 counter 行数
    if counterMaxRows > counterMaxHeight {
        counterMaxRows = counterMaxHeight
    }
    if counterMaxRows < 1 {
        counterMaxRows = 1
    }
    
    // 传递 maxRows 给 counter 渲染
    counterActualRows := m.renderCounterContent(counterWidth, counterMaxRows)
    
    // 计算 bar 可用高度
    barAvailableHeight := availableHeight - timeSelectorHeight - counterActualRows - 1
    if barAvailableHeight < 3 {
        barAvailableHeight = 3
    }
}
```

### 修改点 2：renderCounterContent()

```go
func (m *Model) renderCounterContent(width int, maxRows int) int {
    // 根据 maxRows 动态调整渲染内容
    // 返回实际渲染的行数
    metrics := []struct {...}{...}
    
    // 计算应该渲染的行数
    cols := 3
    if width < 80 { cols = 2 }
    if width < 40 { cols = 1 }
    
    actualRows := (len(visibleMetrics) + cols - 1) / cols
    if actualRows > maxRows {
        actualRows = maxRows
    }
    
    // 只渲染前 actualRows 行的指标
    // ...
    
    return actualRows
}
```

### 修改点 3：renderBarContent()

已有 `maxLines` 参数，确保正确使用：
```go
renderCount := len(displayData)
if renderCount > maxLines {
    renderCount = maxLines
}
```

### 修改点 4：logs 详情视图

保持当前逻辑，验证 `m.height - 15` 在极扁窗口下是否足够：
- 当前使用 `availableLines := m.height - 4` 和 `maxDisplayLines := m.height - headerLines - 1`
- 需添加最小高度保护：`if m.height < 15 { 切换到极简布局 }`

## 验证测试用例

| 窗口尺寸 | 场景 | 预期结果 |
|----------|------|----------|
| 120x40  | 正常 | 全部正常显示 |
| 120x20  | 极扁 | stats: counter 压缩为 1 行，bar 正常；logs: 详情正常 |
| 120x15  | 极限 | stats: 显示核心信息；logs: 详情可滚动 |
| 80x30   | 窄屏 | stats: 2 列 counter；logs: 正常 |

## 实施步骤

1. 修改 `stats/model.go` 的 View() 和 renderCounterContent()
2. 验证 stats 视图
3. 检查 logs 详情视图是否需要类似修改
4. 测试各种窗口尺寸

## 风险与回退

- **风险**：修改可能影响现有布局，需全面测试
- **回退**：git revert 快速回退
