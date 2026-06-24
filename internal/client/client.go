package client

import (
	"litellm-cli/internal/api"
	"litellm-cli/internal/config"
)

type Client struct {
	api    *api.Client
	config *config.Config
}

func New(cfg *config.Config) *Client {
	return &Client{
		api:    api.NewClient(cfg),
		config: cfg,
	}
}

func (c *Client) GetUserDailyActivity(startDate, endDate string) (*api.UserDailyActivityResponse, error) {
	return c.api.GetUserDailyActivity(startDate, endDate)
}

func (c *Client) GetTeamDailyActivity(startDate, endDate string) (*api.TeamDailyActivityResponse, error) {
	return c.api.GetTeamDailyActivity(startDate, endDate)
}

func (c *Client) GetSpendLogs(startDate, endDate string) (*api.SpendLogsResponse, error) {
	return c.api.GetSpendLogs(startDate, endDate)
}

func (c *Client) GetModels() (*api.ModelsResponse, error) {
	return c.api.GetModels()
}

func (c *Client) GetKeyInfo(apiKey string) (*api.KeyInfoResponse, error) {
	return c.api.GetKeyInfo(apiKey)
}

func (c *Client) GetAPIKey() string {
	return c.config.APIKey
}

func (c *Client) GetTeamAvailable() (*api.TeamAvailableResponse, error) {
	return c.api.GetTeamAvailable()
}