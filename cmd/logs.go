package cmd

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
	"litellm-cli/internal/tui/logs"
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
		logs.NewModel(c, interval, model),
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