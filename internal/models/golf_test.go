package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTeeTimeSlot_ParseStartTime_RFC3339(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T08:30:00-04:00",
	}

	parsedTime, err := slot.ParseStartTime()
	if err != nil {
		t.Errorf("ParseStartTime() error = %v, want nil", err)
	}

	expected := time.Date(2025, 10, 29, 8, 30, 0, 0, time.FixedZone("EDT", -4*3600))
	if !parsedTime.Equal(expected) {
		t.Errorf("ParseStartTime() = %v, want %v", parsedTime, expected)
	}
}

func TestTeeTimeSlot_ParseStartTime_WithoutTimezone(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T08:30:00",
	}

	parsedTime, err := slot.ParseStartTime()
	if err != nil {
		t.Errorf("ParseStartTime() error = %v, want nil", err)
	}

	// Should parse successfully without timezone
	expectedTime := time.Date(2025, 10, 29, 8, 30, 0, 0, time.UTC)
	if parsedTime.Year() != expectedTime.Year() ||
		parsedTime.Month() != expectedTime.Month() ||
		parsedTime.Day() != expectedTime.Day() ||
		parsedTime.Hour() != expectedTime.Hour() ||
		parsedTime.Minute() != expectedTime.Minute() {
		t.Errorf("ParseStartTime() = %v, want similar to %v", parsedTime, expectedTime)
	}
}

func TestTeeTimeSlot_ParseStartTime_InvalidFormat(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
	}{
		{"empty string", ""},
		{"invalid format", "October 29, 2025 8:30 AM"},
		{"partial date", "2025-10-29"},
		{"malformed", "not-a-date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot := &TeeTimeSlot{
				StartTime: tt.startTime,
			}

			_, err := slot.ParseStartTime()
			if err == nil {
				t.Errorf("ParseStartTime() expected error for invalid format %q, got nil", tt.startTime)
			}
		})
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_NoFilter(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T10:00:00",
	}

	// No time filter - should always return true
	within, err := slot.IsWithinTimeRange(nil, nil)
	if err != nil {
		t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
	}
	if !within {
		t.Error("IsWithinTimeRange() = false, want true when no filter is applied")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_StartTimeOnly(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T10:00:00",
	}

	startTime := "2025-10-29T08:00:00"

	// Tee time at 10:00 is after 8:00 start - should be within range
	within, err := slot.IsWithinTimeRange(&startTime, nil)
	if err != nil {
		t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
	}
	if !within {
		t.Error("IsWithinTimeRange() = false, want true for time after start")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_BeforeStartTime(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T07:00:00",
	}

	startTime := "2025-10-29T08:00:00"

	// Tee time at 7:00 is before 8:00 start - should be outside range
	within, err := slot.IsWithinTimeRange(&startTime, nil)
	if err != nil {
		t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
	}
	if within {
		t.Error("IsWithinTimeRange() = true, want false for time before start")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_EndTimeOnly(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T10:00:00",
	}

	endTime := "2025-10-29T12:00:00"

	// Tee time at 10:00 is before 12:00 end - should be within range
	within, err := slot.IsWithinTimeRange(nil, &endTime)
	if err != nil {
		t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
	}
	if !within {
		t.Error("IsWithinTimeRange() = false, want true for time before end")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_AfterEndTime(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T14:00:00",
	}

	endTime := "2025-10-29T12:00:00"

	// Tee time at 14:00 is after 12:00 end - should be outside range
	within, err := slot.IsWithinTimeRange(nil, &endTime)
	if err != nil {
		t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
	}
	if within {
		t.Error("IsWithinTimeRange() = true, want false for time after end")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_BothStartAndEnd(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		teeTime   string
		endTime   string
		want      bool
	}{
		{
			name:      "within range",
			teeTime:   "2025-10-29T10:00:00",
			startTime: "2025-10-29T08:00:00",
			endTime:   "2025-10-29T12:00:00",
			want:      true,
		},
		{
			name:      "at start boundary",
			teeTime:   "2025-10-29T08:00:00",
			startTime: "2025-10-29T08:00:00",
			endTime:   "2025-10-29T12:00:00",
			want:      true,
		},
		{
			name:      "at end boundary",
			teeTime:   "2025-10-29T12:00:00",
			startTime: "2025-10-29T08:00:00",
			endTime:   "2025-10-29T12:00:00",
			want:      true,
		},
		{
			name:      "before range",
			teeTime:   "2025-10-29T07:00:00",
			startTime: "2025-10-29T08:00:00",
			endTime:   "2025-10-29T12:00:00",
			want:      false,
		},
		{
			name:      "after range",
			teeTime:   "2025-10-29T13:00:00",
			startTime: "2025-10-29T08:00:00",
			endTime:   "2025-10-29T12:00:00",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot := &TeeTimeSlot{
				StartTime: tt.teeTime,
			}

			within, err := slot.IsWithinTimeRange(&tt.startTime, &tt.endTime)
			if err != nil {
				t.Errorf("IsWithinTimeRange() error = %v, want nil", err)
			}
			if within != tt.want {
				t.Errorf("IsWithinTimeRange() = %v, want %v", within, tt.want)
			}
		})
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_InvalidStartTime(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T10:00:00",
	}

	invalidStart := "invalid-date"

	_, err := slot.IsWithinTimeRange(&invalidStart, nil)
	if err == nil {
		t.Error("IsWithinTimeRange() expected error for invalid start time, got nil")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_InvalidEndTime(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "2025-10-29T10:00:00",
	}

	invalidEnd := "invalid-date"

	_, err := slot.IsWithinTimeRange(nil, &invalidEnd)
	if err == nil {
		t.Error("IsWithinTimeRange() expected error for invalid end time, got nil")
	}
}

func TestTeeTimeSlot_IsWithinTimeRange_InvalidTeeTime(t *testing.T) {
	slot := &TeeTimeSlot{
		StartTime: "invalid-date",
	}

	startTime := "2025-10-29T08:00:00"

	_, err := slot.IsWithinTimeRange(&startTime, nil)
	if err == nil {
		t.Error("IsWithinTimeRange() expected error for invalid tee time, got nil")
	}
}

func TestSearchTeeTimesParams_JSONMarshaling(t *testing.T) {
	startTime := "2025-10-29T07:30:00"
	endTime := "2025-10-29T09:00:00"

	params := SearchTeeTimesParams{
		SearchDate:      "Wed Oct 29 2025",
		NumberOfPlayer:  2,
		StartSearchTime: &startTime,
		EndSearchTime:   &endTime,
		AutoBook:        true,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(params)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled SearchTeeTimesParams
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if unmarshaled.SearchDate != params.SearchDate {
		t.Errorf("SearchDate = %v, want %v", unmarshaled.SearchDate, params.SearchDate)
	}
	if unmarshaled.NumberOfPlayer != params.NumberOfPlayer {
		t.Errorf("NumberOfPlayer = %v, want %v", unmarshaled.NumberOfPlayer, params.NumberOfPlayer)
	}
	if *unmarshaled.StartSearchTime != *params.StartSearchTime {
		t.Errorf("StartSearchTime = %v, want %v", *unmarshaled.StartSearchTime, *params.StartSearchTime)
	}
	if *unmarshaled.EndSearchTime != *params.EndSearchTime {
		t.Errorf("EndSearchTime = %v, want %v", *unmarshaled.EndSearchTime, *params.EndSearchTime)
	}
	if unmarshaled.AutoBook != params.AutoBook {
		t.Errorf("AutoBook = %v, want %v", unmarshaled.AutoBook, params.AutoBook)
	}
}

func TestBookTeeTimeParams_JSONMarshaling(t *testing.T) {
	params := BookTeeTimeParams{
		TeeSheetID:     12345,
		NumberOfPlayer: 3,
		SearchDate:     "Wed Oct 29 2025",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(params)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled BookTeeTimeParams
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if unmarshaled.TeeSheetID != params.TeeSheetID {
		t.Errorf("TeeSheetID = %v, want %v", unmarshaled.TeeSheetID, params.TeeSheetID)
	}
	if unmarshaled.NumberOfPlayer != params.NumberOfPlayer {
		t.Errorf("NumberOfPlayer = %v, want %v", unmarshaled.NumberOfPlayer, params.NumberOfPlayer)
	}
	if unmarshaled.SearchDate != params.SearchDate {
		t.Errorf("SearchDate = %v, want %v", unmarshaled.SearchDate, params.SearchDate)
	}
}

func TestLockTeeTimeRequest_JSONMarshaling(t *testing.T) {
	req := LockTeeTimeRequest{
		TeeSheetIDs:    []int{12345, 67890},
		Email:          "test@example.com",
		Action:         "Online Reservation V5",
		SessionID:      "test-session-123",
		GolferID:       9999,
		ClassCode:      "R",
		NumberOfPlayer: 2,
		NavigateURL:    "",
		IsGroupBooking: false,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled LockTeeTimeRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if len(unmarshaled.TeeSheetIDs) != len(req.TeeSheetIDs) {
		t.Errorf("TeeSheetIDs length = %v, want %v", len(unmarshaled.TeeSheetIDs), len(req.TeeSheetIDs))
	}
	if unmarshaled.Email != req.Email {
		t.Errorf("Email = %v, want %v", unmarshaled.Email, req.Email)
	}
	if unmarshaled.GolferID != req.GolferID {
		t.Errorf("GolferID = %v, want %v", unmarshaled.GolferID, req.GolferID)
	}
}

func TestLockTeeTimeResponse_JSONMarshaling(t *testing.T) {
	resp := LockTeeTimeResponse{
		TeeSheetIDs: []int{12345},
		SessionID:   "session-123",
		Error:       "",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled LockTeeTimeResponse
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if len(unmarshaled.TeeSheetIDs) != len(resp.TeeSheetIDs) {
		t.Errorf("TeeSheetIDs length = %v, want %v", len(unmarshaled.TeeSheetIDs), len(resp.TeeSheetIDs))
	}
	if unmarshaled.SessionID != resp.SessionID {
		t.Errorf("SessionID = %v, want %v", unmarshaled.SessionID, resp.SessionID)
	}
}

func TestPricingCalculationRequest_JSONMarshaling(t *testing.T) {
	req := PricingCalculationRequest{
		SelectedTeeSheetID: 12345,
		BookingList: []PricingBookingItem{
			{
				TeeSheetID:           12345,
				Holes:                18,
				ParticipantNo:        1,
				GolferID:             9999,
				RateCode:             "N",
				IsUnassignedPlayer:   false,
				MemberClassCode:      "R",
				MemberStoreID:        "1",
				CartType:             1,
				PlayerID:             "0",
				Acct:                 "test-acct",
				IsGuestOf:            false,
				IsUseCapacityPricing: false,
			},
		},
		Holes:                18,
		NumberOfPlayer:       1,
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

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled PricingCalculationRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify key fields
	if unmarshaled.SelectedTeeSheetID != req.SelectedTeeSheetID {
		t.Errorf("SelectedTeeSheetID = %v, want %v", unmarshaled.SelectedTeeSheetID, req.SelectedTeeSheetID)
	}
	if len(unmarshaled.BookingList) != len(req.BookingList) {
		t.Errorf("BookingList length = %v, want %v", len(unmarshaled.BookingList), len(req.BookingList))
	}
}

func TestReserveTeeTimeRequest_JSONMarshaling(t *testing.T) {
	req := ReserveTeeTimeRequest{
		CancelReservationLink: "https://example.com/cancel",
		HomePageLink:          "https://example.com/",
		AffiliateID:           nil,
		FinalizeSaleModel: FinalizeSaleModel{
			Acct:     "test-acct",
			PlayerID: 0,
			IsGuest:  false,
			CreditCardInfo: CreditCardInfo{
				CardNumber: nil,
				CardHolder: nil,
				ExpireMM:   nil,
				ExpireYY:   nil,
				CVV:        nil,
				Email:      "test@example.com",
				CardToken:  nil,
			},
			MonerisCC: nil,
			IBXCC:     nil,
		},
		SessionGUID:             nil,
		LockedTeeTimesSessionID: "session-123",
		TransactionID:           "transaction-456",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled ReserveTeeTimeRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if unmarshaled.LockedTeeTimesSessionID != req.LockedTeeTimesSessionID {
		t.Errorf("LockedTeeTimesSessionID = %v, want %v", unmarshaled.LockedTeeTimesSessionID, req.LockedTeeTimesSessionID)
	}
	if unmarshaled.TransactionID != req.TransactionID {
		t.Errorf("TransactionID = %v, want %v", unmarshaled.TransactionID, req.TransactionID)
	}
	if unmarshaled.FinalizeSaleModel.Acct != req.FinalizeSaleModel.Acct {
		t.Errorf("Acct = %v, want %v", unmarshaled.FinalizeSaleModel.Acct, req.FinalizeSaleModel.Acct)
	}
	if unmarshaled.FinalizeSaleModel.CreditCardInfo.Email != req.FinalizeSaleModel.CreditCardInfo.Email {
		t.Errorf("Email = %v, want %v", unmarshaled.FinalizeSaleModel.CreditCardInfo.Email, req.FinalizeSaleModel.CreditCardInfo.Email)
	}
}

func TestReservationResponse_JSONMarshaling(t *testing.T) {
	resp := ReservationResponse{
		ReservationID:     789,
		BookingIDs:        []int{111, 222},
		ConfirmationKey:   "CONF-123",
		ReservationResult: 1,
		BookingGolferID:   9999,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled ReservationResponse
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if unmarshaled.ReservationID != resp.ReservationID {
		t.Errorf("ReservationID = %v, want %v", unmarshaled.ReservationID, resp.ReservationID)
	}
	if unmarshaled.ConfirmationKey != resp.ConfirmationKey {
		t.Errorf("ConfirmationKey = %v, want %v", unmarshaled.ConfirmationKey, resp.ConfirmationKey)
	}
	if unmarshaled.ReservationResult != resp.ReservationResult {
		t.Errorf("ReservationResult = %v, want %v", unmarshaled.ReservationResult, resp.ReservationResult)
	}
}

func TestTeeTimeSlot_JSONMarshaling(t *testing.T) {
	slot := TeeTimeSlot{
		TeeSheetID:    12345,
		StartTime:     "2025-10-29T08:30:00",
		CourseTimeID:  1,
		StartingTee:   1,
		Participants:  4,
		CourseID:      100,
		CourseDate:    "2025-10-29",
		Holes:         18,
		CourseName:    "Test Course",
		MinPlayer:     1,
		MaxPlayer:     4,
		ShItemPrices: []TeeTimePrice{
			{
				ItemGuid:          "guid-123",
				ShItemCode:        "GreenFee18",
				ItemCode:          "GF18",
				Price:             50.00,
				TaxInclusivePrice: 56.50,
				TaxCode:           "TAX1",
				ItemDesc:          "18 Hole Green Fee",
				ClassCode:         "R",
				RateCode:          "N",
				CurrentPrice:      50.00,
				PriceType:         1,
				PriceTypeName:     "Regular",
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(slot)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled TeeTimeSlot
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify key fields
	if unmarshaled.TeeSheetID != slot.TeeSheetID {
		t.Errorf("TeeSheetID = %v, want %v", unmarshaled.TeeSheetID, slot.TeeSheetID)
	}
	if unmarshaled.StartTime != slot.StartTime {
		t.Errorf("StartTime = %v, want %v", unmarshaled.StartTime, slot.StartTime)
	}
	if unmarshaled.CourseName != slot.CourseName {
		t.Errorf("CourseName = %v, want %v", unmarshaled.CourseName, slot.CourseName)
	}
	if len(unmarshaled.ShItemPrices) != len(slot.ShItemPrices) {
		t.Errorf("ShItemPrices length = %v, want %v", len(unmarshaled.ShItemPrices), len(slot.ShItemPrices))
	}
	if len(unmarshaled.ShItemPrices) > 0 {
		if unmarshaled.ShItemPrices[0].Price != slot.ShItemPrices[0].Price {
			t.Errorf("Price = %v, want %v", unmarshaled.ShItemPrices[0].Price, slot.ShItemPrices[0].Price)
		}
	}
}

func TestPricingCalculationResponse_JSONMarshaling(t *testing.T) {
	resp := PricingCalculationResponse{
		TeeSheetID:    12345,
		StartTime:     "2025-10-29T08:30:00",
		CourseName:    "Test Course",
		Holes:         18,
		TransactionID: "txn-123",
		SummaryDetail: PricingSummary{
			SubTotal:         50.00,
			Total:            56.50,
			TotalDueAtCourse: 56.50,
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(resp)
	if err != nil {
		t.Errorf("json.Marshal() error = %v, want nil", err)
	}

	// Unmarshal back
	var unmarshaled PricingCalculationResponse
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Errorf("json.Unmarshal() error = %v, want nil", err)
	}

	// Verify fields
	if unmarshaled.TeeSheetID != resp.TeeSheetID {
		t.Errorf("TeeSheetID = %v, want %v", unmarshaled.TeeSheetID, resp.TeeSheetID)
	}
	if unmarshaled.TransactionID != resp.TransactionID {
		t.Errorf("TransactionID = %v, want %v", unmarshaled.TransactionID, resp.TransactionID)
	}
	if unmarshaled.SummaryDetail.Total != resp.SummaryDetail.Total {
		t.Errorf("Total = %v, want %v", unmarshaled.SummaryDetail.Total, resp.SummaryDetail.Total)
	}
}
