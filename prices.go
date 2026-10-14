package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/tuomaz/nordpool"
)

type PriceService struct {
	area         string
	today        *nordpool.DayAheadData
	tomorrow     *nordpool.DayAheadData
	eventChannel chan event
}

func newPriceService(area string) *PriceService {
	priceService := &PriceService{area: area}
	return priceService
}

func (ps *PriceService) GetCurrentPrice() (float64, error) {
	now := time.Now()

	price, err := ps.getPriceForTime(ps.today, now)
	if err == nil {
		return price, nil
	}

	price, err = ps.getPriceForTime(ps.tomorrow, now)
	if err == nil {
		return price, nil
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
	todayData, err := nordpool.GetDayAheadPrices(now, "SEK", ps.area)
	if err != nil {
		return false, fmt.Errorf("could not fetch today's data from Nordpool: %w", err)
	}

	updated := false
	if ps.today == nil || len(ps.today.MultiAreaEntries) == 0 {
		ps.today = todayData
		updated = true
	} else if len(todayData.MultiAreaEntries) > 0 && len(ps.today.MultiAreaEntries) > 0 {
		if !todayData.MultiAreaEntries[0].DeliveryStart.Equal(ps.today.MultiAreaEntries[0].DeliveryStart) {
			ps.today = todayData
			updated = true
		}
	}

	// Also fetch tomorrow's data if available (typically published in the afternoon)
	tomorrow := now.Add(24 * time.Hour)
	tomorrowData, err := nordpool.GetDayAheadPrices(tomorrow, "SEK", ps.area)
	if err == nil && len(tomorrowData.MultiAreaEntries) > 0 {
		ps.tomorrow = tomorrowData
	}

	return updated, nil
}
