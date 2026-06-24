package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
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
	cfg, err := config.Load()
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
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	resp, err := c.GetSpendLogs(startDate, endDate)
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
		return
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	fmt.Println(headerStyle.Render(fmt.Sprintf(" 📊 LiteLLM 日志 (刷新: %ds) | Ctrl+C 退出 ", interval)))
	fmt.Println()

	if resp == nil || len(*resp) == 0 {
		fmt.Println(contentStyle.Render("暂无数据"))
	} else {
		fmt.Println(contentStyle.Render(fmt.Sprintf("✅ 获取到 %d 条日志记录", len(*resp))))
	}

	fmt.Println()
	fmt.Printf("⏱ 更新次数: %d | 时间: %s\n", tick, time.Now().Format("15:04:05"))
	fmt.Println("\n提示: 使用 --text 或 -t 参数可在非交互环境运行")
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
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	resp, err := m.client.GetSpendLogs(startDate, endDate)
	if err != nil {
		m.data = fmt.Sprintf("❌ 获取失败: %v", err)
		return
	}

	if resp == nil || len(*resp) == 0 {
		m.data = "暂无数据"
		return
	}

	m.data = fmt.Sprintf("✅ 获取到 %d 条日志记录", len(*resp))
}

type tickMsg time.Time