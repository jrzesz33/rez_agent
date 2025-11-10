package webaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/pkg/courses"
)

// WeatherHandler handles weather forecast actions
type WeatherHandler struct {
	httpClient *httpclient.Client
	logger     *slog.Logger
}

// NewWeatherHandler creates a new weather handler
func NewWeatherHandler(httpClient *httpclient.Client, logger *slog.Logger) *WeatherHandler {
	return &WeatherHandler{
		httpClient: httpClient,
		logger:     logger,
	}
}

// GetActionType returns the action type this handler supports
func (h *WeatherHandler) GetActionType() models.WebActionType {
	return models.WebActionTypeWeather
}

// Execute fetches weather forecast and formats notification
func (h *WeatherHandler) Execute(ctx context.Context, args map[string]interface{}, payload *models.WebActionPayload) ([]string, error) {
	h.logger.Debug("executing weather action",
		slog.String("url", payload.URL),
	)
	course, err := courses.GetCourseByID(payload.CourseID)
	if err != nil {
		return nil, fmt.Errorf("failed to load course configuration: %w", err)
	}
	// Route based on operation type
	operation, _ := args["operation"].(string)
	if operation == "" {
		operation = "get_weather"
	}
	// Add course config to payload
	payload.AddCourseConfig(operation, *course)

	h.logger.Debug("loaded course configuration",
		slog.Int("course_id", course.CourseID),
		slog.String("course_name", course.Name),
		slog.String("origin", course.Origin),
		slog.String("url", payload.URL),
	)

	// Extract number of days from arguments (default: 2)
	numDays := 2
	if payload.Days > 0 {
		numDays = payload.Days
	}

	// Fetch weather data
	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method: "GET",
		URL:    payload.URL,
		Headers: map[string]string{
			"Accept":     "application/json",
			"User-Agent": "rez-agent weather notifier (contact@example.com)",
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

	// Format notification message
	notification := h.formatWeatherNotification(weatherData, numDays)

	h.logger.Debug("weather action completed successfully",
		slog.Int("num_days", numDays),
		slog.Int("periods_found", len(weatherData.Properties.Periods)),
	)

	return notification, nil
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

// formatWeatherNotification formats weather data into a readable notification
func (h *WeatherHandler) formatWeatherNotification(data WeatherAPIResponse, numDays int) []string {
	var sb strings.Builder
	var strOut []string

	// Calculate how many periods to include (2 periods per day: day and night)
	maxPeriods := numDays * 2
	if len(data.Properties.Periods) < maxPeriods {
		maxPeriods = len(data.Properties.Periods)
	}

	// Include detailed forecast for each period
	for i := 0; i < maxPeriods; i++ {

		period := data.Properties.Periods[i]

		if sb.Len() == 0 {
			sb.WriteString("🌤️ Weather Forecast\n")
		}

		// Period header
		sb.WriteString(fmt.Sprintf("📅 %s\n", period.Name))

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

		// Short forecast
		//sb.WriteString(fmt.Sprintf("☁️ %s\n", period.ShortForecast))

		// Detailed forecast
		sb.WriteString(fmt.Sprintf("☁️ %s\n", period.DetailedForecast))

		// Separator between periods
		//if i < maxPeriods-1 {
		//	sb.WriteString("\n" + strings.Repeat("─", 40) + "\n\n")
		//}

		if !period.IsDaytime {
			strOut = append(strOut, sb.String())
			sb.Reset()
		} else {
			sb.WriteString("\n\n")
		}
	}

	/*/ Footer with update time
	if data.Properties.Updated != "" {
		updateTime, err := time.Parse(time.RFC3339, data.Properties.Updated)
		if err == nil {
			sb.WriteString(fmt.Sprintf("\n\nUpdated: %s", updateTime.Format("Mon Jan 2, 3:04 PM MST")))
		}
	}
	//*/

	return strOut
}
