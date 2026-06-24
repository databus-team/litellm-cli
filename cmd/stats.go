package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var (
	period string
	by     string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "查看用量统计",
	Run:   runStats,
}

func init() {
	statsCmd.Flags().StringVar(&period, "period", "day", "统计周期: day, week, month")
	statsCmd.Flags().StringVar(&by, "by", "user", "聚合维度: user, team, model")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	startDate, endDate := getDateRange(period)

	switch by {
	case "user":
		printUserStats(c, startDate, endDate)
	case "team":
		printTeamStats(c, startDate, endDate)
	default:
		printUserStats(c, startDate, endDate)
	}
}

func getDateRange(period string) (string, string) {
	now := time.Now()
	endDate := now.Format("2006-01-02")

	var startDate time.Time
	switch period {
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	default: // day
		startDate = now
	}

	return startDate.Format("2006-01-02"), endDate
}

func printUserStats(c *client.Client, startDate, endDate string) {
	resp, err := c.GetUserDailyActivity(startDate, endDate)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n📊 用户用量统计 (%s - %s)\n", startDate, endDate)
	fmt.Println("========================================")

	if len(resp.Results) == 0 {
		fmt.Println("暂无数据")
		return
	}

	// 显示最新一天的数据
	latest := resp.Results[0]
	fmt.Printf("\n📅 %s\n", latest.Date)
	fmt.Printf("   💰 花费: $%.4f\n", latest.Metrics.Spend)
	fmt.Printf("   📝 Prompt Tokens: %d\n", latest.Metrics.PromptTokens)
	fmt.Printf("   ✍️ Completion Tokens: %d\n", latest.Metrics.CompletionTokens)
	fmt.Printf("   📊 总 Tokens: %d\n", latest.Metrics.TotalTokens)
	fmt.Printf("   ✅ 成功请求: %d\n", latest.Metrics.SuccessfulRequests)
	fmt.Printf("   ❌ 失败请求: %d\n", latest.Metrics.FailedRequests)
	fmt.Printf("   📤 总请求: %d\n", latest.Metrics.APIRequests)

	// 按模型显示
	if len(latest.Breakdown.Models) > 0 {
		fmt.Println("\n📦 按模型:")
		for model, data := range latest.Breakdown.Models {
			fmt.Printf("   %s: $%.4f (%d tokens)\n", model, data.Metrics.Spend, data.Metrics.TotalTokens)
		}
	}
}

func printTeamStats(c *client.Client, startDate, endDate string) {
	resp, err := c.GetTeamDailyActivity(startDate, endDate)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n📊 团队用量统计 (%s - %s)\n", startDate, endDate)
	fmt.Println("========================================")

	if len(resp.Results) == 0 {
		fmt.Println("暂无数据")
		return
	}

	// 显示最新一天的数据
	latest := resp.Results[0]
	fmt.Printf("\n📅 %s\n", latest.Date)
	fmt.Printf("   💰 花费: $%.4f\n", latest.Metrics.Spend)
	fmt.Printf("   📝 Prompt Tokens: %d\n", latest.Metrics.PromptTokens)
	fmt.Printf("   ✍️ Completion Tokens: %d\n", latest.Metrics.CompletionTokens)
	fmt.Printf("   📊 总 Tokens: %d\n", latest.Metrics.TotalTokens)
	fmt.Printf("   ✅ 成功请求: %d\n", latest.Metrics.SuccessfulRequests)
	fmt.Printf("   ❌ 失败请求: %d\n", latest.Metrics.FailedRequests)
	fmt.Printf("   📤 总请求: %d\n", latest.Metrics.APIRequests)

	// 按模型显示
	if len(latest.Breakdown.Models) > 0 {
		fmt.Println("\n📦 按模型:")
		for model, data := range latest.Breakdown.Models {
			fmt.Printf("   %s: $%.4f (%d tokens)\n", model, data.Metrics.Spend, data.Metrics.TotalTokens)
		}
	}
}