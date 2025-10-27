package models

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SearchTeeTimesParams contains parameters for tee time search
type SearchTeeTimesParams struct {
	SearchDate      string  `json:"searchDate"`      // "Wed Oct 29 2025"
	NumberOfPlayer  int     `json:"numberOfPlayer"`  // 1-4
	StartSearchTime *string `json:"startSearchTime"` // "2025-10-29T07:30:00" (optional)
	EndSearchTime   *string `json:"endSearchTime"`   // "2025-10-29T09:00:00" (optional)
	AutoBook        bool    `json:"autoBook"`        // Auto-book first available
}

// TeeTimeSlot represents an available tee time from the API
type TeeTimeSlot struct {
	TeeSheetID          int    `json:"teeSheetId"`
	StartTime           string `json:"startTime"` // ISO 8601
	CourseTimeID        int    `json:"courseTimeId"`
	StartingTee         int    `json:"startingTee"`
	CrossOverTeeSheetID int    `json:"crossOverTeeSheetId"`
	Participants        int    `json:"participants"` // Max capacity
	CourseID            int    `json:"courseId"`
	CourseDate          string `json:"courseDate"`
	DefaultRateCode     string `json:"defaultRateCode"`
	TeeTypeID           int    `json:"teeTypeId"`
	Holes               int    `json:"holes"`
	DefaultHoles        int    `json:"defaultHoles"`
	SiteID              int    `json:"siteId"`
	CourseName          string `json:"courseName"`
	//CourseNameIncludeCross string              `json:"courseNameIncludeCrossOver"`
	ShItemPrices []TeeTimePrice `json:"shItemPrices"`
	//ShItemPricesGroup      []interface{}       `json:"shItemPricesGroup"`
	HolesDisplay   string `json:"holesDisplay"`
	PlayersDisplay string `json:"playersDisplay"`
	MinPlayer      int    `json:"minPlayer"`
	MaxPlayer      int    `json:"maxPlayer"`
	//AvailableParticipantNo []int               `json:"availableParticipantNo"`
	//SummaryDetail          map[string]interface{} `json:"summaryDetail"`
	//PlayersRateCode        []interface{}       `json:"playersRateCode"`
	//PlayerNames            []interface{}       `json:"playerNames"`
	//BlockTexts             []interface{}       `json:"blockTexts"`
	DefaultClassCode string `json:"defaultClassCode"`
	TeeSuffix        string `json:"teeSuffix"`
}

// TeeTimePrice represents pricing for a tee time
type TeeTimePrice struct {
	ItemGuid          string  `json:"itemGuid"`
	ShItemCode        string  `json:"shItemCode"` // "GreenFee18" or "GreenFee9"
	ItemCode          string  `json:"itemCode"`
	Price             float64 `json:"price"`
	TaxInclusivePrice float64 `json:"taxInclusivePrice"`
	TaxCode           string  `json:"taxCode"`
	ItemDesc          string  `json:"itemDesc"`
	ClassCode         string  `json:"classCode"`
	RateCode          string  `json:"rateCode"`
	CurrentPrice      float64 `json:"currentPrice"`
	PriceType         int     `json:"priceType"`
	PriceTypeName     string  `json:"priceTypeName"`
}

// BookTeeTimeParams contains parameters for booking
type BookTeeTimeParams struct {
	TeeSheetID     int    `json:"teeSheetId"`
	NumberOfPlayer int    `json:"numberOfPlayer"`
	SearchDate     string `json:"searchDate"` // For context/logging
}

// JWTClaims contains parsed JWT token claims (MUST verify signature!)
type JWTClaims struct {
	GolferID string `json:"golferId"`
	Acct     string `json:"acct"`
	Email    string `json:"email"` // Extract from JWT
	jwt.RegisteredClaims
}

// LockTeeTimeRequest is the request body for locking a tee time
type LockTeeTimeRequest struct {
	TeeSheetIDs    []int  `json:"teeSheetIds"`
	Email          string `json:"email"`
	Action         string `json:"action"`    // "Online Reservation V5"
	SessionID      string `json:"sessionId"` // UUID
	GolferID       int    `json:"golferId"`
	ClassCode      string `json:"classCode"` // "R"
	NumberOfPlayer int    `json:"numberOfPlayer"`
	NavigateURL    string `json:"navigateUrl"`    // ""
	IsGroupBooking bool   `json:"isGroupBooking"` // false
}

// LockTeeTimeResponse is the response from lock tee time API
type LockTeeTimeResponse struct {
	TeeSheetIDs []int  `json:"teeSheetIds"`
	SessionID   string `json:"sessionId"`
	Error       string `json:"error"`
	Warning     string `json:"warning"`
}

// PricingCalculationRequest is the request body for pricing calculation
type PricingCalculationRequest struct {
	SelectedTeeSheetID   int                  `json:"selectedTeeSheetId"`
	BookingList          []PricingBookingItem `json:"bookingList"`
	Holes                int                  `json:"holes"`
	NumberOfPlayer       int                  `json:"numberOfPlayer"`
	NumberOfRider        int                  `json:"numberOfRider"`
	CartType             int                  `json:"cartType"`
	Coupon               interface{}          `json:"coupon"`
	DepositType          int                  `json:"depositType"`
	DepositAmount        float64              `json:"depositAmount"`
	SelectedValuePackage interface{}          `json:"selectedValuePackageCode"`
	IsUseCapacityPricing bool                 `json:"isUseCapacityPricing"`
	ThirdPartyID         interface{}          `json:"thirdPartyId"`
	IBXCardOnFile        interface{}          `json:"ibxCardOnFile"`
	TransactionID        interface{}          `json:"transactionId"`
}

// PricingBookingItem represents a booking item in pricing calculation
type PricingBookingItem struct {
	TeeSheetID           int    `json:"teeSheetId"`
	Holes                int    `json:"holes"`
	ParticipantNo        int    `json:"participantNo"`
	GolferID             int    `json:"golferId"`
	RateCode             string `json:"rateCode"`
	IsUnassignedPlayer   bool   `json:"isUnAssignedPlayer"`
	MemberClassCode      string `json:"memberClassCode"`
	MemberStoreID        string `json:"memberStoreId"`
	CartType             int    `json:"cartType"`
	PlayerID             string `json:"playerId"`
	Acct                 string `json:"acct"`
	IsGuestOf            bool   `json:"isGuestOf"`
	IsUseCapacityPricing bool   `json:"isUseCapacityPricing"`
}

// PricingCalculationResponse is the response from pricing calculation API
type PricingCalculationResponse struct {
	TeeSheetID             int                   `json:"teeSheetId"`
	StartTime              string                `json:"startTime"`
	CourseTimeID           int                   `json:"courseTimeId"`
	StartingTee            int                   `json:"startingTee"`
	Participants           int                   `json:"participants"`
	CourseID               int                   `json:"courseId"`
	CourseDate             string                `json:"courseDate"`
	TeeTypeID              int                   `json:"teeTypeId"`
	Holes                  int                   `json:"holes"`
	DefaultHoles           int                   `json:"defaultHoles"`
	SiteID                 int                   `json:"siteId"`
	CourseName             string                `json:"courseName"`
	CourseNameIncludeCross string                `json:"courseNameIncludeCrossOver"`
	ShItemPrices           []PricingItemDetails  `json:"shItemPrices"`
	ShItemPricesGroup      []PricingGroupDetails `json:"shItemPricesGroup"`
	HolesDisplay           string                `json:"holesDisplay"`
	PlayersDisplay         string                `json:"playersDisplay"`
	MinPlayer              int                   `json:"minPlayer"`
	MaxPlayer              int                   `json:"maxPlayer"`
	AvailableParticipantNo []int                 `json:"availableParticipantNo"`
	SummaryDetail          PricingSummary        `json:"summaryDetail"`
	PlayersRateCode        []interface{}         `json:"playersRateCode"`
	PlayerNames            []interface{}         `json:"playerNames"`
	BlockTexts             []interface{}         `json:"blockTexts"`
	DefaultClassCode       string                `json:"defaultClassCode"`
	TransactionID          string                `json:"transactionId"`
}

// PricingItemDetails represents detailed pricing for an item
type PricingItemDetails struct {
	ItemGuid                        string  `json:"itemGuid"`
	ParticipantNo                   int     `json:"participantNo"`
	ShItemCode                      string  `json:"shItemCode"`
	ItemCode                        string  `json:"itemCode"`
	Price                           float64 `json:"price"`
	TaxInclusivePrice               float64 `json:"taxInclusivePrice"`
	TaxCode                         string  `json:"taxCode"`
	ItemDesc                        string  `json:"itemDesc"`
	ClassCode                       string  `json:"classCode"`
	RateCode                        string  `json:"rateCode"`
	CurrentPrice                    float64 `json:"currentPrice"`
	PriceBeforeDiscount             float64 `json:"priceBeforeDiscount"`
	TaxInclusivePriceBeforeDiscount float64 `json:"taxInclusivePriceBeforeDiscount"`
	PriceType                       int     `json:"priceType"`
	PriceTypeName                   string  `json:"priceTypeName"`
	ExtendedPrice                   float64 `json:"extendedPrice"`
}

// PricingGroupDetails represents grouped pricing details
type PricingGroupDetails struct {
	Qty                             int      `json:"qty"`
	ItemCode                        string   `json:"itemCode"`
	ItemDesc                        string   `json:"itemDesc"`
	Price                           float64  `json:"price"`
	TaxInclusivePrice               float64  `json:"taxInclusivePrice"`
	TaxCode                         string   `json:"taxCode"`
	ExtendedPrice                   float64  `json:"extendedPrice"`
	RateCodes                       []string `json:"rateCodes"`
	ClassCodes                      []string `json:"classCodes"`
	PriceBeforeDiscount             float64  `json:"priceBeforeDiscount"`
	TaxInclusivePriceBeforeDiscount float64  `json:"taxInclusivePriceBeforeDiscount"`
	PriceTypes                      []int    `json:"priceTypes"`
	PriceTypeNames                  []string `json:"priceTypeNames"`
	StoreIDs                        []int    `json:"storeIds"`
}

// PricingSummary contains pricing totals
type PricingSummary struct {
	SubTotal         float64 `json:"subTotal"`
	Total            float64 `json:"total"`
	TotalDueAtCourse float64 `json:"totalDueAtCourse"`
}

// ReserveTeeTimeRequest is the request body for reserving a tee time
type ReserveTeeTimeRequest struct {
	CancelReservationLink   string            `json:"cancelReservationLink"`
	HomePageLink            string            `json:"homePageLink"`
	AffiliateID             interface{}       `json:"affiliateId"`
	FinalizeSaleModel       FinalizeSaleModel `json:"finalizeSaleModel"`
	SessionGUID             interface{}       `json:"sessionGuid"`
	LockedTeeTimesSessionID string            `json:"lockedTeeTimesSessionId"`
	TransactionID           string            `json:"transactionId"`
}

// FinalizeSaleModel contains payment and customer information
type FinalizeSaleModel struct {
	Acct           string         `json:"acct"`
	PlayerID       int            `json:"playerId"`
	IsGuest        bool           `json:"isGuest"`
	CreditCardInfo CreditCardInfo `json:"creditCardInfo"`
	MonerisCC      interface{}    `json:"monerisCC"`
	IBXCC          interface{}    `json:"ibxCC"`
}

// CreditCardInfo contains credit card details (all null for pay-at-course)
type CreditCardInfo struct {
	CardNumber interface{} `json:"cardNumber"`
	CardHolder interface{} `json:"cardHolder"`
	ExpireMM   interface{} `json:"expireMM"`
	ExpireYY   interface{} `json:"expireYY"`
	CVV        interface{} `json:"cvv"`
	Email      string      `json:"email"`
	CardToken  interface{} `json:"cardToken"`
}

// ReservationResponse is the response from reserve tee time API
type ReservationResponse struct {
	ReservationID     int    `json:"reservationId"`
	BookingIDs        []int  `json:"bookingIds"`
	ConfirmationKey   string `json:"confirmationKey"`
	ReservationResult int    `json:"reservationResult"`
	BookingGolferID   int    `json:"bookingGolferId"`
}

// ParseStartTime parses the startTime string into a time.Time
func (t *TeeTimeSlot) ParseStartTime() (time.Time, error) {
	// Try RFC3339 format first
	parsedTime, err := time.Parse(time.RFC3339, t.StartTime)
	if err != nil {
		// Try without timezone
		parsedTime, err = time.Parse("2006-01-02T15:04:05", t.StartTime)
	}
	return parsedTime, err
}

// IsWithinTimeRange checks if the tee time is within the specified range
func (t *TeeTimeSlot) IsWithinTimeRange(startTime, endTime *string) (bool, error) {
	if startTime == nil && endTime == nil {
		return true, nil // No time filter
	}

	teeTime, err := t.ParseStartTime()
	if err != nil {
		return false, err
	}

	if startTime != nil {
		start, err := time.Parse("2006-01-02T15:04:05", *startTime)
		if err != nil {
			return false, err
		}
		if teeTime.Before(start) {
			return false, nil
		}
	}

	if endTime != nil {
		end, err := time.Parse("2006-01-02T15:04:05", *endTime)
		if err != nil {
			return false, err
		}
		if teeTime.After(end) {
			return false, nil
		}
	}

	return true, nil
}
