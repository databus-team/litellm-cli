package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"litellm-cli/internal/config"
)

type Client struct {
	resty  *resty.Client
	config *config.Config
}

func NewClient(cfg *config.Config) *Client {
	client := resty.New()
	client.SetBaseURL(cfg.BaseURL)
	client.SetHeader("Authorization", "Bearer "+cfg.APIKey)
	client.SetHeader("Content-Type", "application/json")

	return &Client{
		resty:  client,
		config: cfg,
	}
}

func (c *Client) Get(path string, result interface{}) error {
	resp, err := c.resty.R().Get(path)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return c.parseError(resp.Body())
	}

	if result != nil {
		if err := json.Unmarshal(resp.Body(), result); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

func (c *Client) parseError(body []byte) error {
	var errResp map[string]interface{}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("请求失败，状态码: %s", string(body))
	}

	if errObj, ok := errResp["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok {
			return fmt.Errorf("%s", msg)
		}
	}

	return fmt.Errorf("请求失败: %s", string(body))
}

// UserDailyActivityResponse represents /user/daily/activity response
type UserDailyActivityResponse struct {
	Results   []UserDailyActivity `json:"results"`
	Metadata  Metadata            `json:"metadata"`
}

type UserDailyActivity struct {
	Date     string         `json:"date"`
	Metrics  ActivityMetrics `json:"metrics"`
	Breakdown Breakdown    `json:"breakdown"`
}

type ActivityMetrics struct {
	Spend                 float64 `json:"spend"`
	PromptTokens          int64   `json:"prompt_tokens"`
	CompletionTokens      int64   `json:"completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	SuccessfulRequests    int64   `json:"successful_requests"`
	FailedRequests       int64   `json:"failed_requests"`
	APIRequests          int64   `json:"api_requests"`
}

type Breakdown struct {
	Models map[string]ModelBreakdown `json:"models"`
	APIKeys map[string]APIKeyMetrics `json:"api_keys"`
}

type ModelBreakdown struct {
	Metrics ActivityMetrics `json:"metrics"`
}

type APIKeyMetrics struct {
	Metrics  ActivityMetrics `json:"metrics"`
	Metadata map[string]string `json:"metadata"`
}

type Metadata struct {
	TotalSpend            float64 `json:"total_spend"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalAPIRequests      int64   `json:"total_api_requests"`
}

// GetUserDailyActivity 获取用户每日活动
func (c *Client) GetUserDailyActivity(startDate, endDate string) (*UserDailyActivityResponse, error) {
	var result UserDailyActivityResponse
	err := c.Get(fmt.Sprintf("/user/daily/activity?start_date=%s&end_date=%s", startDate, endDate), &result)
	return &result, err
}

// TeamDailyActivityResponse represents /team/daily/activity response
type TeamDailyActivityResponse struct {
	Results []TeamDailyActivity `json:"results"`
}

type TeamDailyActivity struct {
	Date     string         `json:"date"`
	Metrics  ActivityMetrics `json:"metrics"`
	Breakdown Breakdown    `json:"breakdown"`
}

// GetTeamDailyActivity 获取团队每日活动
func (c *Client) GetTeamDailyActivity(startDate, endDate string) (*TeamDailyActivityResponse, error) {
	var result TeamDailyActivityResponse
	err := c.Get(fmt.Sprintf("/team/daily/activity?start_date=%s&end_date=%s", startDate, endDate), &result)
	return &result, err
}

// SpendLogsResponse represents /spend/logs response
type SpendLogsResponse []struct {
	Key     string                   `json:"51889b664c55b674542ddda7c3cb2d63a1b35f6a75e1664be7d2d4b3f2d841e0"`
	Models  map[string]float64       `json:"models"`
	Spend   float64                  `json:"spend"`
	StartTime string                `json:"startTime"`
	Users   map[string]float64      `json:"users"`
}

// GetSpendLogs 获取消费日志
func (c *Client) GetSpendLogs(startDate, endDate string) (*SpendLogsResponse, error) {
	var result SpendLogsResponse
	err := c.Get(fmt.Sprintf("/spend/logs?start_date=%s&end_date=%s", startDate, endDate), &result)
	return &result, err
}

// ModelsResponse represents /models response
type ModelsResponse struct {
	Models []ModelInfo `json:"data"`
}

type ModelInfo struct {
	Object      string `json:"object"`
	ID          string `json:"id"`
	ModelName   string `json:"model_name,omitempty"`
}

// GetModels 获取模型列表
func (c *Client) GetModels() (*ModelsResponse, error) {
	var result ModelsResponse
	err := c.Get("/models", &result)
	return &result, err
}

// KeyInfoResponse represents /key/info response
type KeyInfoResponse struct {
	Key  string    `json:"key"`
	Info KeyDetail `json:"info"`
}

type KeyDetail struct {
	KeyName      string             `json:"key_name"`
	KeyAlias     string             `json:"key_alias"`
	Spend        float64            `json:"spend"`
	Models       []string           `json:"models"`
	UserID       string             `json:"user_id"`
	TeamID       string             `json:"team_id"`
	CreatedAt    string             `json:"created_at"`
	LastActive   string             `json:"last_active"`
}

// GetKeyInfo 获取 Key 详情
func (c *Client) GetKeyInfo(apiKey string) (*KeyInfoResponse, error) {
	var result KeyInfoResponse
	err := c.Get("/key/info?api_key="+apiKey, &result)
	return &result, err
}