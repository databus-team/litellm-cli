package stats

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"litellm-cli/internal/api"
	"litellm-cli/internal/client"
	"litellm-cli/internal/config"
	"litellm-cli/internal/testutils"
)

func init() {
	// 强制色彩 Profile 锁定，杜绝 CI 中的色彩漂移
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// mockTransport 拦截底层 Resty 请求并返回内存中的响应
type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestStatsTUI_RenderCharts(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	mockUserResp := api.UserDailyActivityResponse{
		Results: []api.UserDailyActivity{
			{
				Date: "2026-06-24",
				Metrics: api.ActivityMetrics{
					Spend:              2.50,
					PromptTokens:       50000,
					CompletionTokens:   20000,
					TotalTokens:        70000,
					SuccessfulRequests: 95,
					FailedRequests:     5,
					APIRequests:        100,
				},
			},
			{
				Date: "2026-06-25",
				Metrics: api.ActivityMetrics{
					Spend:              5.00,
					PromptTokens:       100000,
					CompletionTokens:   40000,
					TotalTokens:        140000,
					SuccessfulRequests: 190,
					FailedRequests:     10,
					APIRequests:        200,
				},
			},
			{
				Date: "2026-06-26",
				Metrics: api.ActivityMetrics{
					Spend:              1.25,
					PromptTokens:       25000,
					CompletionTokens:   10000,
					TotalTokens:        35000,
					SuccessfulRequests: 48,
					FailedRequests:     2,
					APIRequests:        50,
				},
			},
		},
	}
	mockUserBytes, _ := json.Marshal(mockUserResp)

	transport := &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")

			if strings.Contains(req.URL.Path, "/user/daily/activity") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(string(mockUserBytes))),
					Header:     header,
				}, nil
			}
			return nil, errors.New("unhandled mock path: " + req.URL.Path)
		},
	}

	cfg := &config.Config{
		APIKey:  "test-key",
		BaseURL: "https://mock-api.litellm.local",
	}
	c := client.New(cfg, api.WithTransport(transport))

	m := NewModel(c, "2026-06-24", "2026-06-26")
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newModel.(*Model)

	newModel, _ = m.Update(StatsLoadedMsg{Data: mockUserResp.Results})
	m = newModel.(*Model)

	if m.loading {
		t.Fatal("expected loading to be false")
	}
	if len(m.data) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(m.data))
	}
	testutils.AssertTUISnapshot(t, "stats_counter_view", m.View())

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newModel.(*Model)

	if m.viewMode != "bar" {
		t.Fatalf("expected viewMode to be 'bar', got '%s'", m.viewMode)
	}
	if m.selectedBarIndex != 0 {
		t.Fatalf("expected selectedBarIndex to be 0, got %d", m.selectedBarIndex)
	}
	testutils.AssertTUISnapshot(t, "stats_bar_view_index0", m.View())

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = newModel.(*Model)

	if m.selectedBarIndex != 1 {
		t.Fatalf("expected selectedBarIndex to be 1, got %d", m.selectedBarIndex)
	}
	testutils.AssertTUISnapshot(t, "stats_bar_view_index1", m.View())
}

func TestStatsTUI_ResponsiveLayout(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	mockUserResp := api.UserDailyActivityResponse{
		Results: []api.UserDailyActivity{
			{
				Date: "2026-06-25",
				Metrics: api.ActivityMetrics{
					Spend:              5.00,
					PromptTokens:       100000,
					CompletionTokens:   40000,
					TotalTokens:        140000,
					SuccessfulRequests: 190,
					FailedRequests:     10,
					APIRequests:        200,
				},
			},
		},
	}

	cfg := &config.Config{
		APIKey:  "test-key",
		BaseURL: "https://mock-api.litellm.local",
	}
	c := client.New(cfg)

	// Scene A: Large screen (Width: 140, Height: 40)
	m := NewModel(c, "2026-06-25", "2026-06-25")
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = newModel.(*Model)

	newModel, _ = m.Update(StatsLoadedMsg{Data: mockUserResp.Results})
	m = newModel.(*Model)

	testutils.AssertTUISnapshot(t, "stats_large_screen_layout", m.View())

	// Scene B: Small screen (Width: 80, Height: 24)
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = newModel.(*Model)

	testutils.AssertTUISnapshot(t, "stats_small_screen_layout", m.View())
}

func TestStatsTUI_NoData(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	cfg := &config.Config{
		APIKey:  "test-key",
		BaseURL: "https://mock-api.litellm.local",
	}
	c := client.New(cfg)

	m := NewModel(c, "2026-06-25", "2026-06-25")
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = newModel.(*Model)

	newModel, _ = m.Update(StatsLoadedMsg{Data: []api.UserDailyActivity{}})
	m = newModel.(*Model)

	if len(m.data) != 0 {
		t.Fatal("expected empty data")
	}

	view := m.View()
	if !strings.Contains(view, "暂无数据") {
		t.Errorf("expected view to contain '暂无数据', got: %s", view)
	}

	testutils.AssertTUISnapshot(t, "stats_no_data", view)
}

func TestStatsTUI_TeamData(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	mockTeamResp := api.TeamDailyActivityResponse{
		Results: []api.TeamDailyActivity{
			{
				Date: "2026-06-25",
				Metrics: api.ActivityMetrics{
					Spend:              8.50,
					PromptTokens:       150000,
					CompletionTokens:   60000,
					TotalTokens:        210000,
					SuccessfulRequests: 290,
					FailedRequests:     10,
					APIRequests:        300,
				},
			},
		},
	}
	mockTeamBytes, _ := json.Marshal(mockTeamResp)

	transport := &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")

			if strings.Contains(req.URL.Path, "/team/daily/activity") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(string(mockTeamBytes))),
					Header:     header,
				}, nil
			}
			return nil, errors.New("unhandled mock path: " + req.URL.Path)
		},
	}

	cfg := &config.Config{
		APIKey:  "test-key",
		BaseURL: "https://mock-api.litellm.local",
	}
	c := client.New(cfg, api.WithTransport(transport))

	m := NewModel(c, "2026-06-25", "2026-06-25")
	m.By = "team"

	cmd := m.RefreshCmd()
	msg := cmd()

	loadedMsg, ok := msg.(StatsLoadedMsg)
	if !ok {
		t.Fatalf("expected StatsLoadedMsg, got %T", msg)
	}

	if loadedMsg.Error != nil {
		t.Fatalf("expected no error, got %v", loadedMsg.Error)
	}

	if len(loadedMsg.Data) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(loadedMsg.Data))
	}

	if loadedMsg.Data[0].Metrics.Spend != 8.50 {
		t.Errorf("expected spend to be 8.50, got %.2f", loadedMsg.Data[0].Metrics.Spend)
	}
}
