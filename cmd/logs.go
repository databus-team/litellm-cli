package cmd

import (
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
	viewMode      string          // "list" 或 "detail"
	detailData    map[string]interface{}
	detailError   string
	detailScroll  int // 详情视图滚动偏移量
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
			// 返回列表视图
			if m.viewMode == "detail" {
				m.viewMode = "list"
				m.detailData = nil
				m.detailError = ""
				m.detailScroll = 0 // 重置滚动位置
			}
			return m, nil
		case "enter":
			// 查看详情
			if m.viewMode == "list" {
				m.detailScroll = 0 // 重置滚动位置
				cmd := m.loadDetail()
				return m, cmd
			}
			return m, nil
		case "up", "k":
			// 上移选择或向上滚动
			if m.viewMode == "list" {
				if m.selectedIndex > 0 {
					m.selectedIndex--
				}
			} else if m.viewMode == "detail" {
				if m.detailScroll > 0 {
					m.detailScroll--
				}
			}
			return m, nil
		case "down", "j":
			// 下移选择或向下滚动
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
			} else if m.viewMode == "detail" {
				// 详情视图向下滚动
				m.detailScroll++
			}
			return m, nil
		case "pgup":
			// 详情视图向上翻页
			if m.viewMode == "detail" {
				m.detailScroll = max(0, m.detailScroll-20)
			}
			return m, nil
		case "pgdown":
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

	var lines []string

	lines = append(lines, headerStyle.Render(" 📋 日志详情 | 按 ESC 返回 | Enter 刷新 "))
	lines = append(lines, "")

	if m.detailError != "" {
		lines = append(lines, contentStyle.Render(m.detailError))
		if m.detailError == "加载中..." {
			lines = append(lines, mutedStyle.Render(" ⏳"))
		}
	} else if m.detailData == nil {
		lines = append(lines, mutedStyle.Render("无详情数据，请按 Enter 刷新"))
	}

	if m.detailData != nil {
		// 1. 处理 response 字段 (OpenAI 格式响应)
		if response, ok := m.detailData["response"].(map[string]interface{}); ok && response != nil {
			lines = append(lines, groupStyle.Render("📤 响应 (Response)"))
			if id, ok := response["id"].(string); ok {
				lines = append(lines, keyStyle.Render("ID:")+contentStyle.Render(id))
			}
			if model, ok := response["model"].(string); ok {
				lines = append(lines, keyStyle.Render("模型:")+contentStyle.Render(model))
			}
			if created, ok := response["created"].(float64); ok {
				lines = append(lines, keyStyle.Render("创建时间:")+contentStyle.Render(fmt.Sprintf("%.0f", created)))
			}
			if object, ok := response["object"].(string); ok {
				lines = append(lines, keyStyle.Render("类型:")+contentStyle.Render(object))
			}
			// Usage
			if usage, ok := response["usage"].(map[string]interface{}); ok && usage != nil {
				lines = append(lines, keyStyle.Render("用量:"))
				var usageParts []string
				if pt, ok := usage["prompt_tokens"].(float64); ok {
					usageParts = append(usageParts, fmt.Sprintf("prompt=%.0f", pt))
				}
				if ct, ok := usage["completion_tokens"].(float64); ok {
					usageParts = append(usageParts, fmt.Sprintf("completion=%.0f", ct))
				}
				if tt, ok := usage["total_tokens"].(float64); ok {
					usageParts = append(usageParts, fmt.Sprintf("total=%.0f", tt))
				}
				lines = append(lines, contentStyle.Render(strings.Join(usageParts, ", ")))
			}
			// Choices
			if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				lines = append(lines, keyStyle.Render("回复:")+fmt.Sprintf(" (%d 个选择)", len(choices)))
				for _, choice := range choices {
					if c, ok := choice.(map[string]interface{}); ok {
						var line string
						if idx, ok := c["index"].(float64); ok {
							line += mutedStyle.Render(fmt.Sprintf("  [%.0f]", idx))
						}
						if msg, ok := c["message"].(map[string]interface{}); ok {
							if role, ok := msg["role"].(string); ok {
								line += keyStyle.Render(fmt.Sprintf(" %s: ", role))
							}
							if content, ok := msg["content"].(string); ok {
								// 限制显示长度
								maxLen := 200
								if len(content) > maxLen {
									content = content[:maxLen] + "..."
								}
								content = strings.ReplaceAll(content, "\n", " ")
								line += valueStyle.Render(content)
							}
						}
						if finishReason, ok := c["finish_reason"].(string); ok {
							line += mutedStyle.Render(fmt.Sprintf(" (finish: %s)", finishReason))
						}
						lines = append(lines, line)
					}
				}
			}
			lines = append(lines, "")
		}

		// 2. 处理 messages 字段
		if messages, ok := m.detailData["messages"]; ok && messages != nil {
			lines = append(lines, groupStyle.Render("💬 消息 (Messages)"))
			if msgMap, ok := messages.(map[string]interface{}); ok {
				for k, v := range msgMap {
					lines = append(lines, keyStyle.Render(k+":")+contentStyle.Render(fmt.Sprintf("%v", v)))
				}
			} else {
				lines = append(lines, contentStyle.Render(fmt.Sprintf("%v", messages)))
			}
			lines = append(lines, "")
		}

		// 3. 处理 proxy_server_request 字段
		if proxyReq, ok := m.detailData["proxy_server_request"].(map[string]interface{}); ok && proxyReq != nil {
			lines = append(lines, groupStyle.Render("📥 请求 (Proxy Server Request)"))
			if model, ok := proxyReq["model"].(string); ok {
				lines = append(lines, keyStyle.Render("模型:")+contentStyle.Render(model))
			}
			if messages, ok := proxyReq["messages"].([]interface{}); ok && len(messages) > 0 {
				lines = append(lines, keyStyle.Render("消息:")+fmt.Sprintf(" (%d 条)", len(messages)))
				for i, msg := range messages {
					if mi, ok := msg.(map[string]interface{}); ok {
						var line string
						if role, ok := mi["role"].(string); ok {
							line += mutedStyle.Render(fmt.Sprintf("  [%d] %s: ", i+1, role))
						}
						if content, ok := mi["content"].(string); ok {
							maxLen := 150
							if len(content) > maxLen {
								content = content[:maxLen] + "..."
							}
							content = strings.ReplaceAll(content, "\n", " ")
							line += valueStyle.Render(content)
						}
						lines = append(lines, line)
					}
				}
			}
			if temperature, ok := proxyReq["temperature"].(float64); ok {
				lines = append(lines, keyStyle.Render("temperature:")+contentStyle.Render(fmt.Sprintf("%.1f", temperature)))
			}
			if maxTokens, ok := proxyReq["max_tokens"].(float64); ok {
				lines = append(lines, keyStyle.Render("max_tokens:")+contentStyle.Render(fmt.Sprintf("%.0f", maxTokens)))
			}
			lines = append(lines, "")
		}

		// 4. 其他字段
		var otherFields []string
		for apiKey := range m.detailData {
			if apiKey != "response" && apiKey != "messages" && apiKey != "proxy_server_request" {
				otherFields = append(otherFields, apiKey)
			}
		}
		if len(otherFields) > 0 {
			lines = append(lines, groupStyle.Render("📊 其他信息"))
			sort.Strings(otherFields)
			for _, key := range otherFields {
				v := m.detailData[key]
				if v == nil {
					continue
				}
				valStr := fmt.Sprintf("%v", v)
				// 简化过长字段
				if len(valStr) > 100 {
					valStr = valStr[:100] + "..."
				}
				lines = append(lines, keyStyle.Render(key+":")+contentStyle.Render(valStr))
			}
		}
	}

	// 底部提示
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("提示: ↑↓ 滚动 | Enter 刷新 | ESC 返回"))

	// 计算滚动 - 预留 2 行给头部，1 行给底部提示
	scrollOffset := m.detailScroll
	maxDisplayLines := m.height - 3
	if maxDisplayLines < 10 {
		maxDisplayLines = 20
	}

	// 确保滚动不越界
	totalLines := len(lines)
	if scrollOffset > totalLines-maxDisplayLines {
		scrollOffset = max(0, totalLines-maxDisplayLines)
		m.detailScroll = scrollOffset
	}

	// 截取需要显示的行
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
	} else if m.logDataOld != nil && m.selectedIndex < len(*m.logDataOld) {
		if id, ok := (*m.logDataOld)[m.selectedIndex]["request_id"]; ok {
			requestID, _ = id.(string)
		}
	} else {
		// 无法获取数据
		m.detailError = "暂无数据"
		m.viewMode = "detail"
		return nil
	}

	if requestID == "" {
		m.detailError = "无法获取日志ID"
		m.viewMode = "detail"
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
		log.Printf("[loadDetail] 请求完成, detail=%v", detail)
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