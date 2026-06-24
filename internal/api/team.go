package api

// TeamAvailableResponse represents /team/available response
type TeamAvailableResponse struct {
	Teams []TeamInfo `json:"teams"`
}

type TeamInfo struct {
	TeamID    string `json:"team_id"`
	TeamAlias string `json:"team_alias,omitempty"`
	TeamName  string `json:"team_name,omitempty"`
	CreatedAt string `json:"created_at"`
	Members   int64  `json:"members"`
}

// GetTeamAvailable 获取可用团队
func (c *Client) GetTeamAvailable() (*TeamAvailableResponse, error) {
	var result TeamAvailableResponse
	err := c.Get("/team/available", &result)
	return &result, err
}