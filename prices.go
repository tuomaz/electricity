package main

import (
	"errors"
	"log"
	"time"

	"github.com/tuomaz/nordpool"
)

type PriceService struct {
	area         string
	today        *nordpool.NordpoolData
	tomorrow     *nordpool.NordpoolData
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

func (ps *PriceService) getPriceForTime(data *nordpool.NordpoolData, t time.Time) (float64, error) {
	if data == nil || len(data.Data.Rows) == 0 {
		return 0, errors.New("no data")
	}

	format := "2006-01-02T15:04:05"

	for _, row := range data.Data.Rows {
		if row.IsExtraRow {
			continue
		}

		startTime, err := time.Parse(format, row.StartTime)
		if err != nil {
			continue
		}

		endTime, err := time.Parse(format, row.EndTime)
		if err != nil {
			// If EndTime is missing, assume 1 hour duration as fallback
			endTime = startTime.Add(time.Hour)
		}

		// Check if t is within [startTime, endTime)
		if (t.After(startTime) || t.Equal(startTime)) && t.Before(endTime) {
			for _, col := range row.Columns {
				if col.Name == ps.area {
					return parseFloat(col.Value), nil
				}
			}
		}
	}

	return 0, errors.New("not found in this dataset")
}

func (ps *PriceService) updatePrices() (bool, error) {
	updated := false
	nordpoolData, err := nordpool.GetNordpoolData()
	if err != nil {
		return false, errors.New("could not fetch data from Nordpool: " + err.Error())
	}

	format := "2006-01-02T15:04:05"

	if ps.today != nil && len(ps.today.Data.Rows) > 0 {
		if len(nordpoolData.Data.Rows) == 0 {
			return false, errors.New("received empty data from Nordpool")
		}
		tsOld, err := time.Parse(format, ps.today.Data.Rows[0].StartTime)
		if err != nil {
			log.Fatal(err)
		}

		tsNew, err := time.Parse(format, nordpoolData.Data.Rows[0].StartTime)
		if err != nil {
			log.Fatal(err)
		}
		//log.Printf("date diff: %v", tsNew.Sub(tsOld))

		updated = tsNew.Sub(tsOld) > 10000

		if updated {
			ps.today = ps.tomorrow
			ps.tomorrow = nordpoolData
		}

	} else {
		if len(nordpoolData.Data.Rows) > 0 {
			ps.today = nordpoolData
		} else {
			return false, errors.New("received empty data from Nordpool")
		}
	}

	return updated, nil
}
