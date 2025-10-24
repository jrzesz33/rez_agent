package webaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/secrets"
)

// GolfHandler handles golf reservation actions
type GolfHandler struct {
	httpClient     *httpclient.Client
	oauthClient    *httpclient.OAuthClient
	secretsManager *secrets.Manager
	logger         *slog.Logger
}

// NewGolfHandler creates a new golf handler
func NewGolfHandler(httpClient *httpclient.Client, oauthClient *httpclient.OAuthClient, secretsManager *secrets.Manager, logger *slog.Logger) *GolfHandler {
	return &GolfHandler{
		httpClient:     httpClient,
		oauthClient:    oauthClient,
		secretsManager: secretsManager,
		logger:         logger,
	}
}

// GetActionType returns the action type this handler supports
func (h *GolfHandler) GetActionType() models.WebActionType {
	return models.WebActionTypeGolf
}

// Execute fetches golf reservations and formats notification
func (h *GolfHandler) Execute(ctx context.Context, payload *models.WebActionPayload) ([]string, error) {
	h.logger.Info("executing golf action",
		slog.String("url", payload.URL),
	)

	// Validate authentication configuration
	if payload.AuthConfig == nil || payload.AuthConfig.Type != models.AuthTypeOAuthPassword {
		return nil, fmt.Errorf("golf action requires OAuth password authentication")
	}

	// Perform OAuth authentication
	tokenURL := payload.AuthConfig.TokenURL
	secretName := payload.AuthConfig.SecretName
	scope := payload.AuthConfig.Scope

	// Additional headers for OAuth request
	oauthHeaders := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-language": "en-US,en;q=0.9",
		"cache-control":   "no-cache, no-store, must-revalidate",
		"client-id":       "onlineresweb",
		"origin":          "https://birdsfoot.cps.golf",
		"user-agent":      "Mozilla/5.0 (compatible; rez-agent/1.0)",
	}

	// Get OAuth token
	accessToken, err := h.oauthClient.OAuthPasswordGrant(ctx, tokenURL, secretName, scope, oauthHeaders)
	if err != nil {
		return nil, fmt.Errorf("OAuth authentication failed: %w", err)
	}

	h.logger.Info("OAuth authentication successful, fetching reservations")

	// Fetch reservations
	reservations, err := h.fetchReservations(ctx, payload.URL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reservations: %w", err)
	}

	// Format notification message
	notification := h.formatReservationNotification(reservations)

	h.logger.Info("golf action completed successfully",
		slog.Int("reservations_found", len(reservations)),
	)

	return notification, nil
}

// fetchReservations fetches golf reservations using the access token
func (h *GolfHandler) fetchReservations(ctx context.Context, apiURL, accessToken string) ([]GolfReservation, error) {
	headers := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-language": "en-US,en;q=0.9",
		"authorization":   fmt.Sprintf("Bearer %s", accessToken),
		"cache-control":   "no-cache, no-store, must-revalidate",
		"client-id":       "onlineresweb",
		"referer":         "https://birdsfoot.cps.golf/onlineresweb/my-reservation",
		"user-agent":      "Mozilla/5.0 (compatible; rez-agent/1.0)",
		"x-componentid":   "1",
	}

	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method:  "GET",
		URL:     apiURL,
		Headers: headers,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Parse response
	var apiResp GolfAPIResponse
	if err := json.Unmarshal([]byte(resp.Body), &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse reservations response: %w", err)
	}

	// Extract reservations from response
	if len(apiResp.Items) < 1 {
		h.logger.Warn("no reservations found in response")
		return []GolfReservation{}, nil
	}

	return apiResp.Items, nil
}

// GolfAPIResponse represents the golf API response structure
type GolfAPIResponse struct {
	Items       []GolfReservation `json:"items"`
	TotalCount  int               `json:"totalItems"`
	CurrentPage int               `json:"currentPage"`
	TotalPages  int               `json:"totalPages"`
}

// GolfReservation represents a single golf reservation
type GolfReservation struct {
	ReservationID   int       `json:"reservationId"`
	DateTime        string    `json:"startTime"`
	CourseName      string    `json:"courseName"`
	NumberOfPlayers int       `json:"numberOfPlayer"`
	ConfirmationNum string    `json:"reservationConfirmKey"`
	TeeTimeDT       time.Time // Parsed time for sorting
}

// formatReservationNotification formats reservations into a readable notification
func (h *GolfHandler) formatReservationNotification(reservations []GolfReservation) []string {
	var sb strings.Builder
	var strOut []string
	if len(reservations) == 0 {
		sb.WriteString("⛳ Golf Reservations\n\n")
		sb.WriteString("No upcoming tee times found.\n")
		strOut = append(strOut, sb.String())
		return strOut
	}

	// Parse tee times and sort by date
	for i := range reservations {
		teeTime, err := time.Parse(time.RFC3339, reservations[i].DateTime)
		if err != nil {
			// Try alternative format
			teeTime, err = time.Parse("2006-01-02T15:04:05", reservations[i].DateTime)
			if err != nil {
				h.logger.Warn("failed to parse tee time",
					slog.String("date_time", reservations[i].DateTime),
					slog.String("error", err.Error()),
				)
				continue
			}
		}
		reservations[i].TeeTimeDT = teeTime
	}

	// Sort by tee time (earliest first)
	sort.Slice(reservations, func(i, j int) bool {
		return reservations[i].TeeTimeDT.Before(reservations[j].TeeTimeDT)
	})

	// Limit to 4 tee times
	maxReservations := 4
	if len(reservations) > maxReservations {
		reservations = reservations[:maxReservations]
	}

	sb.WriteString("⛳ Upcoming Tee Times\n\n")

	for i, res := range reservations {
		// Format tee time
		teeTimeStr := res.TeeTimeDT.Format("Mon, Jan 2 at 3:04 PM")

		// Days until tee time
		daysUntil := int(time.Until(res.TeeTimeDT).Hours() / 24)
		urgency := ""
		if daysUntil == 0 {
			urgency = " 🔴 TODAY"
		} else if daysUntil == 1 {
			urgency = " 🟡 TOMORROW"
		} else if daysUntil <= 3 {
			urgency = fmt.Sprintf(" 🟢 in %d days", daysUntil)
		}

		// Reservation header
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, teeTimeStr, urgency))

		// Course name
		if res.CourseName != "" {
			sb.WriteString(fmt.Sprintf("   📍 %s\n", res.CourseName))
		}

		// Players
		sb.WriteString(fmt.Sprintf("   👥 %d player(s)\n", res.NumberOfPlayers))

		// Confirmation number
		if res.ConfirmationNum != "" {
			sb.WriteString(fmt.Sprintf("   🎟️ Confirmation: %s\n", res.ConfirmationNum))
		}

		// Separator
		if i < len(reservations)-1 {
			sb.WriteString("\n")
		}
	}

	// Footer
	if len(reservations) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n🏌️ Total: %d upcoming reservation(s)", len(reservations)))
	}
	strOut = append(strOut, sb.String())
	return strOut
}
