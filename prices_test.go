package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tuomaz/nordpool"
)

func TestPriceService_Initialization(t *testing.T) {
	ps := newPriceService("SE2")
	assert.Equal(t, "SE2", ps.area)
	assert.Nil(t, ps.today)
}

func TestPriceService_GetPriceForTime(t *testing.T) {
	ps := newPriceService("SE2")

	format := "2006-01-02T15:04:05Z"
	t1Start, _ := time.Parse(format, "2026-04-02T10:00:00Z")
	t1End, _ := time.Parse(format, "2026-04-02T11:00:00Z")
	t2Start, _ := time.Parse(format, "2026-04-02T11:00:00Z")
	t2End, _ := time.Parse(format, "2026-04-02T11:15:00Z")

	data := &nordpool.DayAheadData{
		Currency: "SEK",
		MultiAreaEntries: []nordpool.MultiAreaEntry{
			{
				DeliveryStart: t1Start,
				DeliveryEnd:   t1End,
				EntryPerArea: map[string]float64{
					"SE2": 50.5,
				},
			},
			{
				DeliveryStart: t2Start,
				DeliveryEnd:   t2End,
				EntryPerArea: map[string]float64{
					"SE2": 75.0,
				},
			},
		},
	}

	t1, _ := time.Parse(format, "2026-04-02T10:30:00Z")
	p1, err := ps.getPriceForTime(data, t1)
	assert.NoError(t, err)
	assert.Equal(t, 50.5, p1)

	t2, _ := time.Parse(format, "2026-04-02T11:05:00Z")
	p2, err := ps.getPriceForTime(data, t2)
	assert.NoError(t, err)
	assert.Equal(t, 75.0, p2)
}

func TestPriceService_LiveUpdatePrices(t *testing.T) {
	ps := newPriceService("SE2")
	updated, err := ps.updatePrices()
	assert.NoError(t, err, "Live updatePrices should succeed")
	assert.True(t, updated, "First updatePrices should mark updated as true")
	assert.NotNil(t, ps.today, "Today data should be populated")

	price, err := ps.GetCurrentPrice()
	assert.NoError(t, err, "GetCurrentPrice should return price for current hour")
	t.Logf("Live current electricity price for SE2: %.2f SEK/MWh", price)
	assert.True(t, price > 0, "Price should be greater than 0")
}
