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

func (ps *PriceService) updatePrices() (bool, error) {
	updated := false
	nordpoolData, err := nordpool.GetNordpoolData()
	if err != nil {
		return false, errors.New("could not fetch data from Nordpool: " + err.Error())
	}

	format := "2006-01-02T15:04:05"

	if ps.today != nil && len(ps.today.Data.Rows) > 0 {
		tsOld, err := time.Parse(format, ps.today.Data.Rows[0].StartTime)
		if err != nil {
			log.Fatal(err)
		}

		tsNew, err := time.Parse(format, nordpoolData.Data.Rows[0].StartTime)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("date diff: %v", tsNew.Sub(tsOld))

		updated = tsNew.Sub(tsOld) > 10000

		if updated {
			ps.today = ps.tomorrow
			ps.tomorrow = nordpoolData
		}

	} else {
		ps.today = nordpoolData
	}

	return updated, nil
}
