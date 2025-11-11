package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

// WeatherTool implements the get_weather MCP tool
type WeatherTool struct {
	httpClient *httpclient.Client
	logger     *slog.Logger
}

// NewWeatherTool creates a new weather tool
func NewWeatherTool(httpClient *httpclient.Client, logger *slog.Logger) *WeatherTool {
	return &WeatherTool{
		httpClient: httpClient,
		logger:     logger,
	}
}

// GetDefinition returns the tool's MCP definition
func (t *WeatherTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        "get_weather",
		Description: "Get current weather forecast for a location using weather.gov API (US locations only)",
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"location": {
					Type:        "string",
					Description: "URL of the the Course from weather.gov (e.g. https://api.weather.gov/gridpoints/TOP/31,80/forecast)",
				},
				"days": {
					Type:        "integer",
					Description: "Number of days to forecast (default: 2)",
					Minimum:     intPtr(1),
					Maximum:     intPtr(7),
					Default:     2,
				},
			},
			Required: []string{"location"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *WeatherTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *WeatherTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	location := GetStringArg(args, "location", "")
	numDays := GetIntArg(args, "days", 2)

	if location == "" {
		return nil, fmt.Errorf("location cannot be empty")
	}

	t.logger.Info("fetching weather forecast",
		slog.String("location", location),
		slog.Int("days", numDays),
	)

	// Fetch weather data
	resp, err := t.httpClient.Do(ctx, httpclient.RequestConfig{
		Method: "GET",
		URL:    location,
		Headers: map[string]string{
			"Accept":     "application/json",
			"User-Agent": "rez-agent MCP weather tool (contact@example.com)",
		},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %w", err)
	}

	// Parse weather response
	var weatherData WeatherAPIResponse
	if err := json.Unmarshal([]byte(resp.Body), &weatherData); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %w", err)
	}

	// Format weather forecast
	forecast := t.formatWeatherForecast(weatherData, numDays)

	t.logger.Info("weather forecast retrieved successfully",
		slog.Int("periods", len(weatherData.Properties.Periods)),
	)

	return []protocol.Content{
		protocol.NewTextContent(forecast),
	}, nil
}

// WeatherAPIResponse represents the weather.gov API response structure
type WeatherAPIResponse struct {
	Properties struct {
		Updated string          `json:"updated"`
		Periods []WeatherPeriod `json:"periods"`
	} `json:"properties"`
}

// WeatherPeriod represents a single forecast period
type WeatherPeriod struct {
	Number           int    `json:"number"`
	Name             string `json:"name"`
	StartTime        string `json:"startTime"`
	EndTime          string `json:"endTime"`
	IsDaytime        bool   `json:"isDaytime"`
	Temperature      int    `json:"temperature"`
	TemperatureUnit  string `json:"temperatureUnit"`
	TemperatureTrend string `json:"temperatureTrend"`
	WindSpeed        string `json:"windSpeed"`
	WindDirection    string `json:"windDirection"`
	ShortForecast    string `json:"shortForecast"`
	DetailedForecast string `json:"detailedForecast"`
}

// formatWeatherForecast formats weather data into a readable forecast
func (t *WeatherTool) formatWeatherForecast(data WeatherAPIResponse, numDays int) string {
	var sb strings.Builder

	sb.WriteString("🌤️ Weather Forecast\n\n")

	// Calculate how many periods to include (2 periods per day: day and night)
	maxPeriods := numDays * 2
	if len(data.Properties.Periods) < maxPeriods {
		maxPeriods = len(data.Properties.Periods)
	}

	// Include detailed forecast for each period
	for i := 0; i < maxPeriods; i++ {
		period := data.Properties.Periods[i]

		// Period header
		sb.WriteString(fmt.Sprintf("📅 **%s**\n", period.Name))

		// Temperature
		tempEmoji := "🌡️"
		if period.Temperature < 32 {
			tempEmoji = "❄️"
		} else if period.Temperature > 80 {
			tempEmoji = "🔥"
		}
		sb.WriteString(fmt.Sprintf("%s %d°%s", tempEmoji, period.Temperature, period.TemperatureUnit))

		// Temperature trend
		if period.TemperatureTrend != "" {
			trendEmoji := "↗️"
			if period.TemperatureTrend == "falling" {
				trendEmoji = "↘️"
			}
			sb.WriteString(fmt.Sprintf(" %s %s", trendEmoji, period.TemperatureTrend))
		}
		sb.WriteString("\n")

		// Wind
		sb.WriteString(fmt.Sprintf("💨 Wind: %s %s\n", period.WindSpeed, period.WindDirection))

		// Detailed forecast
		sb.WriteString(fmt.Sprintf("☁️ %s\n", period.DetailedForecast))

		// Separator between periods
		if i < maxPeriods-1 {
			sb.WriteString("\n")
		}
	}

	// Footer with update time
	if data.Properties.Updated != "" {
		updateTime, err := time.Parse(time.RFC3339, data.Properties.Updated)
		if err == nil {
			sb.WriteString(fmt.Sprintf("\n_Updated: %s_", updateTime.Format("Mon Jan 2, 3:04 PM MST")))
		}
	}

	return sb.String()
}

// Helper function to create int pointer
