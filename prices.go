package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tuomaz/nordpool"
)

type PriceService struct {
	area         string
	todayDate    string
	tomorrowDate string
	today        *nordpool.DayAheadData
	tomorrow     *nordpool.DayAheadData
	eventChannel chan event
}

func newPriceService(area string) *PriceService {
	priceService := &PriceService{area: area}
	return priceService
}

// GetCurrentPrice returns the current spot price in öre/kWh.
// Nordpool provides prices in SEK/MWh.
// Dividing by 10.0 converts SEK/MWh to öre/kWh (1 MWh = 1000 kWh, 1 SEK = 100 öre).
func (ps *PriceService) GetCurrentPrice() (float64, error) {
	now := time.Now()

	price, err := ps.getPriceForTime(ps.today, now)
	if err == nil {
		return price / 10.0, nil
	}

	price, err = ps.getPriceForTime(ps.tomorrow, now)
	if err == nil {
		return price / 10.0, nil
	}

	return 0, errors.New("no price found for current time")
}

func (ps *PriceService) getPriceForTime(data *nordpool.DayAheadData, t time.Time) (float64, error) {
	if data == nil || len(data.MultiAreaEntries) == 0 {
		return 0, errors.New("no data")
	}

	return data.GetPriceForTime(t, ps.area)
}

func (ps *PriceService) updatePrices() (bool, error) {
	now := time.Now()
	todayStr := now.Format("2006-01-02")
	tomorrowStr := now.Add(24 * time.Hour).Format("2006-01-02")
	updated := false

	// 1. Rollover tomorrow -> today at midnight
	if ps.tomorrowDate == todayStr && ps.tomorrow != nil {
		ps.today = ps.tomorrow
		ps.todayDate = ps.tomorrowDate
		ps.tomorrow = nil
		ps.tomorrowDate = ""
		updated = true
		log.Printf("PRICES: Rolled over cached data for today (%s)", todayStr)
	}

	// 2. Fetch today's 24h prices if not already cached
	if ps.today == nil || ps.todayDate != todayStr {
		todayData, err := nordpool.GetDayAheadPrices(now, "SEK", ps.area)
		if err != nil {
			return false, fmt.Errorf("could not fetch today's data from Nordpool: %w", err)
		}
		ps.today = todayData
		ps.todayDate = todayStr
		updated = true
		log.Printf("PRICES: Successfully fetched 24h prices for today (%s)", todayStr)
	}

	// 3. Fetch tomorrow's 24h prices if not already cached and afternoon has arrived (>= 13:00)
	if (ps.tomorrow == nil || ps.tomorrowDate != tomorrowStr) && now.Hour() >= 13 {
		tomorrowTime := now.Add(24 * time.Hour)
		tomorrowData, err := nordpool.GetDayAheadPrices(tomorrowTime, "SEK", ps.area)
		if err == nil && len(tomorrowData.MultiAreaEntries) > 0 {
			ps.tomorrow = tomorrowData
			ps.tomorrowDate = tomorrowStr
			log.Printf("PRICES: Successfully fetched 24h prices for tomorrow (%s)", tomorrowStr)
		}
	}

	return updated, nil
}
