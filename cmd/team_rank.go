package cmd

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"litellm-cli/internal/api"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "查看团队用量排行榜",
	Run:   runTeam,
}

var teamID string

func init() {
	teamCmd.Flags().StringVar(&teamID, "team-id", "", "团队 ID (不指定则显示所有团队)")
	rootCmd.AddCommand(teamCmd)
}

func runTeam(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadWithAutoLogin()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	// 获取用户信息（包含团队列表）
	userInfo, err := c.GetUserInfo()
	if err != nil {
		log.Fatalf("获取用户信息失败: %v", err)
	}

	if len(userInfo.Teams) == 0 {
		log.Fatal("没有找到所属团队")
	}

	// 找到目标团队
	var targetTeam *api.UserTeam
	if teamID != "" {
		for i := range userInfo.Teams {
			if userInfo.Teams[i].TeamID == teamID {
				targetTeam = &userInfo.Teams[i]
				break
			}
		}
		if targetTeam == nil {
			log.Fatalf("未找到团队: %s", teamID)
		}
	} else {
		// 默认使用第一个团队
		targetTeam = &userInfo.Teams[0]
	}

	printTeamLeaderboard(targetTeam, userInfo.UserID)
}

func printTeamLeaderboard(team *api.UserTeam, currentUserID string) {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("236"))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))

	// 使用 team_info 中的数据
	teamAlias := team.TeamAlias
	teamSpend := team.Spend

	// 构建 user_id -> email 映射
	userEmailMap := make(map[string]string)
	for _, m := range team.MembersWithRoles {
		userEmailMap[m.UserID] = m.Email
	}

	// 按 user_id 聚合用量
	userSpend := make(map[string]float64)
	for _, k := range team.Keys {
		if k.UserID != "" {
			userSpend[k.UserID] += k.Spend
		}
	}

	// 样式
	greenStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("76"))
	yellowStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("226"))

	// 转换为排序切片
	type userRank struct {
		userID  string
		email   string
		spend   float64
		percent float64
	}
	var ranks []userRank
	for uid, spend := range userSpend {
		email := userEmailMap[uid]
		if email == "" {
			email = uid[:8] + "..."
		}
		percent := 0.0
		if teamSpend > 0 {
			percent = (spend / teamSpend) * 100
		}
		ranks = append(ranks, userRank{
			userID:  uid,
			email:   email,
			spend:   spend,
			percent: percent,
		})
	}

	// 按用量降序排序
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i].spend > ranks[j].spend
	})

	// 打印
	fmt.Println()
	fmt.Println(headerStyle.Render(fmt.Sprintf(" 🏆 %s 用量排行榜 ", teamAlias)))
	fmt.Println()
	fmt.Println(contentStyle.Render(fmt.Sprintf(" 团队总用量: %s", greenStyle.Render(fmt.Sprintf("$%.2f", teamSpend)))))
	fmt.Println()

	// 表头
	fmt.Printf("  %-4s %-30s %-11s %-8s\n", "排名", "用户", "用量", "占比")
	fmt.Println(mutedStyle.Render(" " + strings.Repeat("─", 60)))

	// 找出当前用户排名
	var myRank int
	var mySpend float64
	var myPercent float64

	for i, r := range ranks {
		rank := i + 1
		email := r.email
		if len(email) > 28 {
			email = email[:25] + "..."
		}

		// 高亮当前用户
		isMe := r.userID == currentUserID
		style := contentStyle
		rankStr := fmt.Sprintf("#%d", rank)

		if isMe {
			style = cyanStyle.Bold(true)
			rankStr = "→" + fmt.Sprintf("%d", rank)
			myRank = rank
			mySpend = r.spend
			myPercent = r.percent
		}

		percentStr := fmt.Sprintf("%.1f%%", r.percent)
		spendStr := fmt.Sprintf("$%.2f", r.spend)

		// 手动对齐
		spendStrPadded := spendStr + strings.Repeat(" ", int(math.Max(0, 10-float64(len(spendStr)))))
		percentStrPadded := percentStr + strings.Repeat(" ", int(math.Max(0, 8-float64(len(percentStr)))))

		fmt.Printf(style.Render("  %-4s %-30s ")+"%s %s\n",
			rankStr,
			email,
			greenStyle.Render(spendStrPadded),
			yellowStyle.Render(percentStrPadded),
		)
	}

	// 显示我的排名统计
	if myRank > 0 {
		fmt.Println()
		fmt.Println(cyanStyle.Render(fmt.Sprintf(" 📊 你的排名: #%d / %d", myRank, len(ranks))))
		fmt.Println(cyanStyle.Render(fmt.Sprintf("    你的用量: %s (占总用量 %.1f%%)", greenStyle.Render(fmt.Sprintf("$%.2f", mySpend)), myPercent)))
	}

	// 图示化
	if len(ranks) > 0 && teamSpend > 0 {
		fmt.Println()
		fmt.Println(mutedStyle.Render(" 用量分布:"))
		barWidth := 30
		for i, r := range ranks {
			if i >= 10 { // 只显示 top 10
				break
			}
			barLen := int((r.spend / teamSpend) * float64(barWidth))
			if barLen == 0 && r.spend > 0 {
				barLen = 1
			}
			bar := strings.Repeat("█", barLen)
			if r.userID == currentUserID {
				fmt.Printf("  %s %s\n", cyanStyle.Render(bar), mutedStyle.Render(fmt.Sprintf(" ← 你 (%.1f%%)", r.percent)))
			} else {
				fmt.Printf("  %s\n", contentStyle.Render(bar))
			}
		}
	}

	fmt.Println()
}