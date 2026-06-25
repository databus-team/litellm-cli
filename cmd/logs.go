package cmd

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"litellm-cli/internal/api"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var (
	interval int
	model    string
	textMode bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "轮询查看日志 (TUI)",
	Run:   runLogs,
}

func init() {
	logsCmd.Flags().IntVarP(&interval, "interval", "i", 5, "刷新间隔 (秒)")
	logsCmd.Flags().StringVarP(&model, "model", "m", "", "过滤模型")
	logsCmd.Flags().BoolVarP(&textMode, "text", "t", false, "文本模式 (非交互环境)")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadWithAutoLogin()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	if textMode {
		runLogsText(c, interval, model)
		return
	}

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
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	tick := 0
	for {
		clearScreen()
		printLogs(c, model, tick)
		tick++

		select {
		case <-ticker.C:
			continue
		}
	}
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
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

	fmt.Println(headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 实时日志 (刷新: %ds) | Ctrl+C 退出 ", interval)))
	fmt.Println()

	if resp == nil || len(resp.Data) == 0 {
		fmt.Println(contentStyle.Render("暂无数据"))
	} else {
		// 显示日志条目
		for _, entry := range resp.Data {
			// 过滤模型
			if modelFilter != "" && entry.Model != modelFilter {
				continue
			}

			// 状态图标和颜色
			statusIcon := "✓"
			if entry.Status != "success" {
				statusIcon = "✗"
			}

			// 时间
			startTime := entry.StartTime
			if len(startTime) > 19 {
				startTime = startTime[:19]
			}

			// 显示
			fmt.Printf("%s %s ", statusIcon, contentStyle.Render(startTime))
			fmt.Printf("📦 %s ", contentStyle.Render(entry.Model))

			// Tokens
			if entry.TotalTokens > 0 {
				fmt.Printf("🔢 %d tokens ", entry.TotalTokens)
			}

			// 费用
			if entry.TotalSpend > 0 {
				fmt.Printf("%s", greenStyle.Render(fmt.Sprintf("$%.4f", entry.TotalSpend)))
			}

			// 延迟
			if entry.Latency > 0 {
				fmt.Printf(" ⏱ %.2fs", entry.Latency)
			}

			// 错误信息
			if entry.ErrorMessage != "" {
				fmt.Printf(" %s", errorStyle.Render(entry.ErrorMessage))
			}

			fmt.Println()
		}

		fmt.Println()
		fmt.Println(mutedStyle.Render(fmt.Sprintf("共 %d 条记录 (总 %d)", len(resp.Data), resp.Total)))
	}

	fmt.Println()
	fmt.Printf("⏱ 更新次数: %d | 时间: %s\n", tick, time.Now().Format("15:04:05"))
	fmt.Println(mutedStyle.Render("\n提示: 使用 --text 或 -t 参数可在非交互环境运行"))
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

// TUI 模式
type logsModel struct {
	client   *client.Client
	data     string
	interval int
	model    string
	tick     int
	quitting bool
}

func NewLogsModel(c *client.Client, interval int, model string) *logsModel {
	m := &logsModel{
		client:   c,
		interval: interval,
		model:    model,
		data:     "加载中...",
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
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tickMsg:
		m.refresh()
		m.tick++
		return m, tea.Tick(time.Duration(m.interval)*time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}
	return m, nil
}

func (m *logsModel) View() string {
	if m.quitting {
		return "👋 已退出\n"
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("236"))

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	return headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 日志 (刷新: %ds) | 按 q 退出 ", m.interval)) +
		"\n\n" +
		contentStyle.Render(m.data) +
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
			return
		}
		if respOld == nil || len(*respOld) == 0 {
			m.data = "暂无数据"
			return
		}
		m.data = fmt.Sprintf("✅ 获取到 %d 条日志记录", len(*respOld))
		return
	}

	if resp == nil || len(resp.Data) == 0 {
		m.data = "暂无数据"
		return
	}

	m.data = fmt.Sprintf("✅ 获取到 %d 条日志记录 (总 %d)", len(resp.Data), resp.Total)
}

type tickMsg time.Time