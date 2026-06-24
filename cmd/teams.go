package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var teamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "查看可用团队列表",
	Run:   runTeams,
}

func init() {
	rootCmd.AddCommand(teamsCmd)
}

func runTeams(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	resp, err := c.GetTeamAvailable()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n👥 可用团队列表:")
	fmt.Println("=================")

	if resp == nil || len(*resp) == 0 {
		fmt.Println("暂无数据")
		return
	}

	for _, team := range *resp {
		alias := team.TeamAlias
		if alias == "" {
			alias = team.TeamName
		}
		if alias == "" {
			alias = team.TeamID
		}
		fmt.Printf("  • %s (ID: %s)\n", alias, team.TeamID)
	}

	fmt.Printf("\n共 %d 个团队\n", len(*resp))
}