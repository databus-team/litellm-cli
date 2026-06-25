package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "查看可用模型列表",
	Run:   runModels,
}

func init() {
	rootCmd.AddCommand(modelsCmd)
}

func runModels(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadWithAutoLogin()
	if err != nil {
		log.Fatal(err)
	}

	c := client.New(cfg)

	resp, err := c.GetModels()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n📦 可用模型列表:")
	fmt.Println("================")

	if resp == nil || len(resp.Models) == 0 {
		fmt.Println("暂无数据")
		return
	}

	for _, model := range resp.Models {
		fmt.Printf("  • %s\n", model.ID)
	}

	fmt.Printf("\n共 %d 个模型\n", len(resp.Models))
}