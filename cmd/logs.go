package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"litellm-cli/internal/api"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var (
	interval int
	model    string
	verbose  bool
)

// detailTabs 定义详情视图的 tab 页面
var detailTabs = []string{"main", "system", "tools", "messages", "choices"}

// detailMainSections 定义主视图的区块
var detailMainSections = []string{"request", "response"}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "轮询查看日志 (TUI)",
	Run:   runLogs,
}

func init() {
	logsCmd.Flags().IntVarP(&interval, "interval", "i", 5, "刷新间隔 (秒)")
	logsCmd.Flags().StringVarP(&model, "model", "m", "", "过滤模型")
	logsCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "详细日志模式")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) {
	// 设置日志输出
	if verbose {
		logFile, err := os.OpenFile("litellm-cli.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal("无法创建日志文件:", err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
		log.Println("=== LiteLLM CLI 启动 ===")
	}

	cfg, err := config.LoadWithAutoLogin()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	p := tea.NewProgram(
		NewLogsModel(c, interval, model),
		tea.WithAltScreen(),
	)

	// 处理退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		p.Send(tea.Quit())
	}()

	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}

// 文本模式 - 用于非交互环境
func runLogsText(c *client.Client, interval int, model string) {
	// 创建退出信号通道
	quitChan := make(chan struct{})

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		close(quitChan)
	}()

	// 监听键盘输入 (q 退出)
	go func() {
		var buf [1]byte
		for {
			n, err := os.Stdin.Read(buf[:])
			if err != nil {
				return
			}
			if n > 0 && buf[0] == 'q' {
				close(quitChan)
				return
			}
		}
	}()

	tick := 0
	for {
		clearScreen()
		printLogs(c, model, tick)
		tick++

		select {
		case <-quitChan:
			fmt.Println("\n👋 已退出")
			return
		case <-time.After(time.Duration(interval) * time.Second):
			continue
		}
	}
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// formatLocalTime 将 UTC 时间转换为本地时区显示
func formatLocalTime(utcTime string) string {
	if len(utcTime) >= 19 {
		// 解析 UTC 时间
		t, err := time.Parse("2006-01-02T15:04:05", utcTime[:19])
		if err == nil {
			// 转换为本地时区并格式化为字符串
			return t.Local().Format("2006-01-02 15:04")
		}
		// 解析失败则回退到简单替换
		fallback := utcTime[:19]
		fallback = strings.Replace(fallback, "T", " ", 1)
		return fallback
	}
	return utcTime
}

func printLogs(c *client.Client, model string, tick int) {
	// 使用 datetime 格式，并 URL 编码空格
	endDate := url.QueryEscape(time.Now().Format("2006-01-02 15:04:05"))
	startDate := url.QueryEscape(time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05"))

	// 优先使用 /spend/logs/ui (需要 JWT token)
	resp, err := c.GetSpendLogsUI(startDate, endDate)
	if err != nil {
		// 如果失败，回退到旧的 /spend/logs
		respOld, err2 := c.GetSpendLogs(
			time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
			time.Now().Format("2006-01-02"),
		)
		if err2 != nil {
			fmt.Printf("❌ 获取失败: %v\n", err)
			return
		}
		printSpendLogs(respOld, tick, model)
		return
	}

	printSpendLogsUI(resp, tick, model)
}

func printSpendLogsUI(resp *api.SpendLogsUIResponse, tick int, modelFilter string) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	// 辅助函数：按显示宽度填充
	padRight := func(s string, width int) string {
		w := runewidth.StringWidth(s)
		if w >= width {
			return s
		}
		return s + strings.Repeat(" ", width-w)
	}

	fmt.Println(headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 实时日志 (刷新: %ds) | 按 q 退出 ", interval)))
	fmt.Println()

	if resp == nil || len(resp.Data) == 0 {
		fmt.Println(contentStyle.Render("暂无数据"))
	} else {
		// 先过滤数据
		var filteredData []api.SpendLogEntry
		for _, entry := range resp.Data {
			if modelFilter != "" && !strings.Contains(entry.Model, modelFilter) {
				continue
			}
			filteredData = append(filteredData, entry)
		}

		// 自动计算每列的最大宽度
		colWidths := struct {
			time   int
			status int
			spend  int
			latency int
			tokens  int
			model   int
			tags    int
		}{
			time:   runewidth.StringWidth("时间"),
			status: runewidth.StringWidth("状态"),
			spend:  runewidth.StringWidth("费用"),
			latency: runewidth.StringWidth("耗时"),
			tokens: runewidth.StringWidth("Tokens"),
			model:  runewidth.StringWidth("模型"),
			tags:   runewidth.StringWidth("Tags"),
		}

		for _, entry := range filteredData {
			// 时间
			startTime := formatLocalTime(entry.StartTime)
			colWidths.time = max(colWidths.time, runewidth.StringWidth(startTime))

			// 状态
			status := "✓"
			if entry.Status != "success" && entry.ErrorMessage != "" {
				status = "✗"
			}
			colWidths.status = max(colWidths.status, runewidth.StringWidth(status))

			// 费用
			spendStr := "-"
			if entry.TotalSpend > 0 {
				spendStr = fmt.Sprintf("$%.2f", entry.TotalSpend)
			}
			colWidths.spend = max(colWidths.spend, runewidth.StringWidth(spendStr))

			// 耗时
			latencyStr := "-"
			if entry.StartTime != "" && entry.EndTime != "" {
				start, err := time.Parse(time.RFC3339, entry.StartTime)
				if err == nil {
					end, err := time.Parse(time.RFC3339, entry.EndTime)
					if err == nil {
						duration := end.Sub(start)
						if duration > 0 {
							latencyStr = fmt.Sprintf("%.2fs", duration.Seconds())
						}
					}
				}
			}
			colWidths.latency = max(colWidths.latency, runewidth.StringWidth(latencyStr))

			// Tokens
			tokensStr := "-"
			if entry.TotalTokens > 0 {
				tokensStr = fmt.Sprintf("%d(%d+%d)", entry.TotalTokens, entry.PromptTokens, entry.CompletionTokens)
			}
			colWidths.tokens = max(colWidths.tokens, runewidth.StringWidth(tokensStr))

			// 模型
			model := entry.ModelGroup
			if model == "" {
				model = entry.Model
			}
			colWidths.model = max(colWidths.model, runewidth.StringWidth(model))

			// Tags
			tag := ""
			if len(entry.RequestTags) > 0 {
				tags := entry.RequestTags
				if len(tags) > 1 {
					sort.Slice(tags, func(i, j int) bool {
						return len(tags[i]) < len(tags[j])
					})
					longest := tags[len(tags)-1]
					longest = strings.TrimPrefix(longest, "User-Agent: ")
					if idx := strings.Index(longest, "("); idx != -1 {
						longest = longest[:idx]
					}
					tag = strings.TrimSpace(longest)
				} else {
					tag = tags[0]
				}
			}
			colWidths.tags = max(colWidths.tags, runewidth.StringWidth(tag))
		}

		// 打印表头
		fmt.Println(headerStyle.Render(fmt.Sprintf("%s %s %s %s %s %s %s",
			padRight("时间", colWidths.time),
			padRight("状态", colWidths.status),
			padRight("费用", colWidths.spend),
			padRight("耗时", colWidths.latency),
			padRight("Tokens", colWidths.tokens),
			padRight("模型", colWidths.model),
			padRight("Tags", colWidths.tags))))

		// 打印分隔线
		totalWidth := colWidths.time + colWidths.status + colWidths.spend + colWidths.latency + colWidths.tokens + colWidths.model + colWidths.tags + 6
		fmt.Println(mutedStyle.Render(strings.Repeat("─", totalWidth)))

		// 打印数据
		for _, entry := range filteredData {
			// 时间
			startTime := formatLocalTime(entry.StartTime)
			startTime = padRight(startTime, colWidths.time)

			// 状态
			status := "✓"
			if entry.Status != "success" && entry.ErrorMessage != "" {
				status = "✗"
			}
			status = padRight(status, colWidths.status)

			// 费用
			spendStr := "-"
			if entry.TotalSpend > 0 {
				spendStr = fmt.Sprintf("$%.2f", entry.TotalSpend)
			}
			spendStr = padRight(spendStr, colWidths.spend)

			// 耗时
			latencyStr := "-"
			if entry.StartTime != "" && entry.EndTime != "" {
				start, err := time.Parse(time.RFC3339, entry.StartTime)
				if err == nil {
					end, err := time.Parse(time.RFC3339, entry.EndTime)
					if err == nil {
						duration := end.Sub(start)
						if duration > 0 {
							latencyStr = fmt.Sprintf("%.2fs", duration.Seconds())
						}
					}
				}
			}
			latencyStr = padRight(latencyStr, colWidths.latency)

			// Tokens
			var tokensStr string
			if entry.TotalTokens > 0 {
				tokensStr = fmt.Sprintf("%d(%d+%d)", entry.TotalTokens, entry.PromptTokens, entry.CompletionTokens)
			} else {
				tokensStr = "-"
			}
			tokensStr = padRight(tokensStr, colWidths.tokens)

			// 模型
			model := entry.ModelGroup
			if model == "" {
				model = entry.Model
			}
			model = padRight(model, colWidths.model)

			// Tags
			tag := ""
			if len(entry.RequestTags) > 0 {
				tags := entry.RequestTags
				if len(tags) > 1 {
					sort.Slice(tags, func(i, j int) bool {
						return len(tags[i]) < len(tags[j])
					})
					longest := tags[len(tags)-1]
					longest = strings.TrimPrefix(longest, "User-Agent: ")
					if idx := strings.Index(longest, "("); idx != -1 {
						longest = longest[:idx]
					}
					tag = strings.TrimSpace(longest)
				} else {
					tag = tags[0]
				}
			}

			// 打印行
			if entry.Status != "success" && entry.ErrorMessage != "" {
				fmt.Printf("%s %s %s %s %s %s %s\n",
					contentStyle.Render(startTime),
					errorStyle.Render(status),
					greenStyle.Render(spendStr),
					yellowStyle.Render(latencyStr),
					contentStyle.Render(tokensStr),
					contentStyle.Render(model),
					mutedStyle.Render(tag))
			} else {
				fmt.Printf("%s %s %s %s %s %s %s\n",
					contentStyle.Render(startTime),
					greenStyle.Render(status),
					greenStyle.Render(spendStr),
					yellowStyle.Render(latencyStr),
					contentStyle.Render(tokensStr),
					contentStyle.Render(model),
					mutedStyle.Render(tag))
			}
		}

		fmt.Println()
		fmt.Println(mutedStyle.Render(fmt.Sprintf("共 %d 条记录 (总 %d)", len(filteredData), resp.Total)))
	}

	fmt.Println()
	fmt.Printf("⏱ 更新次数: %d | 时间: %s\n", tick, time.Now().Format("15:04:05"))
}

// printSpendLogs 回退使用的旧格式显示
func printSpendLogs(resp *api.SpendLogsResponse, tick int, modelFilter string) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))

	fmt.Println(headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 日志 (刷新: %ds) | Ctrl+C 退出 ", interval)))
	fmt.Println()

	if resp == nil || len(*resp) == 0 {
		fmt.Println(contentStyle.Render("暂无数据"))
	} else {
		for _, entry := range *resp {
			spendVal, hasSpend := entry["spend"]
			if hasSpend {
				spend, _ := spendVal.(float64)

				keyLabel := "当前 Key"
				if len(entry) > 0 {
					for k := range entry {
						if k != "spend" && k != "models" && k != "users" && k != "startTime" {
							keyLabel = k
							break
						}
					}
				}
				if len(keyLabel) > 12 {
					keyLabel = keyLabel[:8] + "..."
				}

				fmt.Printf(contentStyle.Render("📦 %s "), keyLabel)
				if spend > 0 {
					fmt.Printf("%s", greenStyle.Render(fmt.Sprintf("$%.4f ", spend)))
				}
				fmt.Println()
			}
		}

		fmt.Println()
		fmt.Println(mutedStyle.Render(fmt.Sprintf("共 %d 条记录", len(*resp))))
	}

	fmt.Println()
	fmt.Printf("⏱ 更新次数: %d | 时间: %s\n", tick, time.Now().Format("15:04:05"))
	fmt.Println(mutedStyle.Render("\n提示: 使用 --text 或 -t 参数可在非交互环境运行 (回退模式)"))
}

// renderLogsTable 渲染日志表格 (用于 TUI 模式)
func renderLogsTable(data []api.SpendLogEntry, total int, newLogIDs map[string]bool, maxRows int, selectedIndex int) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	// 新记录高亮样式 (青色粗体)
	newHighlightStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	newHighlightMutedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	// 选中行样式 (反色背景)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("86"))
	selectedMutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("86")).Bold(true)

	padRight := func(s string, width int) string {
		w := runewidth.StringWidth(s)
		if w >= width {
			return s
		}
		return s + strings.Repeat(" ", width-w)
	}

	var sb strings.Builder

	// 自动计算列宽
	colWidths := struct {
		time    int
		status  int
		spend   int
		latency int
		tokens  int
		model   int
		tags    int
	}{
		time:    runewidth.StringWidth("时间"),
		status:  runewidth.StringWidth("状态"),
		spend:   runewidth.StringWidth("费用"),
		latency: runewidth.StringWidth("耗时"),
		tokens:  runewidth.StringWidth("Tokens"),
		model:   runewidth.StringWidth("模型"),
		tags:    runewidth.StringWidth("Tags"),
	}

	for _, entry := range data {
		startTime := formatLocalTime(entry.StartTime)
		colWidths.time = max(colWidths.time, runewidth.StringWidth(startTime))

		status := "✓"
		if entry.Status != "success" && entry.ErrorMessage != "" {
			status = "✗"
		}
		colWidths.status = max(colWidths.status, runewidth.StringWidth(status))

		spendStr := "-"
		if entry.TotalSpend > 0 {
			spendStr = fmt.Sprintf("$%.2f", entry.TotalSpend)
		}
		colWidths.spend = max(colWidths.spend, runewidth.StringWidth(spendStr))

		latencyStr := "-"
		if entry.StartTime != "" && entry.EndTime != "" {
			start, err := time.Parse(time.RFC3339, entry.StartTime)
			if err == nil {
				end, err := time.Parse(time.RFC3339, entry.EndTime)
				if err == nil {
					duration := end.Sub(start)
					if duration > 0 {
						latencyStr = fmt.Sprintf("%.2fs", duration.Seconds())
					}
				}
			}
		}
		colWidths.latency = max(colWidths.latency, runewidth.StringWidth(latencyStr))

		tokensStr := "-"
		if entry.TotalTokens > 0 {
			tokensStr = fmt.Sprintf("%d(%d+%d)", entry.TotalTokens, entry.PromptTokens, entry.CompletionTokens)
		}
		colWidths.tokens = max(colWidths.tokens, runewidth.StringWidth(tokensStr))

		model := entry.ModelGroup
		if model == "" {
			model = entry.Model
		}
		colWidths.model = max(colWidths.model, runewidth.StringWidth(model))

		tag := ""
		if len(entry.RequestTags) > 0 {
			tags := entry.RequestTags
			if len(tags) > 1 {
				sort.Slice(tags, func(i, j int) bool { return len(tags[i]) < len(tags[j]) })
				longest := tags[len(tags)-1]
				longest = strings.TrimPrefix(longest, "User-Agent: ")
				if idx := strings.Index(longest, "("); idx != -1 {
					longest = longest[:idx]
				}
				tag = strings.TrimSpace(longest)
			} else {
				tag = tags[0]
			}
		}
		colWidths.tags = max(colWidths.tags, runewidth.StringWidth(tag))
	}

	// 打印表头
	sb.WriteString(headerStyle.Render(fmt.Sprintf("%s %s %s %s %s %s %s",
		padRight("时间", colWidths.time),
		padRight("状态", colWidths.status),
		padRight("费用", colWidths.spend),
		padRight("耗时", colWidths.latency),
		padRight("Tokens", colWidths.tokens),
		padRight("模型", colWidths.model),
		padRight("Tags", colWidths.tags))) + "\n")

	// 分隔线
	totalWidth := colWidths.time + colWidths.status + colWidths.spend + colWidths.latency + colWidths.tokens + colWidths.model + colWidths.tags + 6
	sb.WriteString(mutedStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	// 打印数据
	rowCount := 0
	for i, entry := range data {
		// 限制显示行数，预留2行给表头和分隔线
		if maxRows > 0 && rowCount >= maxRows-2 {
			sb.WriteString(mutedStyle.Render(fmt.Sprintf("\n... 还有 %d 条记录 (总 %d)", len(data)-rowCount, total)))
			break
		}
		rowCount++

		// 判断是否被选中
		isSelected := i == selectedIndex

		startTime := formatLocalTime(entry.StartTime)

		// 判断是否是新记录
		isNew := newLogIDs != nil && newLogIDs[entry.ID]

		status := "✓"
		if entry.Status != "success" && entry.ErrorMessage != "" {
			status = "✗"
		}

		spendStr := "-"
		if entry.TotalSpend > 0 {
			spendStr = fmt.Sprintf("$%.2f", entry.TotalSpend)
		}

		latencyStr := "-"
		if entry.StartTime != "" && entry.EndTime != "" {
			start, _ := time.Parse(time.RFC3339, entry.StartTime)
			end, _ := time.Parse(time.RFC3339, entry.EndTime)
			duration := end.Sub(start)
			if duration > 0 {
				latencyStr = fmt.Sprintf("%.2fs", duration.Seconds())
			}
		}

		tokensStr := "-"
		if entry.TotalTokens > 0 {
			tokensStr = fmt.Sprintf("%d(%d+%d)", entry.TotalTokens, entry.PromptTokens, entry.CompletionTokens)
		}

		model := entry.ModelGroup
		if model == "" {
			model = entry.Model
		}

		tag := ""
		if len(entry.RequestTags) > 0 {
			tags := entry.RequestTags
			if len(tags) > 1 {
				sort.Slice(tags, func(i, j int) bool { return len(tags[i]) < len(tags[j]) })
				longest := tags[len(tags)-1]
				longest = strings.TrimPrefix(longest, "User-Agent: ")
				if idx := strings.Index(longest, "("); idx != -1 {
					longest = longest[:idx]
				}
				tag = strings.TrimSpace(longest)
			} else {
				tag = tags[0]
			}
		}

		// 判断样式
		var timeStyle, statusStyle, spendStyle, latencyStyle, tokensStyle, modelStyle, tagStyle lipgloss.Style

		if isSelected {
			// 选中行使用反色样式
			timeStyle = selectedStyle
			statusStyle = selectedStyle
			spendStyle = selectedStyle
			latencyStyle = selectedStyle
			tokensStyle = selectedStyle
			modelStyle = selectedStyle
			tagStyle = selectedMutedStyle
		} else if isNew {
			// 新记录使用高亮样式
			timeStyle = newHighlightStyle
			statusStyle = newHighlightStyle
			spendStyle = newHighlightStyle
			latencyStyle = newHighlightStyle
			tokensStyle = newHighlightStyle
			modelStyle = newHighlightStyle
			tagStyle = newHighlightMutedStyle
		} else if entry.Status != "success" && entry.ErrorMessage != "" {
			timeStyle = contentStyle
			statusStyle = errorStyle
			spendStyle = greenStyle
			latencyStyle = yellowStyle
			tokensStyle = contentStyle
			modelStyle = contentStyle
			tagStyle = mutedStyle
		} else {
			timeStyle = contentStyle
			statusStyle = greenStyle
			spendStyle = greenStyle
			latencyStyle = yellowStyle
			tokensStyle = contentStyle
			modelStyle = contentStyle
			tagStyle = mutedStyle
		}

		sb.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s\n",
			timeStyle.Render(padRight(startTime, colWidths.time)),
			statusStyle.Render(padRight(status, colWidths.status)),
			spendStyle.Render(padRight(spendStr, colWidths.spend)),
			latencyStyle.Render(padRight(latencyStr, colWidths.latency)),
			tokensStyle.Render(padRight(tokensStr, colWidths.tokens)),
			modelStyle.Render(padRight(model, colWidths.model)),
			tagStyle.Render(padRight(tag, colWidths.tags))))
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", mutedStyle.Render(fmt.Sprintf("共 %d 条记录 (总 %d)", len(data), total))))

	return sb.String()
}

// renderLogsTableOld 渲染旧版日志表格 (用于 TUI 模式回退)
func renderLogsTableOld(resp *api.SpendLogsResponse, intervalVal int, newLogIDs map[string]bool, maxRows int, selectedIndex int) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))

	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 日志 (刷新: %ds) | Ctrl+C 退出 ", intervalVal)) + "\n\n")

	rowCount := 0
	for _, entry := range *resp {
		// 限制显示行数
		if maxRows > 0 && rowCount >= maxRows {
			sb.WriteString(mutedStyle.Render(fmt.Sprintf("\n... 还有 %d 条记录", len(*resp)-rowCount)))
			break
		}

		spendVal, hasSpend := entry["spend"]
		if hasSpend {
			spend, _ := spendVal.(float64)

			keyLabel := "当前 Key"
			if len(entry) > 0 {
				for k := range entry {
					if k != "spend" && k != "models" && k != "users" && k != "startTime" {
						keyLabel = k
						break
					}
				}
			}
			if len(keyLabel) > 12 {
				keyLabel = keyLabel[:8] + "..."
			}

			sb.WriteString(contentStyle.Render(fmt.Sprintf("📦 %s ", keyLabel)))
			if spend > 0 {
				sb.WriteString(greenStyle.Render(fmt.Sprintf("$%.4f ", spend)))
			}
			sb.WriteString("\n")
			rowCount++
		}
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", mutedStyle.Render(fmt.Sprintf("共 %d 条记录", len(*resp)))))
	return sb.String()
}

// TUI 模式
type logsModel struct {
	client        *client.Client
	data          string
	interval      int
	model         string
	tick          int
	quitting      bool
	logData       *api.SpendLogsUIResponse
	logDataOld    *api.SpendLogsResponse
	seenLogIDs    map[string]bool // 已看到的日志ID
	newLogIDs     map[string]bool // 本次新增的日志ID（用于高亮）
	initialized   bool            // 是否已完成首次加载
	width         int             // 窗口宽度
	height        int             // 窗口高度
	selectedIndex int            // 当前选中的日志索引
	selectedEntry *api.SpendLogEntry // 当前选中的日志条目（用于详情页）
	viewMode      string          // "list" 或 "detail"
	detailData    map[string]interface{}
	detailError   string
	detailScroll  int // 详情视图滚动偏移量
	detailState   *detailViewState // 详情视图状态（展开/折叠）
}

func NewLogsModel(c *client.Client, interval int, model string) *logsModel {
	m := &logsModel{
		client:       c,
		interval:     interval,
		model:        model,
		data:         "加载中...",
		seenLogIDs:   make(map[string]bool),
		newLogIDs:    make(map[string]bool),
		width:        120,  // 默认宽度
		height:       40,   // 默认高度
		viewMode:     "list", // 默认视图模式
		selectedIndex: 0,
	}
	m.refresh()
	return m
}

func (m *logsModel) Init() tea.Cmd {
	return tea.Tick(time.Duration(m.interval)*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *logsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			// 返回列表视图或上一个 tab
			if m.viewMode == "detail" && m.detailState != nil {
				if m.detailState.activeTab != "main" {
					// 从详情 tab 返回主视图
					m.detailState.activeTab = "main"
					m.detailState.selectedItem = 0
					m.detailState.scrollOffset = 0
				} else {
					// 从主视图返回列表视图
					m.viewMode = "list"
					m.detailData = nil
					m.detailError = ""
					m.detailScroll = 0
					m.detailState = nil
				}
			}
			return m, nil
		case "enter":
			// Enter: 进入详情 tab 或选中数组项
			if m.viewMode == "list" {
				m.detailScroll = 0
				m.detailState = nil
				cmd := m.loadDetail()
				return m, cmd
			} else if m.viewMode == "detail" && m.detailState != nil {
				if m.detailState.activeTab == "main" {
					// 主视图：进入选中的详情 tab
					// focusedSection: 0=system, 1=tools, 2=messages, 3=choices
					tabMap := map[int]string{
						0: "system",
						1: "tools",
						2: "messages",
						3: "choices",
					}
					m.detailState.activeTab = tabMap[m.detailState.focusedSection]
					m.detailState.selectedItem = 0
					m.detailState.scrollOffset = 0
				} else {
					// 详情视图：展开/折叠当前选中的数组项
					tab := m.detailState.activeTab
					key := fmt.Sprintf("%s_%d", tab, m.detailState.selectedItem)
					m.detailState.expandedSections[key] = !m.detailState.expandedSections[key]
				}
			}
			return m, nil
		case "tab":
			// Tab: 切换 tab 页面或展开/折叠
			if m.viewMode == "detail" && m.detailState != nil {
				if m.detailState.activeTab == "main" {
					// 主视图：切换聚焦 (0-3: system/tools/messages/choices)
					m.detailState.focusedSection = (m.detailState.focusedSection + 1) % 4
				} else {
					// 详情视图：在数组项之间切换
					maxItems := m.getTabItemCount(m.detailState.activeTab)
					if maxItems > 0 {
						m.detailState.selectedItem = (m.detailState.selectedItem + 1) % maxItems
					}
				}
			}
			return m, nil
		case "up", "k", "ctrl+p", "\x1b[A":
			// 上移选择或切换聚焦区块
			if m.viewMode == "list" {
				if m.selectedIndex > 0 {
					m.selectedIndex--
				}
			} else if m.viewMode == "detail" && m.detailState != nil {
				if m.detailState.activeTab == "main" {
					// 主视图：切换聚焦 (0-3: system/tools/messages/choices)
					m.detailState.focusedSection = (m.detailState.focusedSection - 1 + 4) % 4
				} else {
					// 详情视图：在数组项之间切换
					maxItems := m.getTabItemCount(m.detailState.activeTab)
					if maxItems > 0 {
						m.detailState.selectedItem = (m.detailState.selectedItem - 1 + maxItems) % maxItems
					}
				}
				m.detailState.scrollOffset = 0
			}
			return m, nil
		case "down", "j", "ctrl+n", "\x1b[B":
			// 下移选择或切换聚焦区块
			if m.viewMode == "list" {
				maxIdx := -1
				if m.logData != nil && len(m.logData.Data) > 0 {
					maxIdx = len(m.logData.Data) - 1
				} else if m.logDataOld != nil && len(*m.logDataOld) > 0 {
					maxIdx = len(*m.logDataOld) - 1
				}
				if maxIdx >= 0 && m.selectedIndex < maxIdx {
					m.selectedIndex++
				}
			} else if m.viewMode == "detail" && m.detailState != nil {
				if m.detailState.activeTab == "main" {
					// 主视图：切换聚焦 (0-3: system/tools/messages/choices)
					m.detailState.focusedSection = (m.detailState.focusedSection + 1) % 4
				} else {
					// 详情视图：在数组项之间切换
					maxItems := m.getTabItemCount(m.detailState.activeTab)
					if maxItems > 0 {
						m.detailState.selectedItem = (m.detailState.selectedItem + 1) % maxItems
					}
				}
				m.detailState.scrollOffset = 0
			}
			return m, nil
		case "pgup", "\x1b[5~":
			// 详情视图向上翻页
			if m.viewMode == "detail" {
				m.detailScroll = max(0, m.detailScroll-20)
			}
			return m, nil
		case "pgdown", "\x1b[6~":
			// 详情视图向下翻页
			if m.viewMode == "detail" {
				m.detailScroll += 20
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.refresh()
		m.tick++
		return m, tea.Tick(time.Duration(m.interval)*time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	case detailLoadedMsg:
		if msg.error != "" {
			m.detailError = msg.error
		} else {
			m.detailData = msg.data
			m.detailError = ""
		}
		return m, nil
	}
	return m, nil
}

func (m *logsModel) View() string {
	if m.quitting {
		return "👋 已退出\n"
	}

	// 详情视图
	if m.viewMode == "detail" {
		return m.renderDetailView()
	}

	// 列表视图
	return m.renderListView()
}

// detailViewState 保存详情视图的状态
type detailViewState struct {
	activeTab          string              // 当前 tab: "main", "system", "tools", "messages", "choices"
	expandedSections   map[string]bool     // 展开的区块
	focusedSection     int                 // 当前聚焦的区块索引
	selectedItem       int                 // 选中的数组项索引（用于详情tab）
	scrollOffset       int                 // 滚动偏移量
}

func (m *logsModel) renderDetailView() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("236"))

	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).MarginRight(1)
	groupStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("210"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("159"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))

	// 卡片样式
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("236")).
		Padding(0, 1)

	// 聚焦卡片样式（高亮边框）
	focusedCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(0, 1)

	// 初始化状态（如果需要）
	if m.detailState == nil {
		m.detailState = &detailViewState{
			activeTab:        "main",
			expandedSections: make(map[string]bool),
			focusedSection:  0,
			selectedItem:    0,
			scrollOffset:    0,
		}
	}

	// 获取数据
	proxyReq, _ := m.detailData["proxy_server_request"].(map[string]interface{})
	respData, _ := m.detailData["response"].(map[string]interface{})

	var lines []string

	// 渲染头部
	if m.detailState.activeTab == "main" {
		lines = append(lines, headerStyle.Render(" 📋 日志详情 | ESC 返回 | ↑↓ 切换 | Tab 切换 | Enter 进入 "))
	} else {
		tabTitle := map[string]string{
			"system":   "System Messages",
			"tools":    "Tools",
			"messages": "Messages",
			"choices":  "Choices",
		}[m.detailState.activeTab]
		lines = append(lines, headerStyle.Render(fmt.Sprintf(" 📋 日志详情 > %s | ESC 返回 | ↑↓ 选择 | Enter 展开 ", tabTitle)))
	}
	lines = append(lines, "")

	// 加载状态
	if m.detailError != "" {
		lines = append(lines, contentStyle.Render(m.detailError))
		if m.detailError == "加载中..." {
			lines = append(lines, mutedStyle.Render(" ⏳"))
		}
		return strings.Join(lines, "\n")
	}

	if m.detailData == nil {
		lines = append(lines, mutedStyle.Render("无详情数据，请按 Enter 刷新"))
		return strings.Join(lines, "\n")
	}

	// 根据当前 tab 渲染不同内容
	if m.detailState.activeTab == "main" {
		lines = append(lines, m.renderMainView(proxyReq, respData, cardStyle, focusedCardStyle, contentStyle, mutedStyle, groupStyle, valueStyle, keyStyle, infoStyle, successStyle)...)
	} else {
		lines = append(lines, m.renderArrayDetailView(proxyReq, respData, cardStyle, focusedCardStyle, contentStyle, mutedStyle, groupStyle, valueStyle, keyStyle)...)
	}

	// 底部提示
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("提示: ↑↓ 切换 | Tab 切换 | Enter 进入/展开 | ESC 返回"))

	// 计算滚动
	scrollOffset := m.detailState.scrollOffset
	maxDisplayLines := m.height - 3
	if maxDisplayLines < 10 {
		maxDisplayLines = 20
	}

	totalLines := len(lines)
	if scrollOffset > totalLines-maxDisplayLines {
		scrollOffset = max(0, totalLines-maxDisplayLines)
		m.detailState.scrollOffset = scrollOffset
	}

	endLine := scrollOffset + maxDisplayLines
	if endLine > totalLines {
		endLine = totalLines
	}
	visibleLines := lines[scrollOffset:endLine]

	// 构建最终输出
	var sb strings.Builder
	for i, line := range visibleLines {
		sb.WriteString(line)
		if i < len(visibleLines)-1 {
			sb.WriteString("\n")
		}
	}

	// 添加滚动指示器
	if scrollOffset > 0 || endLine < totalLines {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render(fmt.Sprintf(" ──◀ %d/%d ▶─ ", scrollOffset+1, totalLines)))
	}

	return sb.String()
}

// renderMainView 渲染主视图（Request/Response 分栏）
func (m *logsModel) renderMainView(proxyReq, respData map[string]interface{}, cardStyle, focusedCardStyle, contentStyle, mutedStyle, groupStyle, valueStyle, keyStyle, infoStyle, successStyle lipgloss.Style) []string {
	var lines []string

	// 判断宽屏还是窄屏
	isWideScreen := m.width >= 100

	// 统计各项数量 - 使用正确的字段
	systemCount := 0
	toolsCount := 0
	messagesCount := 0
	if proxyReq != nil {
		// system 是独立的字段，不是从 messages 中过滤
		if system, ok := proxyReq["system"].([]interface{}); ok {
			systemCount = len(system)
		}
		// messages 是独立的字段
		if messages, ok := proxyReq["messages"].([]interface{}); ok {
			messagesCount = len(messages)
		}
		// tools 是独立的字段
		if tools, ok := proxyReq["tools"].([]interface{}); ok {
			toolsCount = len(tools)
		}
	}

	choicesCount := 0
	if respData != nil {
		if choices, ok := respData["choices"].([]interface{}); ok {
			choicesCount = len(choices)
		}
	}

	// 获取 Model
	modelName := "-"
	if proxyReq != nil {
		if model, ok := proxyReq["model"].(string); ok {
			modelName = model
		}
	}

	if isWideScreen {
		// ========== 宽屏：左右分栏 ==========
		var leftCol, rightCol []string

		// 左侧：Request - 显示可进入的选项
		leftCol = append(leftCol, groupStyle.Render("📤 REQUEST"))
		leftCol = append(leftCol, fmt.Sprintf("  🤖 %s", valueStyle.Render(modelName)))
		leftCol = append(leftCol, "") // 分隔

		// 可进入的选项（使用特殊的聚焦标记）
		requestOptions := []struct {
			key      string
			icon     string
			label    string
			count    int
		}{
			{"system", "📦", "system", systemCount},
			{"tools", "🔧", "tools", toolsCount},
			{"messages", "💬", "messages", messagesCount},
		}

		// focusedSection 在主视图时：0-2 是 request 的选项，3 是 response
		requestFocusedIdx := m.detailState.focusedSection

		for i, opt := range requestOptions {
			prefix := "  "
			if opt.count > 0 {
				if i == requestFocusedIdx {
					// 聚焦的选项
					prefix = "▶ "
					leftCol = append(leftCol, successStyle.Render(fmt.Sprintf("%s%s %s [%d]", prefix, opt.icon, opt.label, opt.count)))
				} else {
					leftCol = append(leftCol, fmt.Sprintf("%s%s %s [%d]", prefix, opt.icon, opt.label, opt.count))
				}
			} else {
				leftCol = append(leftCol, mutedStyle.Render(fmt.Sprintf("  %s %s [无]", opt.icon, opt.label)))
			}
		}

		// 元信息
		var metaInfo []string
		if proxyReq != nil {
			if maxTokens, ok := proxyReq["max_tokens"].(float64); ok {
				metaInfo = append(metaInfo, fmt.Sprintf("max_tokens: %.0f", maxTokens))
			}
			if outputConfig, ok := proxyReq["output_config"].(map[string]interface{}); ok {
				if reasoningEffort, ok := outputConfig["reasoning_effort"].(string); ok {
					metaInfo = append(metaInfo, fmt.Sprintf("thinking: %s", reasoningEffort))
				}
			}
		}
		if len(metaInfo) > 0 {
			leftCol = append(leftCol, "")
			leftCol = append(leftCol, mutedStyle.Render("  ⚙️ 元信息"))
			for _, m := range metaInfo {
				leftCol = append(leftCol, fmt.Sprintf("    %s", contentStyle.Render(m)))
			}
		}

		// 右侧：Response
		rightCol = append(rightCol, groupStyle.Render("📥 RESPONSE"))
		rightCol = append(rightCol, "") // 分隔

		// choices 选项
		if requestFocusedIdx == 3 {
			rightCol = append(rightCol, successStyle.Render("  ▶ 💬 choices ["+fmt.Sprintf("%d", choicesCount)+"]"))
		} else {
			if choicesCount > 0 {
				rightCol = append(rightCol, fmt.Sprintf("  💬 choices [%d]", choicesCount))
			} else {
				rightCol = append(rightCol, mutedStyle.Render("  💬 choices [无]"))
			}
		}

		// Usage (不参与聚焦)
		if respData != nil {
			if usage, ok := respData["usage"].(map[string]interface{}); ok {
				var pt, ct, tt float64
				if p, ok := usage["prompt_tokens"].(float64); ok {
					pt = p
				}
				if c, ok := usage["completion_tokens"].(float64); ok {
					ct = c
				}
				if t, ok := usage["total_tokens"].(float64); ok {
					tt = t
				}
				if tt > 0 {
					rightCol = append(rightCol, fmt.Sprintf("  📊 tokens: %.0f (📝%.0f + ✍️%.0f)", tt, pt, ct))
				}
			}
		}
		rightCol = append(rightCol, fmt.Sprintf("  💬 choices: %d", choicesCount))

		// 费用信息
		if m.selectedEntry != nil && m.selectedEntry.TotalSpend > 0 {
			rightCol = append(rightCol, fmt.Sprintf("  💰 $%.4f", m.selectedEntry.TotalSpend))
		}

		// 渲染分栏
		leftWidth := m.width / 2
		rightWidth := m.width - leftWidth - 1

		leftContent := strings.Join(leftCol, "\n")
		rightContent := strings.Join(rightCol, "\n")

		// 根据聚焦状态选择卡片样式
		var leftCardStyle, rightCardStyle lipgloss.Style
		if m.detailState.focusedSection == 0 {
			leftCardStyle = focusedCardStyle
			rightCardStyle = cardStyle
		} else {
			leftCardStyle = cardStyle
			rightCardStyle = focusedCardStyle
		}

		leftCard := leftCardStyle.Width(leftWidth - 2).Render(leftContent)
		rightCard := rightCardStyle.Width(rightWidth - 2).Render(rightContent)

		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCard))
	} else {
		// ========== 窄屏：上下分栏 ==========
		// Request 部分 - 显示可进入的选项
		requestLines := []string{groupStyle.Render("📤 REQUEST")}
		requestLines = append(requestLines, fmt.Sprintf("  🤖 %s", valueStyle.Render(modelName)))
		requestLines = append(requestLines, "") // 分隔

		// 可进入的选项
		requestOptions := []struct {
			key      string
			icon     string
			label    string
			count    int
		}{
			{"system", "📦", "system", systemCount},
			{"tools", "🔧", "tools", toolsCount},
			{"messages", "💬", "messages", messagesCount},
		}

		// focusedSection: 0-2 是 request 选项，3 是 response
		requestFocusedIdx := m.detailState.focusedSection

		for i, opt := range requestOptions {
			if opt.count > 0 {
				if i == requestFocusedIdx {
					requestLines = append(requestLines, successStyle.Render(fmt.Sprintf("  ▶ %s %s [%d]", opt.icon, opt.label, opt.count)))
				} else {
					requestLines = append(requestLines, fmt.Sprintf("  %s %s [%d]", opt.icon, opt.label, opt.count))
				}
			} else {
				requestLines = append(requestLines, mutedStyle.Render(fmt.Sprintf("  %s %s [无]", opt.icon, opt.label)))
			}
		}

		// 元信息
		var metaInfo []string
		if proxyReq != nil {
			if maxTokens, ok := proxyReq["max_tokens"].(float64); ok {
				metaInfo = append(metaInfo, fmt.Sprintf("max_tokens: %.0f", maxTokens))
			}
			if outputConfig, ok := proxyReq["output_config"].(map[string]interface{}); ok {
				if reasoningEffort, ok := outputConfig["reasoning_effort"].(string); ok {
					metaInfo = append(metaInfo, fmt.Sprintf("thinking: %s", reasoningEffort))
				}
			}
		}
		if len(metaInfo) > 0 {
			requestLines = append(requestLines, "")
			requestLines = append(requestLines, mutedStyle.Render("  ⚙️ 元信息"))
			for _, m := range metaInfo {
				requestLines = append(requestLines, fmt.Sprintf("    %s", contentStyle.Render(m)))
			}
		}

		// Response 部分
		responseLines := []string{groupStyle.Render("📥 RESPONSE")}
		responseLines = append(responseLines, "") // 分隔

		// choices 选项
		if requestFocusedIdx == 3 {
			responseLines = append(responseLines, successStyle.Render(fmt.Sprintf("  ▶ 💬 choices [%d]", choicesCount)))
		} else {
			if choicesCount > 0 {
				responseLines = append(responseLines, fmt.Sprintf("  💬 choices [%d]", choicesCount))
			} else {
				responseLines = append(responseLines, mutedStyle.Render("  💬 choices [无]"))
			}
		}
		if respData != nil {
			if usage, ok := respData["usage"].(map[string]interface{}); ok {
				var pt, ct, tt float64
				if p, ok := usage["prompt_tokens"].(float64); ok {
					pt = p
				}
				if c, ok := usage["completion_tokens"].(float64); ok {
					ct = c
				}
				if t, ok := usage["total_tokens"].(float64); ok {
					tt = t
				}
				if tt > 0 {
					responseLines = append(responseLines, fmt.Sprintf("  📊 tokens: %.0f (📝%.0f + ✍️%.0f)", tt, pt, ct))
				}
			}
		}
		// 费用信息
		if m.selectedEntry != nil && m.selectedEntry.TotalSpend > 0 {
			responseLines = append(responseLines, fmt.Sprintf("  💰 $%.4f", m.selectedEntry.TotalSpend))
		}

		cardWidth := m.width - 4

		// 窄屏模式下，聚焦状态只影响整体卡片，不影响具体选项
		// 因为选项是在 Request 内部切换
		lines = append(lines, cardStyle.Width(cardWidth).Render(strings.Join(requestLines, "\n")))
		lines = append(lines, cardStyle.Width(cardWidth).Render(strings.Join(responseLines, "\n")))
	}

	return lines
}

// renderArrayDetailView 渲染数组详情视图
func (m *logsModel) renderArrayDetailView(proxyReq, respData map[string]interface{}, cardStyle, focusedCardStyle, contentStyle, mutedStyle, groupStyle, valueStyle, keyStyle lipgloss.Style) []string {
	var lines []string

	tab := m.detailState.activeTab
	_ = m.getTabItemCount(tab) // 确保有数据
	selectedIdx := m.detailState.selectedItem

	// 边界检查：确保 selectedIdx 不超出范围
	if selectedIdx < 0 {
		selectedIdx = 0
	}

	// 渲染数组项列表
	switch tab {
	case "system":
		// system 是独立的字段
		if proxyReq != nil {
			if system, ok := proxyReq["system"].([]interface{}); ok {
				itemCount := len(system)
				if selectedIdx >= itemCount {
					selectedIdx = 0
					m.detailState.selectedItem = 0
				}

				for i := 0; i < len(system); i++ {
					if i == selectedIdx {
						lines = append(lines, m.renderSystemItem(system[i], i, contentStyle, mutedStyle, groupStyle, valueStyle)...)
					} else {
						lines = append(lines, m.renderSystemSummary(system[i], i, contentStyle, mutedStyle)...)
					}
				}
			}
		}
	case "messages":
		// messages 是独立的字段
		if proxyReq != nil {
			if messages, ok := proxyReq["messages"].([]interface{}); ok {
				itemCount := len(messages)
				if selectedIdx >= itemCount {
					selectedIdx = 0
					m.detailState.selectedItem = 0
				}

				for i := 0; i < len(messages); i++ {
					if i == selectedIdx {
						lines = append(lines, m.renderMessageItem(messages[i], i, contentStyle, mutedStyle, groupStyle, valueStyle)...)
					} else {
						lines = append(lines, m.renderMessageSummary(messages[i], i, contentStyle, mutedStyle)...)
					}
				}
			}
		}
	case "tools":
		if proxyReq != nil {
			if tools, ok := proxyReq["tools"].([]interface{}); ok {
				itemCount := len(tools)
				if selectedIdx >= itemCount {
					selectedIdx = 0
					m.detailState.selectedItem = 0
				}

				for i := 0; i < len(tools) && i < 20; i++ {
					if i == selectedIdx {
						lines = append(lines, m.renderToolItem(tools[i], i, contentStyle, mutedStyle, groupStyle, valueStyle)...)
					} else {
						lines = append(lines, m.renderToolSummary(tools[i], i, contentStyle, mutedStyle)...)
					}
				}
				if len(tools) > 20 {
					lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ... 还有 %d 个", len(tools)-20)))
				}
			}
		}
	case "choices":
		if respData != nil {
			if choices, ok := respData["choices"].([]interface{}); ok {
				itemCount := len(choices)
				if selectedIdx >= itemCount {
					selectedIdx = 0
					m.detailState.selectedItem = 0
				}

				for i := 0; i < len(choices) && i < 10; i++ {
					if i == selectedIdx {
						lines = append(lines, m.renderChoiceItem(choices[i], i, contentStyle, mutedStyle, groupStyle, valueStyle)...)
					} else {
						lines = append(lines, m.renderChoiceSummary(choices[i], i, contentStyle, mutedStyle)...)
					}
				}
				if len(choices) > 10 {
					lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ... 还有 %d 个", len(choices)-10)))
				}
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, mutedStyle.Render("无数据"))
	}

	return lines
}

// renderMessageSummary 渲染消息摘要
func (m *logsModel) renderMessageSummary(msg interface{}, idx int, contentStyle, mutedStyle lipgloss.Style) []string {
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		// 安全处理非 map 类型
		if jsonBytes, err := json.Marshal(msg); err == nil {
			return []string{mutedStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 50)))}
		}
		return []string{mutedStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, msg))}
	}

	role, _ := msgMap["role"].(string)
	content, _ := msgMap["content"].(string)
	toolCalls, _ := msgMap["tool_calls"].([]interface{})

	roleIcon := map[string]string{
		"system":   "📦",
		"user":     "👤",
		"assistant": "🤖",
		"tool":     "🔧",
	}[role]
	if roleIcon == "" {
		roleIcon = "💬"
	}

	summary := roleIcon + " " + role
	if content != "" {
		summary += ": " + truncate(content, 50)
	}
	if len(toolCalls) > 0 {
		summary += fmt.Sprintf(" [+%d tool_calls]", len(toolCalls))
	}

	return []string{mutedStyle.Render(fmt.Sprintf("  [%d] %s", idx, summary))}
}

// renderMessageItem 渲染消息详情
func (m *logsModel) renderMessageItem(msg interface{}, idx int, contentStyle, mutedStyle, groupStyle, valueStyle lipgloss.Style) []string {
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		// 安全处理非 map 类型
		if jsonBytes, err := json.Marshal(msg); err == nil {
			return []string{contentStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 200)))}
		}
		return []string{contentStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, msg))}
	}

	role, _ := msgMap["role"].(string)
	// 安全处理 content，可能是 string 或其他类型
	content, contentIsString := msgMap["content"].(string)
	if !contentIsString {
		// content 不是 string，可能是其他类型（如 []interface{} 用于 tool 结果）
		rawContent := msgMap["content"]
		if rawContent != nil {
			if jsonBytes, err := json.Marshal(rawContent); err == nil {
				content = fmt.Sprintf("(type: %T) %s", rawContent, truncate(string(jsonBytes), 100))
			}
		}
	}
	toolCalls, _ := msgMap["tool_calls"].([]interface{})

	var lines []string
	lines = append(lines, groupStyle.Render(fmt.Sprintf("  [%d] %s", idx, role)))

	if content != "" {
		lines = append(lines, contentStyle.Render(truncate(content, 500)))
	}

	if len(toolCalls) > 0 {
		lines = append(lines, mutedStyle.Render("  tool_calls:"))
		for _, tc := range toolCalls {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				var fnName string
				var args string
				if fn, ok := tcMap["function"].(map[string]interface{}); ok {
					if n, ok := fn["name"].(string); ok {
						fnName = n
					}
					// arguments 可能是 string 或其他类型
					if a, ok := fn["arguments"].(string); ok {
						args = truncate(a, 200)
					} else if rawArgs := fn["arguments"]; rawArgs != nil {
						if jsonBytes, err := json.Marshal(rawArgs); err == nil {
							args = truncate(string(jsonBytes), 200)
						}
					}
				}
				if args != "" {
					lines = append(lines, contentStyle.Render(fmt.Sprintf("    - %s(%s)", fnName, args)))
				} else {
					lines = append(lines, contentStyle.Render(fmt.Sprintf("    - %s()", fnName)))
				}
			}
		}
	}

	return lines
}

// renderToolSummary 渲染工具摘要
func (m *logsModel) renderToolSummary(tool interface{}, idx int, contentStyle, mutedStyle lipgloss.Style) []string {
	toolMap, ok := tool.(map[string]interface{})
	if !ok {
		// 安全处理非 map 类型
		if jsonBytes, err := json.Marshal(tool); err == nil {
			return []string{mutedStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 50)))}
		}
		return []string{mutedStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, tool))}
	}

	var name, desc string
	if fn, ok := toolMap["function"].(map[string]interface{}); ok {
		if n, ok := fn["name"].(string); ok {
			name = n
		}
		if d, ok := fn["description"].(string); ok {
			desc = truncate(d, 40)
		}
	}

	summary := fmt.Sprintf("🔧 %s", name)
	if desc != "" {
		summary += ": " + desc
	}

	return []string{mutedStyle.Render(fmt.Sprintf("  [%d] %s", idx, summary))}
}

// renderSystemSummary 渲染 system 消息摘要
func (m *logsModel) renderSystemSummary(sys interface{}, idx int, contentStyle, mutedStyle lipgloss.Style) []string {
	sysMap, ok := sys.(map[string]interface{})
	if !ok {
		if jsonBytes, err := json.Marshal(sys); err == nil {
			return []string{mutedStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 50)))}
		}
		return []string{mutedStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, sys))}
	}

	sysType, _ := sysMap["type"].(string)
	text, _ := sysMap["text"].(string)

	summary := fmt.Sprintf("📦 system[%d] (%s)", idx, sysType)
	if text != "" {
		summary += ": " + truncate(text, 40)
	}

	return []string{mutedStyle.Render("  " + summary)}
}

// renderSystemItem 渲染 system 消息详情
func (m *logsModel) renderSystemItem(sys interface{}, idx int, contentStyle, mutedStyle, groupStyle, valueStyle lipgloss.Style) []string {
	sysMap, ok := sys.(map[string]interface{})
	if !ok {
		if jsonBytes, err := json.Marshal(sys); err == nil {
			return []string{contentStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 200)))}
		}
		return []string{contentStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, sys))}
	}

	var lines []string
	sysType, _ := sysMap["type"].(string)
	lines = append(lines, groupStyle.Render(fmt.Sprintf("  [%d] system (%s)", idx, sysType)))

	if text, ok := sysMap["text"].(string); ok && text != "" {
		lines = append(lines, contentStyle.Render(truncate(text, 500)))
	}

	// 显示 cache_control 如果有
	if cacheControl, ok := sysMap["cache_control"].(map[string]interface{}); ok {
		if ctype, ok := cacheControl["type"].(string); ok {
			lines = append(lines, mutedStyle.Render("  cache_control: "+ctype))
		}
	}

	return lines
}

// renderToolItem 渲染工具详情
func (m *logsModel) renderToolItem(tool interface{}, idx int, contentStyle, mutedStyle, groupStyle, valueStyle lipgloss.Style) []string {
	toolMap, ok := tool.(map[string]interface{})
	if !ok {
		// 安全处理非 map 类型
		if jsonBytes, err := json.Marshal(tool); err == nil {
			return []string{contentStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 200)))}
		}
		return []string{contentStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, tool))}
	}

	var lines []string
	lines = append(lines, groupStyle.Render(fmt.Sprintf("  [%d] tool", idx)))

	if fn, ok := toolMap["function"].(map[string]interface{}); ok {
		if name, ok := fn["name"].(string); ok {
			lines = append(lines, valueStyle.Render("  name: "+name))
		}
		if desc, ok := fn["description"].(string); ok {
			lines = append(lines, contentStyle.Render("  description: "+truncate(desc, 300)))
		}
		if params, ok := fn["parameters"].(map[string]interface{}); ok {
			// 序列化 parameters
			if jsonBytes, err := json.MarshalIndent(params, "    ", "  "); err == nil {
				lines = append(lines, mutedStyle.Render("  parameters:"))
				lines = append(lines, contentStyle.Render("    "+truncate(string(jsonBytes), 300)))
			}
		}
	}

	return lines
}

// renderChoiceSummary 渲染选择摘要
func (m *logsModel) renderChoiceSummary(choice interface{}, idx int, contentStyle, mutedStyle lipgloss.Style) []string {
	c, ok := choice.(map[string]interface{})
	if !ok {
		return []string{mutedStyle.Render(fmt.Sprintf("  [%d] 无效数据", idx))}
	}

	var finishReason string
	if fr, ok := c["finish_reason"].(string); ok {
		finishReason = fr
	}

	summary := fmt.Sprintf("💬 choice[%d]", idx)
	if finishReason != "" {
		summary += fmt.Sprintf(" (%s)", finishReason)
	}

	return []string{mutedStyle.Render("  " + summary)}
}

// renderChoiceItem 渲染选择详情
func (m *logsModel) renderChoiceItem(choice interface{}, idx int, contentStyle, mutedStyle, groupStyle, valueStyle lipgloss.Style) []string {
	// 安全的类型检查
	c, ok := choice.(map[string]interface{})
	if !ok {
		// 尝试 JSON 序列化看看原始内容
		if jsonBytes, err := json.Marshal(choice); err == nil {
			return []string{contentStyle.Render(fmt.Sprintf("  [%d] %s", idx, truncate(string(jsonBytes), 200)))}
		}
		return []string{contentStyle.Render(fmt.Sprintf("  [%d] 无效数据类型: %T", idx, choice))}
	}

	var lines []string
	lines = append(lines, groupStyle.Render(fmt.Sprintf("  [%d] choice", idx)))

	if fr, ok := c["finish_reason"].(string); ok {
		lines = append(lines, valueStyle.Render("  finish_reason: "+fr))
	}

	if msg, ok := c["message"].(map[string]interface{}); ok {
		if role, ok := msg["role"].(string); ok {
			lines = append(lines, valueStyle.Render("  role: "+role))
		}
		// 安全处理 content，可能是 string 或其他类型
		rawContent := msg["content"]
		if content, ok := rawContent.(string); ok && content != "" {
			lines = append(lines, contentStyle.Render("  content: "+truncate(content, 300)))
		} else if rawContent != nil {
			// content 不是 string，可能是其他类型
			if jsonBytes, err := json.Marshal(rawContent); err == nil {
				lines = append(lines, contentStyle.Render("  content: "+truncate(string(jsonBytes), 200)))
			}
		}
		if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
			lines = append(lines, mutedStyle.Render("  tool_calls:"))
			for _, tc := range toolCalls {
				if tcMap, ok := tc.(map[string]interface{}); ok {
					var fnName string
					var args string
					if fn, ok := tcMap["function"].(map[string]interface{}); ok {
						if n, ok := fn["name"].(string); ok {
							fnName = n
						}
						// arguments 可能是 string 或其他类型（如 map）
						if a, ok := fn["arguments"].(string); ok {
							args = truncate(a, 200)
						} else if rawArgs := fn["arguments"]; rawArgs != nil {
							// 尝试 JSON 序列化
							if jsonBytes, err := json.Marshal(rawArgs); err == nil {
								args = truncate(string(jsonBytes), 200)
							}
						}
					}
					if args != "" {
						lines = append(lines, contentStyle.Render(fmt.Sprintf("    - %s(%s)", fnName, args)))
					} else {
						lines = append(lines, contentStyle.Render(fmt.Sprintf("    - %s()", fnName)))
					}
				}
			}
		}
	}

	return lines
}

// toolInfo 表示一个工具的信息
type toolInfo struct {
	name    string
	called  bool
	schema  string
}

// parseToolsInfo 解析工具信息
func (m *logsModel) parseToolsInfo() (result struct {
	total int
	called int
	tools []toolInfo
}) {
	result.tools = []toolInfo{}

	// 优先从 proxy_server_request 解析 tools
	proxyReq, _ := m.detailData["proxy_server_request"].(map[string]interface{})
	if proxyReq != nil {
		if tools, ok := proxyReq["tools"].([]interface{}); ok {
			for _, tool := range tools {
				if toolMap, ok := tool.(map[string]interface{}); ok {
					var name string
					var schema string
					if fn, ok := toolMap["function"].(map[string]interface{}); ok {
						if n, ok := fn["name"].(string); ok {
							name = n
						}
						if desc, ok := fn["description"].(string); ok {
							schema = truncate(desc, 100)
						}
					}
					if name != "" {
						result.total++
						result.tools = append(result.tools, toolInfo{name: name, called: false, schema: schema})
					}
				}
			}
		}
	}

	// 从 messages 中解析 tool_calls（已调用的工具）
	if messages, ok := m.detailData["messages"].([]interface{}); ok {
		calledNames := make(map[string]bool)
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				// 检查 tool_calls
				if toolCalls, ok := msgMap["tool_calls"].([]interface{}); ok {
					for _, tc := range toolCalls {
						if tcMap, ok := tc.(map[string]interface{}); ok {
							var name string
							if n, ok := tcMap["function"].(map[string]interface{}); ok {
								if fn, ok := n["name"].(string); ok {
									name = fn
								}
							}
							if name != "" {
								calledNames[name] = true
								// 检查是否已存在
								found := false
								for i, t := range result.tools {
									if t.name == name {
										result.tools[i].called = true
										found = true
										break
									}
								}
								if !found {
									result.total++
									result.called++
									result.tools = append(result.tools, toolInfo{name: name, called: true})
								}
							}
						}
					}
				}
			}
		}
		// 更新 called 计数
		for i := range result.tools {
			if calledNames[result.tools[i].name] {
				result.called++
			}
		}
	}

	return result
}

// renderInputContent 渲染 Input 内容
func (m *logsModel) renderInputContent() string {
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	roleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Bold(true)

	var lines []string

	// 解析 messages
	if messages, ok := m.detailData["messages"].([]interface{}); ok {
		for i, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)

				roleStr := roleStyle.Render(role + ":")
				contentStr := truncate(content, 200)

				// system 和 history 默认折叠，最后一条 user message 默认展开
				if i < len(messages)-1 || role == "system" || role == "assistant" {
					contentStr = mutedStyle.Render("[点击展开] " + truncate(content, 50))
				}

				lines = append(lines, roleStr+" "+contentStyle.Render(contentStr))
			}
		}
	} else if proxyReq, ok := m.detailData["proxy_server_request"].(map[string]interface{}); ok {
		// 回退到 proxy_server_request
		if messages, ok := proxyReq["messages"].([]interface{}); ok {
			for i, msg := range messages {
				if msgMap, ok := msg.(map[string]interface{}); ok {
					role, _ := msgMap["role"].(string)
					content, _ := msgMap["content"].(string)

					roleStr := roleStyle.Render(role + ":")
					contentStr := truncate(content, 200)

					// 最后一条 user message 默认展开
					if i < len(messages)-1 || role == "system" || role == "assistant" {
						contentStr = mutedStyle.Render("[点击展开] " + truncate(content, 50))
					}

					lines = append(lines, roleStr+" "+contentStyle.Render(contentStr))
				}
			}
		}
	}

	if len(lines) == 0 {
		return mutedStyle.Render("无 Input 数据")
	}

	return strings.Join(lines, "\n")
}

// renderOutputContent 渲染 Output 内容
func (m *logsModel) renderOutputContent() string {
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	roleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Bold(true)
	toolCallStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)

	var lines []string

	// 检查是否有 tool_calls 响应
	if response, ok := m.detailData["response"].(map[string]interface{}); ok {
		if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
			for _, choice := range choices {
				if c, ok := choice.(map[string]interface{}); ok {
					if msg, ok := c["message"].(map[string]interface{}); ok {
						// 先检查 tool_calls
						if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
							lines = append(lines, toolCallStyle.Render("🔧 Tool Calls:"))
							for _, tc := range toolCalls {
								if tcMap, ok := tc.(map[string]interface{}); ok {
									var fnName string
									var args string
									if fn, ok := tcMap["function"].(map[string]interface{}); ok {
										if n, ok := fn["name"].(string); ok {
											fnName = n
										}
										// arguments 可能是 string 或其他类型
										if a, ok := fn["arguments"].(string); ok {
											args = truncate(a, 100)
										} else if rawArgs := fn["arguments"]; rawArgs != nil {
											if jsonBytes, err := json.Marshal(rawArgs); err == nil {
												args = truncate(string(jsonBytes), 100)
											}
										}
									}
									if args != "" {
										lines = append(lines, "  "+toolCallStyle.Render("• ")+contentStyle.Render(fnName+"("+args+")"))
									} else {
										lines = append(lines, "  "+toolCallStyle.Render("• ")+contentStyle.Render(fnName+"()"))
									}
								}
							}
							lines = append(lines, "")
						}

						// 再检查 content (普通回复)
						if content, ok := msg["content"].(string); ok && content != "" {
							lines = append(lines, roleStyle.Render("assistant:")+" "+contentStyle.Render(truncate(content, 300)))
						}
					}
				}
			}
		}
	}

	// 回退：如果没有 response，检查 messages 中的 assistant 消息
	if len(lines) == 0 {
		if messages, ok := m.detailData["messages"].([]interface{}); ok {
			for _, msg := range messages {
				if msgMap, ok := msg.(map[string]interface{}); ok {
					if role, ok := msgMap["role"].(string); ok && role == "assistant" {
						// 检查 tool_calls
						if toolCalls, ok := msgMap["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
							lines = append(lines, toolCallStyle.Render("🔧 Tool Calls:"))
							for _, tc := range toolCalls {
								if tcMap, ok := tc.(map[string]interface{}); ok {
									var fnName string
									var args string
									if fn, ok := tcMap["function"].(map[string]interface{}); ok {
										if n, ok := fn["name"].(string); ok {
											fnName = n
										}
										// arguments 可能是 string 或其他类型
										if a, ok := fn["arguments"].(string); ok {
											args = truncate(a, 100)
										} else if rawArgs := fn["arguments"]; rawArgs != nil {
											if jsonBytes, err := json.Marshal(rawArgs); err == nil {
												args = truncate(string(jsonBytes), 100)
											}
										}
									}
									if args != "" {
										lines = append(lines, "  "+toolCallStyle.Render("• ")+contentStyle.Render(fnName+"("+args+")"))
									} else {
										lines = append(lines, "  "+toolCallStyle.Render("• ")+contentStyle.Render(fnName+"()"))
									}
								}
							}
							lines = append(lines, "")
						}
						// 检查 content
						if content, ok := msgMap["content"].(string); ok && content != "" {
							lines = append(lines, roleStyle.Render("assistant:")+" "+contentStyle.Render(truncate(content, 300)))
						}
					}
				}
			}
		}
	}

	if len(lines) == 0 {
		return mutedStyle.Render("无 Output 数据")
	}

	return strings.Join(lines, "\n")
}

// renderMetadataContent 渲染 Metadata 内容
func (m *logsModel) renderMetadataContent() string {
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// 序列化 metadata 为 JSON
	if metadata, ok := m.detailData["metadata"].(map[string]interface{}); ok && len(metadata) > 0 {
		jsonBytes, err := json.MarshalIndent(metadata, "  ", "  ")
		if err != nil {
			return mutedStyle.Render("metadata 解析失败")
		}
		return contentStyle.Render(string(jsonBytes))
	}

	// 回退：显示其他未处理的字段
	var otherFields []string
	skipKeys := map[string]bool{
		"request_tags": true, "model": true, "model_id": true, "call_type": true,
		"status": true, "total_tokens": true, "prompt_tokens": true, "completion_tokens": true,
		"spend": true, "latency": true, "cache_hit": true, "litellm_overhead_time": true,
		"retries": true, "startTime": true, "endTime": true, "prompt_cost": true,
		"completion_cost": true, "messages": true, "response": true, "proxy_server_request": true,
		"available_tools": true, "tools": true, "metadata": true,
	}

	for k, v := range m.detailData {
		if skipKeys[k] {
			continue
		}
		valStr := fmt.Sprintf("%v", v)
		if len(valStr) > 80 {
			valStr = valStr[:80] + "..."
		}
		otherFields = append(otherFields, k+": "+valStr)
	}

	if len(otherFields) > 0 {
		return contentStyle.Render(strings.Join(otherFields, "\n"))
	}

	return mutedStyle.Render("无 Metadata 数据")
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getMapKeys 获取 map 的 keys（用于调试日志）
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (m *logsModel) renderListView() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("236"))

	// 直接渲染完整的日志表格
	var content strings.Builder

	// 计算可用的行数 (预留表头3行 + 底部状态栏2行)
	availableRows := 50
	if m.height > 10 {
		availableRows = m.height - 10
	}

	if m.logData != nil && len(m.logData.Data) > 0 {
		// 过滤数据
		filteredData := m.logData.Data
		if m.model != "" {
			var filtered []api.SpendLogEntry
			for _, entry := range m.logData.Data {
				if strings.Contains(entry.Model, m.model) {
					filtered = append(filtered, entry)
				}
			}
			filteredData = filtered
		}
		content.WriteString(renderLogsTable(filteredData, int(m.logData.Total), m.newLogIDs, availableRows, m.selectedIndex))
	} else if m.logDataOld != nil && len(*m.logDataOld) > 0 {
		content.WriteString(renderLogsTableOld(m.logDataOld, m.interval, m.newLogIDs, availableRows, m.selectedIndex))
	} else {
		content.WriteString("暂无数据")
	}

	return headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 日志 (刷新: %ds) | ↑↓ 选择 | Enter 查看详情 | q 退出 ", m.interval)) +
		"\n\n" +
		content.String() +
		fmt.Sprintf("\n\n⏱ 更新次数: %d | 时间: %s", m.tick, time.Now().Format("15:04:05"))
}

func (m *logsModel) refresh() {
	// 使用 datetime 格式，并 URL 编码空格
	endDate := url.QueryEscape(time.Now().Format("2006-01-02 15:04:05"))
	startDate := url.QueryEscape(time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05"))

	// 优先使用 /spend/logs/ui
	resp, err := m.client.GetSpendLogsUI(startDate, endDate)
	if err != nil {
		// 回退到旧的 /spend/logs
		respOld, err2 := m.client.GetSpendLogs(
			time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
			time.Now().Format("2006-01-02"),
		)
		if err2 != nil {
			m.data = fmt.Sprintf("❌ 获取失败: %v", err)
			m.logData = nil
			m.logDataOld = nil
			return
		}
		if respOld == nil || len(*respOld) == 0 {
			m.data = "暂无数据"
			m.logData = nil
			m.logDataOld = nil
			return
		}
		m.data = fmt.Sprintf("✅ 获取到 %d 条日志记录", len(*respOld))

		// 首次加载只记录日志ID，不高亮
		if !m.initialized {
			m.initialized = true
			for _, entry := range *respOld {
				if id, ok := entry["request_id"]; ok {
					if logID, ok := id.(string); ok {
						m.seenLogIDs[logID] = true
					}
				}
			}
			m.logData = nil
			m.logDataOld = respOld
			return
		}

		// 识别新增日志
		m.newLogIDs = make(map[string]bool)
		for _, entry := range *respOld {
			var logID string
			if id, ok := entry["request_id"]; ok {
				logID, _ = id.(string)
			}
			if logID != "" && !m.seenLogIDs[logID] {
				m.newLogIDs[logID] = true
			}
			if logID != "" {
				m.seenLogIDs[logID] = true
			}
		}

		m.logData = nil
		m.logDataOld = respOld
		return
	}

	if resp == nil || len(resp.Data) == 0 {
		m.data = "暂无数据"
		m.logData = nil
		m.logDataOld = nil
		return
	}

	m.data = fmt.Sprintf("✅ 获取到 %d 条日志记录 (总 %d)", len(resp.Data), resp.Total)

	// 首次加载只记录日志ID，不高亮
	if !m.initialized {
		m.initialized = true
		for _, entry := range resp.Data {
			m.seenLogIDs[entry.ID] = true
		}
		m.logData = resp
		m.logDataOld = nil
		return
	}

	// 识别新增日志
	m.newLogIDs = make(map[string]bool)
	for _, entry := range resp.Data {
		if !m.seenLogIDs[entry.ID] {
			m.newLogIDs[entry.ID] = true
		}
		m.seenLogIDs[entry.ID] = true
	}

	m.logData = resp
	m.logDataOld = nil
}

func (m *logsModel) loadDetail() tea.Cmd {
	var requestID string

	if m.logData != nil && m.selectedIndex < len(m.logData.Data) {
		requestID = m.logData.Data[m.selectedIndex].ID
		m.selectedEntry = &m.logData.Data[m.selectedIndex]
	} else if m.logDataOld != nil && m.selectedIndex < len(*m.logDataOld) {
		if id, ok := (*m.logDataOld)[m.selectedIndex]["request_id"]; ok {
			requestID, _ = id.(string)
		}
		m.selectedEntry = nil
	} else {
		// 无法获取数据
		m.detailError = "暂无数据"
		m.viewMode = "detail"
		m.selectedEntry = nil
		return nil
	}

	if requestID == "" {
		m.detailError = "无法获取日志ID"
		m.viewMode = "detail"
		m.selectedEntry = nil
		return nil
	}

	m.viewMode = "detail"
	m.detailData = nil
	m.detailError = "加载中..."

	// 异步加载详情
	return func() tea.Msg {
		log.Printf("[loadDetail] 开始加载详情, requestID=%s", requestID)
		detail, err := m.client.GetSpendLogDetail(requestID)
		if err != nil {
			log.Printf("[loadDetail] 请求失败: %v", err)
			return detailLoadedMsg{error: fmt.Sprintf("请求失败: %v", err)}
		}
		log.Printf("[loadDetail] 请求完成, requestID=%s, keys=%v", requestID, getMapKeys(detail))
		if detail == nil {
			return detailLoadedMsg{error: "API 返回空数据，请确认日志详情接口是否可用"}
		}
		return detailLoadedMsg{data: detail}
	}
}

type tickMsg time.Time
type detailLoadedMsg struct {
	data  map[string]interface{}
	error string
}

// getTabItemCount 返回指定 tab 的数组项数量
func (m *logsModel) getTabItemCount(tab string) int {
	proxyReq, _ := m.detailData["proxy_server_request"].(map[string]interface{})
	respData, _ := m.detailData["response"].(map[string]interface{})

	switch tab {
	case "system":
		// system 是独立的字段
		if proxyReq != nil {
			if system, ok := proxyReq["system"].([]interface{}); ok {
				return len(system)
			}
		}
	case "tools":
		if proxyReq != nil {
			if tools, ok := proxyReq["tools"].([]interface{}); ok {
				return len(tools)
			}
		}
	case "messages":
		if proxyReq != nil {
			if messages, ok := proxyReq["messages"].([]interface{}); ok {
				return len(messages)
			}
		}
	case "choices":
		if respData != nil {
			if choices, ok := respData["choices"].([]interface{}); ok {
				return len(choices)
			}
		}
	}
	return 0
}

// getArrayItem 获取指定 tab 的第 index 项
func (m *logsModel) getArrayItem(tab string, index int) interface{} {
	proxyReq, _ := m.detailData["proxy_server_request"].(map[string]interface{})
	respData, _ := m.detailData["response"].(map[string]interface{})

	switch tab {
	case "system":
		if proxyReq != nil {
			if messages, ok := proxyReq["messages"].([]interface{}); ok {
				count := 0
				for _, msg := range messages {
					if msgMap, ok := msg.(map[string]interface{}); ok {
						if role, _ := msgMap["role"].(string); role == "system" {
							if count == index {
								return msgMap
							}
							count++
						}
					}
				}
			}
		}
	case "tools":
		if proxyReq != nil {
			if tools, ok := proxyReq["tools"].([]interface{}); ok {
				if index >= 0 && index < len(tools) {
					return tools[index]
				}
			}
		}
	case "messages":
		if proxyReq != nil {
			if messages, ok := proxyReq["messages"].([]interface{}); ok {
				if index >= 0 && index < len(messages) {
					return messages[index]
				}
			}
		}
	case "choices":
		if respData != nil {
			if choices, ok := respData["choices"].([]interface{}); ok {
				if index >= 0 && index < len(choices) {
					return choices[index]
				}
			}
		}
	}
	return nil
}