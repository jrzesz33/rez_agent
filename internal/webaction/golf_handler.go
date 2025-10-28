package webaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

	// Parse and verify JWT claims WITH signature verification (CRITICAL SECURITY FIX)
	var claims *models.JWTClaims
	if payload.AuthConfig.JWKSURL != "" {
		claims, err = parseAndVerifyJWT(accessToken, payload.AuthConfig.JWKSURL)
		if err != nil {
			h.logger.Error("JWT verification failed", slog.String("error", err.Error()))
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
		h.logger.Info("JWT verified successfully",
			slog.String("golfer_id", claims.GolferID),
			slog.String("acct", claims.Acct))
	}

	// Route based on operation type
	operation, _ := payload.Arguments["operation"].(string)

	switch operation {
	case "search_tee_times":
		return h.handleSearchTeeTimes(ctx, payload, accessToken, claims)
	case "book_tee_time":
		if claims == nil {
			return nil, fmt.Errorf("JWT verification required for booking operations")
		}
		return h.handleBookTeeTime(ctx, payload, accessToken, claims)
	case "fetch_reservations", "":
		// Default to existing behavior
		return h.handleFetchReservations(ctx, payload, accessToken)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// handleFetchReservations handles fetching upcoming reservations
func (h *GolfHandler) handleFetchReservations(ctx context.Context, payload *models.WebActionPayload, accessToken string) ([]string, error) {
	h.logger.Info("fetching golf reservations")

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

// handleSearchTeeTimes searches for available tee times
func (h *GolfHandler) handleSearchTeeTimes(ctx context.Context, payload *models.WebActionPayload, accessToken string, claims *models.JWTClaims) ([]string, error) {
	h.logger.Info("searching for tee times")

	// Parse search parameters from payload.Arguments
	params, err := h.parseSearchTeeTimesParams(payload.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid search parameters: %w", err)
	}

	h.logger.Info("search parameters",
		slog.String("search_date", params.SearchDate),
		slog.Int("num_players", params.NumberOfPlayer),
		slog.Bool("auto_book", params.AutoBook))

	// Search for available tee times
	teeTimeSlots, err := h.searchTeeTimes(ctx, accessToken, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search tee times: %w", err)
	}

	h.logger.Info("tee times found",
		slog.Int("count", len(teeTimeSlots)))

	// If auto-book and tee times found, book the first one
	if params.AutoBook && len(teeTimeSlots) > 0 && claims != nil {

		h.logger.Info("auto-booking tee time for...", slog.Int("teeSheetId", teeTimeSlots[0].TeeSheetID))

		// Create a new payload for booking
		bookPayload := *payload
		bookPayload.Arguments = map[string]interface{}{
			"teeSheetId":      teeTimeSlots[0].TeeSheetID,
			"numberOfPlayer":  params.NumberOfPlayer,
			"startSearchTime": params.StartSearchTime,
		}

		return h.handleBookTeeTime(ctx, &bookPayload, accessToken, claims)
	}

	// Format search results as notification
	return h.formatSearchResults(teeTimeSlots, params), nil
}

// parseSearchTeeTimesParams parses search parameters from arguments
func (h *GolfHandler) parseSearchTeeTimesParams(args map[string]interface{}) (*models.SearchTeeTimesParams, error) {
	params := &models.SearchTeeTimesParams{
		NumberOfPlayer: 1, // Default
		AutoBook:       false,
	}

	// Extract number of players (optional, default 1)
	if numPlayers, ok := args["numberOfPlayer"].(float64); ok {
		params.NumberOfPlayer = int(numPlayers)
	}

	// Extract start time (optional)
	if startTime, ok := args["startSearchTime"].(string); ok && startTime != "" {
		params.StartSearchTime = &startTime
		_searchDate, err := time.Parse("2006-01-02T15:04:05", startTime)
		if err != nil {
			return nil, fmt.Errorf("invalid startSearchTime format: %w", err)
		}
		params.SearchDate = _searchDate.Format("Mon Jan 2 2006")

	} else {
		return nil, fmt.Errorf("startSearchTime is required")
	}

	// Extract end time (optional)
	if endTime, ok := args["endSearchTime"].(string); ok && endTime != "" {
		params.EndSearchTime = &endTime
	}

	// Extract auto-book flag (optional)
	if autoBook, ok := args["autoBook"].(bool); ok {
		params.AutoBook = autoBook
	} else {
		h.logger.Info("Auto Booking Not Requested")
	}

	// Validate number of players
	if params.NumberOfPlayer < 1 || params.NumberOfPlayer > 4 {
		return nil, fmt.Errorf("numberOfPlayer must be between 1 and 4")
	}

	return params, nil
}

// searchTeeTimes searches for available tee times
func (h *GolfHandler) searchTeeTimes(ctx context.Context, accessToken string, params *models.SearchTeeTimesParams) ([]models.TeeTimeSlot, error) {
	// Build search URL with query parameters
	baseURL := "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/TeeTimes"
	searchURL := fmt.Sprintf("%s?searchDate=%s&holes=0&numberOfPlayer=%d&courseIds=1&searchTimeType=0&teeSheetSearchView=5&classCode=R&defaultOnlineRate=N&isUseCapacityPricing=false&memberStoreId=1&searchType=1",
		baseURL,
		strings.ReplaceAll(params.SearchDate, " ", "%20"),
		params.NumberOfPlayer)

	headers := map[string]string{
		"accept":            "application/json, text/plain, */*",
		"accept-language":   "en-US,en;q=0.9",
		"authorization":     fmt.Sprintf("Bearer %s", accessToken),
		"cache-control":     "no-cache, no-store, must-revalidate",
		"client-id":         "onlineresweb",
		"user-agent":        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
		"x-componentid":     "1",
		"x-timezone-offset": "240",
		"x-timezoneid":      "America/New_York",
	}

	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method:  "GET",
		URL:     searchURL,
		Headers: headers,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Parse response
	var teeTimeSlots []models.TeeTimeSlot
	if err := json.Unmarshal([]byte(resp.Body), &teeTimeSlots); err != nil {
		h.logger.Warn("problem with response body", slog.String("resp", string(resp.Body)))
		if !strings.Contains(string(resp.Body), "NO_TEETIMES") {
			return nil, fmt.Errorf("failed to parse tee times response: %s", string(resp.Body))
		}
	}
	//h.logger.Info("get tee times", slog.Int("unmarshalled slots", len(teeTimeSlots)))

	// Filter by time range if specified
	if params.StartSearchTime != nil || params.EndSearchTime != nil {
		filteredSlots := make([]models.TeeTimeSlot, 0)
		for _, slot := range teeTimeSlots {
			withinRange, err := slot.IsWithinTimeRange(params.StartSearchTime, params.EndSearchTime)
			if err != nil {
				h.logger.Warn("failed to parse tee time",
					slog.String("start_time", slot.StartTime),
					slog.String("error", err.Error()))
				continue
			}
			if withinRange {
				filteredSlots = append(filteredSlots, slot)
			}
		}
		teeTimeSlots = filteredSlots
	}

	return teeTimeSlots, nil
}

// formatSearchResults formats tee time search results as notification
func (h *GolfHandler) formatSearchResults(slots []models.TeeTimeSlot, params *models.SearchTeeTimesParams) []string {
	var sb strings.Builder
	var strOut []string

	if len(slots) == 0 {
		sb.WriteString("⛳ Tee Time Search Results\n\n")
		sb.WriteString(fmt.Sprintf("No available tee times found for %s", params.SearchDate))
		if params.StartSearchTime != nil || params.EndSearchTime != nil {
			sb.WriteString("\nTry adjusting your time range.")
		}
		strOut = append(strOut, sb.String())
		return strOut
	}

	// Limit to 5 tee times
	maxResults := 5
	if len(slots) > maxResults {
		slots = slots[:maxResults]
	}

	sb.WriteString("⛳ Available Tee Times\n\n")
	sb.WriteString(fmt.Sprintf("Date: %s\n", params.SearchDate))
	sb.WriteString(fmt.Sprintf("Players: %d\n\n", params.NumberOfPlayer))

	for i, slot := range slots {
		// Parse and format tee time
		teeTime, err := slot.ParseStartTime()
		if err != nil {
			h.logger.Warn("failed to parse start time", slog.String("error", err.Error()))
			continue
		}
		teeTimeStr := teeTime.Format("3:04 PM")

		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, teeTimeStr))
		sb.WriteString(fmt.Sprintf("   📍 %s\n", slot.CourseName))
		sb.WriteString(fmt.Sprintf("   ⛳ %d holes available\n", slot.Holes))

		// Find pricing
		for _, price := range slot.ShItemPrices {
			if price.ShItemCode == "GreenFee18" {
				sb.WriteString(fmt.Sprintf("   💵 $%.2f - %s\n", price.Price, price.ItemDesc))
				break
			}
		}

		if i < len(slots)-1 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\n\nFound %d available time(s)", len(slots)))
	strOut = append(strOut, sb.String())
	return strOut
}

// handleBookTeeTime books a tee time (3-step process)
func (h *GolfHandler) handleBookTeeTime(ctx context.Context, payload *models.WebActionPayload, accessToken string, claims *models.JWTClaims) ([]string, error) {
	h.logger.Info("booking tee time")

	// Parse booking parameters
	params, err := h.parseBookTeeTimeParams(payload.Arguments)
	if err != nil {
		return nil, fmt.Errorf("invalid booking parameters: %w", err)
	}

	h.logger.Info("booking parameters",
		slog.Int("tee_sheet_id", params.TeeSheetID),
		slog.Int("num_players", params.NumberOfPlayer))

	// Step 1: Lock tee time
	lockResp, err := h.lockTeeTime(ctx, params, accessToken, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to lock tee time: %w", err)
	}

	if lockResp.Error != "" {
		return nil, fmt.Errorf("lock error: %s", lockResp.Error)
	}

	h.logger.Info("tee time locked",
		slog.String("session_id", lockResp.SessionID))

	// Step 2: Calculate pricing
	pricingResp, err := h.calculatePricing(ctx, params, accessToken, claims)
	if err != nil {
		// Lock will auto-expire server-side
		return nil, fmt.Errorf("pricing calculation failed: %w", err)
	}

	h.logger.Info("pricing calculated",
		slog.String("transaction_id", pricingResp.TransactionID),
		slog.Float64("total", pricingResp.SummaryDetail.Total))

	// Pause execution for 3 seconds
	time.Sleep(3 * time.Second)

	// Step 3: Reserve tee time
	reserveResp, err := h.reserveTeeTime(ctx, accessToken, claims, lockResp.SessionID, pricingResp.TransactionID)
	if err != nil {
		return nil, fmt.Errorf("reservation failed: %w", err)
	}

	h.logger.Info("tee time reserved",
		slog.Int("reservation_id", reserveResp.ReservationID),
		slog.String("confirmation_key", reserveResp.ConfirmationKey))

	// Format success notification
	return h.formatBookingSuccess(reserveResp, pricingResp), nil
}

// parseBookTeeTimeParams parses booking parameters from arguments
func (h *GolfHandler) parseBookTeeTimeParams(args map[string]interface{}) (*models.BookTeeTimeParams, error) {
	params := &models.BookTeeTimeParams{
		NumberOfPlayer: 1, // Default
	}

	// Extract teeSheetId (required)
	// Handle both int and float64 (JSON unmarshals numbers as float64)
	if teeSheetID, ok := args["teeSheetId"].(float64); ok {
		params.TeeSheetID = int(teeSheetID)
	} else if teeSheetID, ok := args["teeSheetId"].(int); ok {
		params.TeeSheetID = teeSheetID
	} else {
		return nil, fmt.Errorf("teeSheetId is required")
	}

	// Extract number of players (optional, default 1)
	// Handle both int and float64 (JSON unmarshals numbers as float64)
	if numPlayers, ok := args["numberOfPlayer"].(float64); ok {
		params.NumberOfPlayer = int(numPlayers)
	} else if numPlayers, ok := args["numberOfPlayer"].(int); ok {
		params.NumberOfPlayer = numPlayers
	}

	/*if startTime, ok := args["startSearchTime"].(string); ok && startTime != "" {
		_searchDate, err := time.Parse("2006-01-02T15:04:05", startTime)
		if err != nil {
			return nil, fmt.Errorf("invalid startSearchTime format: %w", err)
		}
		params.SearchDate = _searchDate.Format("Mon Jan 2 2006")

	} else {
		return nil, fmt.Errorf("startSearchTime is required")
	}*/

	// Validate
	if params.TeeSheetID <= 0 {
		return nil, fmt.Errorf("invalid teeSheetId")
	}

	if params.NumberOfPlayer < 1 || params.NumberOfPlayer > 4 {
		return nil, fmt.Errorf("numberOfPlayer must be between 1 and 4")
	}

	return params, nil
}

// lockTeeTime performs step 1 of booking (lock)
func (h *GolfHandler) lockTeeTime(ctx context.Context, params *models.BookTeeTimeParams, accessToken string, claims *models.JWTClaims) (*models.LockTeeTimeResponse, error) {
	sessionID := uuid.New().String() //time.Now().Format("20060102-150405")

	_golferId, err := strconv.Atoi(claims.GolferID)
	if err != nil {
		return nil, fmt.Errorf("invalid GolferID in claims: %w", err)
	}

	lockReq := models.LockTeeTimeRequest{
		TeeSheetIDs:    []int{params.TeeSheetID},
		Email:          claims.Email, // Use email from JWT (security fix)
		Action:         "Online Reservation V5",
		SessionID:      sessionID,
		GolferID:       _golferId,
		ClassCode:      "R",
		NumberOfPlayer: params.NumberOfPlayer,
		NavigateURL:    "",
		IsGroupBooking: false,
	}

	headers := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-language": "en-US,en;q=0.9",
		"authorization":   fmt.Sprintf("Bearer %s", accessToken),
		"cache-control":   "no-cache, no-store, must-revalidate",
		"client-id":       "onlineresweb",
		"content-type":    "application/json",
		"x-componentid":   "1",
		"x-websiteid":     "94fa26b7-2e63-4cbc-99e5-08d7d7f41522",
	}

	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method:  "POST",
		URL:     "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/LockTeeTimes",
		Headers: headers,
		Body:    lockReq,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	h.logger.Info("lock tee time response", slog.String("body", resp.Body))
	// Parse response
	var lockResp models.LockTeeTimeResponse
	if err := json.Unmarshal([]byte(resp.Body), &lockResp); err != nil {
		return nil, fmt.Errorf("failed to parse lock response: %w", err)
	}
	if strings.Contains(lockResp.Warning, "already have a reservation") {
		return nil, fmt.Errorf("reservation conflict: %s", lockResp.Warning)
	}
	if lockResp.Error != "" {
		return nil, fmt.Errorf("issue with locking a tee time: %s", lockResp.Error)
	}

	return &lockResp, nil
}

// calculatePricing performs step 2 of booking (pricing)
func (h *GolfHandler) calculatePricing(ctx context.Context, params *models.BookTeeTimeParams, accessToken string, claims *models.JWTClaims) (*models.PricingCalculationResponse, error) {
	_golferId, err := strconv.Atoi(claims.GolferID)
	if err != nil {
		return nil, fmt.Errorf("invalid GolferID in claims: %w", err)
	}
	pricingReq := models.PricingCalculationRequest{
		SelectedTeeSheetID: params.TeeSheetID,
		BookingList: []models.PricingBookingItem{
			{
				TeeSheetID:           params.TeeSheetID,
				Holes:                18,
				ParticipantNo:        1,
				GolferID:             _golferId,
				RateCode:             "N",
				IsUnassignedPlayer:   false,
				MemberClassCode:      "R",
				MemberStoreID:        "1",
				CartType:             1,
				PlayerID:             "0",
				Acct:                 claims.Acct,
				IsGuestOf:            false,
				IsUseCapacityPricing: false,
			},
		},
		Holes:                18,
		NumberOfPlayer:       params.NumberOfPlayer,
		NumberOfRider:        1,
		CartType:             1,
		Coupon:               nil,
		DepositType:          0,
		DepositAmount:        0,
		SelectedValuePackage: nil,
		IsUseCapacityPricing: false,
		ThirdPartyID:         nil,
		IBXCardOnFile:        nil,
		TransactionID:        nil,
	}

	headers := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-language": "en-US,en;q=0.9",
		"authorization":   fmt.Sprintf("Bearer %s", accessToken),
		"cache-control":   "no-cache, no-store, must-revalidate",
		"client-id":       "onlineresweb",
		"content-type":    "application/json",
		"x-componentid":   "1",
		"x-websiteid":     "94fa26b7-2e63-4cbc-99e5-08d7d7f41522",
	}

	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method:  "POST",
		URL:     "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/TeeTimePricesCalculation",
		Headers: headers,
		Body:    pricingReq,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	h.logger.Info("pricing calculation response", slog.String("body", resp.Body))
	// Parse response
	var pricingResp models.PricingCalculationResponse
	if err := json.Unmarshal([]byte(resp.Body), &pricingResp); err != nil {
		return nil, fmt.Errorf("failed to parse pricing response: %w", err)
	}

	return &pricingResp, nil
}

// reserveTeeTime performs step 3 of booking (reserve)
func (h *GolfHandler) reserveTeeTime(ctx context.Context, accessToken string, claims *models.JWTClaims, sessionID, transactionID string) (*models.ReservationResponse, error) {
	reserveReq := models.ReserveTeeTimeRequest{
		CancelReservationLink: "https://birdsfoot.cps.golf/onlineresweb/auth/verify-email?returnUrl=cancel-booking",
		HomePageLink:          "https://birdsfoot.cps.golf/onlineresweb/",
		AffiliateID:           nil,
		FinalizeSaleModel: models.FinalizeSaleModel{
			Acct:     claims.Acct,
			PlayerID: 0,
			IsGuest:  false,
			CreditCardInfo: models.CreditCardInfo{
				CardNumber: nil,
				CardHolder: nil,
				ExpireMM:   nil,
				ExpireYY:   nil,
				CVV:        nil,
				Email:      claims.Email, // Use email from JWT (security fix)
				CardToken:  nil,
			},
			MonerisCC: nil,
			IBXCC:     nil,
		},
		SessionGUID:             nil,
		LockedTeeTimesSessionID: sessionID,
		TransactionID:           transactionID,
	}

	headers := map[string]string{
		"accept":             "application/json, text/plain, */*",
		"accept-language":    "en-US,en;q=0.9",
		"authorization":      fmt.Sprintf("Bearer %s", accessToken),
		"cache-control":      "no-cache, no-store, must-revalidate",
		"client-id":          "onlineresweb",
		"content-type":       "application/json",
		"x-componentid":      "1",
		"x-websiteid":        "94fa26b7-2e63-4cbc-99e5-08d7d7f41522",
		"if-modified-since":  "0",
		"origin":             "https://birdsfoot.cps.golf",
		"pragma":             "no-cache",
		"priority":           "u=1, i",
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": "macOS",
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
		"x-ismobile":         "true",
		"x-moduleid":         "7",
		"x-productid":        "1",
		"x-siteid":           "3",
		"x-terminalid":       "7",
		"x-timezone-offset":  "240",
		"x-timezoneid":       "America/New_York",
	}
	h.logger.Warn("reserve request", slog.String("body", fmt.Sprint(reserveReq)), slog.String("header", fmt.Sprint(headers)))

	resp, err := h.httpClient.Do(ctx, httpclient.RequestConfig{
		Method:  "POST",
		URL:     "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/ReserveTeeTimes",
		Headers: headers,
		Body:    reserveReq,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Parse response
	var reserveResp models.ReservationResponse
	if err := json.Unmarshal([]byte(resp.Body), &reserveResp); err != nil {
		return nil, fmt.Errorf("failed to parse reservation response: %w", err)
	}

	// Check if booking succeeded
	if reserveResp.ReservationResult != 1 {
		return nil, fmt.Errorf("reservation failed with result code: %d", reserveResp.ReservationResult)
	}

	return &reserveResp, nil
}

// formatBookingSuccess formats successful booking as notification
func (h *GolfHandler) formatBookingSuccess(reserve *models.ReservationResponse, pricing *models.PricingCalculationResponse) []string {
	var sb strings.Builder
	var strOut []string

	sb.WriteString("⛳ Tee Time Booked Successfully!\n\n")

	// Confirmation details
	sb.WriteString(fmt.Sprintf("Confirmation: %s\n", reserve.ConfirmationKey))
	sb.WriteString(fmt.Sprintf("Reservation ID: %d\n\n", reserve.ReservationID))

	// Tee time details
	teeTime, err := time.Parse("2006-01-02T15:04:05", pricing.StartTime)
	if err == nil {
		sb.WriteString(fmt.Sprintf("Date/Time: %s\n", teeTime.Format("Mon, Jan 2 at 3:04 PM")))
	}
	sb.WriteString(fmt.Sprintf("Course: %s\n", pricing.CourseName))
	sb.WriteString(fmt.Sprintf("Holes: %d\n\n", pricing.Holes))

	// Pricing
	sb.WriteString(fmt.Sprintf("Total: $%.2f\n", pricing.SummaryDetail.Total))
	sb.WriteString(fmt.Sprintf("Due at Course: $%.2f\n\n", pricing.SummaryDetail.TotalDueAtCourse))

	sb.WriteString("See you on the course!")
	strOut = append(strOut, sb.String())
	return strOut
}
