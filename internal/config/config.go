package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	APIKey  string
	BaseURL string
}

func Load() (*Config, error) {
	apiKey := viper.GetString("api_key")
	baseURL := viper.GetString("base_url")

	if apiKey == "" {
		apiKey = os.Getenv("LITELLM_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API Key 未设置，请通过 --api-key 或 LITELLM_API_KEY 环境变量配置")
	}

	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}

	return &Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}, nil
}

func GetAPIKey() string {
	if key := viper.GetString("api_key"); key != "" {
		return key
	}
	return os.Getenv("LITELLM_API_KEY")
}

func GetBaseURL() string {
	if url := viper.GetString("base_url"); url != "" {
		return url
	}
	return "http://localhost:4000"
}