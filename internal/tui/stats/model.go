package stats

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"litellm-cli/internal/api"
	"litellm-cli/internal/tui/components"
)

// StatsClient defines the client interface required by the stats TUI
type StatsClient interface {
	GetUserDailyActivity(startDate, endDate string) (*api.UserDailyActivityResponse, error)
	GetTeamDailyActivity(startDate, endDate string) (*api.TeamDailyActivityResponse, error)
}

// Model represents the stats TUI model
type Model struct {
	client           StatsClient
	startDate        string
	endDate          string
	viewMode         string // "counter" or "bar"
	data             []api.UserDailyActivity
	aggregated       aggregatedMetrics
	selectedBarIndex int
	width            int
	height           int
	quitting         bool
	err              string
	loading          bool
	By               string // Aggregation dimension: "user", "team", etc.
	showHeader       bool   // 是否显示顶部 header（在 dashboard 中隐藏）
}

type aggregatedMetrics struct {
	TotalSpend       float64
	TotalRequests    int64
	Successful       int64
	Failed           int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	AvgCostPerReq    float64
}

// StatsLoadedMsg is triggered when stats data has finished loading asynchronously
type StatsLoadedMsg struct {
	Data  []api.UserDailyActivity
	Error error
}

// NewModel creates a new stats TUI Model
func NewModel(client StatsClient, startDate, endDate string) *Model {
	return &Model{
		client:           client,
		startDate:        startDate,
		endDate:          endDate,
		viewMode:         "counter",
		selectedBarIndex: -1,
		width:            120,
		height:           40,
		loading:          true,
		By:               "user",
		showHeader:      true, // 默认显示 header
	}
}

// Init initializes the Model by returning the refresh command
func (m *Model) Init() tea.Cmd {
	return m.RefreshCmd()
}

// Update handles messages and updates the Model's state
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StatsLoadedMsg:
		m.loading = false
		if msg.Error != nil {
			m.err = fmt.Sprintf("获取数据失败: %v", msg.Error)
			return m, nil
		}
		m.data = msg.Data
		m.calculateAggregated()
		if len(m.data) > 0 && m.selectedBarIndex < 0 {
			m.selectedBarIndex = 0
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			if m.viewMode == "counter" {
				m.viewMode = "bar"
				if len(m.data) > 0 && m.selectedBarIndex < 0 {
					m.selectedBarIndex = 0
				}
			} else {
				m.viewMode = "counter"
			}
			return m, nil
		case "shift+tab":
			if m.viewMode == "bar" {
				m.viewMode = "counter"
			} else {
				m.viewMode = "bar"
				if len(m.data) > 0 && m.selectedBarIndex < 0 {
					m.selectedBarIndex = 0
				}
			}
			return m, nil
		case "down", "j":
			if len(m.data) > 0 {
				if m.selectedBarIndex < len(m.data)-1 {
					m.selectedBarIndex++
				}
			}
			return m, nil
		case "up", "k":
			if len(m.data) > 0 {
				if m.selectedBarIndex > 0 {
					m.selectedBarIndex--
				}
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View renders the terminal user interface
func (m *Model) View() string {
	if m.quitting {
		return "👋 已退出\n"
	}
	if m.err != "" {
		return components.NewErrorBanner(m.err).View(m.width) + "\n"
	}
	if m.loading {
		return components.NewLoader("正在加载统计数据...").View() + "\n"
	}
	if len(m.data) == 0 {
		return components.NewPlaceholder("暂无数据").View() + "\n"
	}

	// 响应式断点
	isLargeScreen := m.width >= 100

	var sb strings.Builder

	// 显示 header（如果启用）
	if m.showHeader {
		if isLargeScreen {
			header := components.NewHeader("用量统计看板", fmt.Sprintf("%s - %s | 按 q 退出", m.startDate, m.endDate))
			sb.WriteString(header.View(m.width))
			sb.WriteString("\n")
		} else {
			header := components.NewHeader("用量统计", fmt.Sprintf("%s - %s | 按 Tab 切换视图 | 按 q 退出", m.startDate, m.endDate))
			sb.WriteString(header.View(m.width))
			sb.WriteString("\n")
		}
	}

	// 新布局：顶部紧凑卡片 + 底部水平柱状图
	// 顶部卡片区域（紧凑排列）
	counterWidth := m.width - 4
	if counterWidth > 100 {
		counterWidth = 100
	}
	sb.WriteString(m.renderCounterContent(counterWidth))
	sb.WriteString("\n")

	// 分隔线
	if isLargeScreen {
		sb.WriteString(strings.Repeat("─", m.width))
		sb.WriteString("\n")
	}

	// 底部水平柱状图
	barWidth := m.width - 4
	sb.WriteString(m.renderBarContent(barWidth))
	sb.WriteString("\n")

	// 帮助信息
	if m.viewMode == "bar" {
		help := components.NewHelp([]components.HelpKey{
			{Key: "j/k 或 ↓/↑", Desc: "移动选择"},
			{Key: "Tab", Desc: "切换视图"},
			{Key: "q", Desc: "退出"},
		})
		sb.WriteString(help.View(m.width))
	} else {
		help := components.NewHelp([]components.HelpKey{
			{Key: "Tab", Desc: "切换视图"},
			{Key: "q", Desc: "退出"},
		})
		sb.WriteString(help.View(m.width))
	}

	return sb.String()
}

// RefreshCmd performs asynchronous data loading
func (m *Model) RefreshCmd() tea.Cmd {
	return func() tea.Msg {
		var data []api.UserDailyActivity
		var err error
		if m.By == "team" {
			var resp *api.TeamDailyActivityResponse
			resp, err = m.client.GetTeamDailyActivity(m.startDate, m.endDate)
			if err == nil && resp != nil {
				data = make([]api.UserDailyActivity, len(resp.Results))
				for i, r := range resp.Results {
					data[i] = api.UserDailyActivity{
						Date:      r.Date,
						Metrics:   r.Metrics,
						Breakdown: r.Breakdown,
					}
				}
			}
		} else {
			var resp *api.UserDailyActivityResponse
			resp, err = m.client.GetUserDailyActivity(m.startDate, m.endDate)
			if err == nil && resp != nil {
				data = resp.Results
			}
		}
		return StatsLoadedMsg{Data: data, Error: err}
	}
}

func (m *Model) calculateAggregated() {
	var totalSpend float64
	var totalPrompt, totalCompletion, totalTokens int64
	var totalSuccess, totalFailed, totalRequests int64

	for _, r := range m.data {
		totalSpend += r.Metrics.Spend
		totalPrompt += r.Metrics.PromptTokens
		totalCompletion += r.Metrics.CompletionTokens
		totalTokens += r.Metrics.TotalTokens
		totalSuccess += r.Metrics.SuccessfulRequests
		totalFailed += r.Metrics.FailedRequests
		totalRequests += r.Metrics.APIRequests
	}

	avgCost := 0.0
	if totalRequests > 0 {
		avgCost = totalSpend / float64(totalRequests)
	}

	m.aggregated = aggregatedMetrics{
		TotalSpend:       totalSpend,
		TotalRequests:    totalRequests,
		Successful:       totalSuccess,
		Failed:           totalFailed,
		PromptTokens:     totalPrompt,
		CompletionTokens: totalCompletion,
		TotalTokens:      totalTokens,
		AvgCostPerReq:    avgCost,
	}
}

func (m *Model) renderCounterContent(width int) string {
	var sb strings.Builder

	// 紧凑卡片样式（无边框紧凑显示）
	cardStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("159"))

	// 6 个核心指标（紧凑显示：图标 + 标签 + 数值在同一行）
	metrics := []struct {
		label string
		value string
	}{
		{"💰 总花费", fmt.Sprintf("$%.2f", m.aggregated.TotalSpend)},
		{"📤 请求", fmt.Sprintf("%d", m.aggregated.TotalRequests)},
		{"✅ 成功", fmt.Sprintf("%d", m.aggregated.Successful)},
		{"❌ 失败", fmt.Sprintf("%d", m.aggregated.Failed)},
		{"📊 Tokens", formatTokens(m.aggregated.TotalTokens)},
		{"📈 均费", fmt.Sprintf("$%.4f", m.aggregated.AvgCostPerReq)},
	}

	// 动态列数：大屏3列，中屏2列，小屏1列
	cols := 3
	if width < 80 {
		cols = 2
	}
	if width < 40 {
		cols = 1
	}

	// 更短的卡片宽度
	cardWidth := (width / cols) - 1
	if cardWidth < 14 {
		cardWidth = 14
	}
	if cardWidth > 22 {
		cardWidth = 22
	}

	// 渲染紧凑卡片（图标+标签+数值单行显示，无边框高度）
	for row := 0; row < len(metrics); row += cols {
		var rowCards []string
		for col := 0; col < cols && row+col < len(metrics); col++ {
			metric := metrics[row+col]
			// 紧凑格式：标签 + 数值（单行）
			card := labelStyle.Render(metric.label) + " " + valueStyle.Render(metric.value)
			rowCards = append(rowCards, cardStyle.Width(cardWidth).Render(card))
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, rowCards...))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *Model) renderBarContent(width int) string {
	var sb strings.Builder

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("76"))

	barFocusedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)

	dateLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("159")).
		Bold(true)

	spendLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("159"))

	var maxSpend float64
	for _, r := range m.data {
		if r.Metrics.Spend > maxSpend {
			maxSpend = r.Metrics.Spend
		}
	}

	// 水平柱状图：计算进度条可用宽度
	// 布局：[日期] [████████░░░░] $XX.XX
	// 日期占 12 字符，金额占 10 字符，预留 2 字符间距
	labelWidth := 12
	spendWidth := 10
	barAvailableWidth := width - labelWidth - spendWidth - 4
	if barAvailableWidth < 8 {
		barAvailableWidth = 8
	}
	if barAvailableWidth > 50 {
		barAvailableWidth = 50
	}

	// 渲染每日的水平进度条
	for i, r := range m.data {
		isSelected := i == m.selectedBarIndex

		// 计算进度条宽度
		var barWidth int
		if maxSpend > 0 {
			barWidth = int(float64(barAvailableWidth) * r.Metrics.Spend / maxSpend)
		}
		barStr := strings.Repeat("█", barWidth)
		barStr += strings.Repeat("░", barAvailableWidth-barWidth)

		// 格式化日期（简化显示）
		dateStr := r.Date
		if len(dateStr) > 10 {
			dateStr = dateStr[5:] // 只显示 MM-DD
		}

		// 渲染行：[日期] [████████░░] $XX.XX
		if isSelected {
			sb.WriteString(selectedStyle.Render("▶ "))
			sb.WriteString(dateLabelStyle.Render(fmt.Sprintf("%-12s", dateStr)))
			sb.WriteString(" ")
			sb.WriteString(barFocusedStyle.Render(barStr))
			sb.WriteString(" ")
			sb.WriteString(spendLabelStyle.Render(fmt.Sprintf("$%.2f", r.Metrics.Spend)))
		} else {
			sb.WriteString("  ")
			sb.WriteString(dateLabelStyle.Render(fmt.Sprintf("%-12s", dateStr)))
			sb.WriteString(" ")
			sb.WriteString(barStyle.Render(barStr))
			sb.WriteString(" ")
			sb.WriteString(spendLabelStyle.Render(fmt.Sprintf("$%.2f", r.Metrics.Spend)))
		}

		sb.WriteString("\n")
	}

	// 选中项详情面板（显示在底部）
	if m.selectedBarIndex >= 0 && m.selectedBarIndex < len(m.data) {
		sb.WriteString("\n")
		sb.WriteString(m.renderDetailPanelCompact(m.data[m.selectedBarIndex], width))
	}

	return sb.String()
}

// renderDetailPanelCompact 显示选中日期的超紧凑详情面板（单行）
func (m *Model) renderDetailPanelCompact(data api.UserDailyActivity, width int) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("159"))

	// 单行显示所有指标
	detail := keyStyle.Render("📅 ") + valueStyle.Render(fmt.Sprintf("%s ", data.Date))
	detail += keyStyle.Render("💰 ") + valueStyle.Render(fmt.Sprintf("$%.2f ", data.Metrics.Spend))
	detail += keyStyle.Render("📤 ") + valueStyle.Render(fmt.Sprintf("%d ", data.Metrics.APIRequests))
	detail += keyStyle.Render("✅ ") + valueStyle.Render(fmt.Sprintf("%d ", data.Metrics.SuccessfulRequests))
	detail += keyStyle.Render("❌ ") + valueStyle.Render(fmt.Sprintf("%d ", data.Metrics.FailedRequests))
	detail += keyStyle.Render("📊 ") + valueStyle.Render(formatTokens(data.Metrics.TotalTokens))

	return detail
}

func (m *Model) renderDetailPanel(data api.UserDailyActivity, width int) string {
	var sb strings.Builder

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("159"))

	content := titleStyle.Render(fmt.Sprintf("📅 %s", data.Date)) + "\n"
	content += keyStyle.Render("💰 花费: ") + valueStyle.Render(fmt.Sprintf("$%.4f", data.Metrics.Spend)) + "\n"
	content += keyStyle.Render("📤 请求: ") + valueStyle.Render(fmt.Sprintf("%d", data.Metrics.APIRequests)) + "\n"
	content += keyStyle.Render("✅ 成功: ") + valueStyle.Render(fmt.Sprintf("%d", data.Metrics.SuccessfulRequests)) + "\n"
	content += keyStyle.Render("❌ 失败: ") + valueStyle.Render(fmt.Sprintf("%d", data.Metrics.FailedRequests)) + "\n"
	content += keyStyle.Render("📝 Prompt: ") + valueStyle.Render(formatTokens(data.Metrics.PromptTokens)) + "\n"
	content += keyStyle.Render("✍️ Completion: ") + valueStyle.Render(formatTokens(data.Metrics.CompletionTokens)) + "\n"
	content += keyStyle.Render("📊 总 Tokens: ") + valueStyle.Render(formatTokens(data.Metrics.TotalTokens))

	// 动态计算详情面板的宽度
	panelWidth := int(float64(width) * 0.7)
	if panelWidth < 30 {
		panelWidth = 30
	}
	if panelWidth > 50 {
		panelWidth = 50
	}

	sb.WriteString(panelStyle.Width(panelWidth).Render(content))

	return sb.String()
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	} else if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// ShowHeader 控制是否显示顶部 header
func (m *Model) ShowHeader(show bool) {
	m.showHeader = show
}
