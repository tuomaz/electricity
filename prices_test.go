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
	
	data := &nordpool.NordpoolData{}
	data.Data.Rows = []struct {
		Columns []struct {
			Index                            int    `json:"Index,omitempty"`
			Scale                            int    `json:"Scale,omitempty"`
			SecondaryValue                   any    `json:"SecondaryValue,omitempty"`
			IsDominatingDirection            bool   `json:"IsDominatingDirection,omitempty"`
			IsValid                          bool   `json:"IsValid,omitempty"`
			IsAdditionalData                 bool   `json:"IsAdditionalData,omitempty"`
			Behavior                         int    `json:"Behavior,omitempty"`
			Name                             string `json:"Name,omitempty"`
			Value                            string `json:"Value,omitempty"`
			GroupHeader                      string `json:"GroupHeader,omitempty"`
			DisplayNegativeValueInBlue       bool   `json:"DisplayNegativeValueInBlue,omitempty"`
			CombinedName                     string `json:"CombinedName,omitempty"`
			DateTimeForData                  string `json:"DateTimeForData,omitempty"`
			DisplayName                      string `json:"DisplayName,omitempty"`
			DisplayNameOrDominatingDirection string `json:"DisplayNameOrDominatingDirection,omitempty"`
			IsOfficial                       bool   `json:"IsOfficial,omitempty"`
			UseDashDisplayStyle              bool   `json:"UseDashDisplayStyle,omitempty"`
		} `json:"Columns,omitempty"`
		Name            string `json:"Name,omitempty"`
		StartTime       string `json:"StartTime,omitempty"`
		EndTime         string `json:"EndTime,omitempty"`
		DateTimeForData string `json:"DateTimeForData,omitempty"`
		DayNumber       int    `json:"DayNumber,omitempty"`
		StartTimeDate   string `json:"StartTimeDate,omitempty"`
		IsExtraRow      bool   `json:"IsExtraRow,omitempty"`
		IsNtcRow        bool   `json:"IsNtcRow,omitempty"`
		EmptyValue      string `json:"EmptyValue,omitempty"`
		Parent          any    `json:"Parent,omitempty"`
	}{
		{
			StartTime: "2026-04-02T10:00:00",
			EndTime:   "2026-04-02T11:00:00",
		},
		{
			StartTime: "2026-04-02T11:00:00",
			EndTime:   "2026-04-02T11:15:00",
		},
	}
	
	// We need to initialize the Columns slice for each row
	data.Data.Rows[0].Columns = []struct {
		Index                            int    `json:"Index,omitempty"`
		Scale                            int    `json:"Scale,omitempty"`
		SecondaryValue                   any    `json:"SecondaryValue,omitempty"`
		IsDominatingDirection            bool   `json:"IsDominatingDirection,omitempty"`
		IsValid                          bool   `json:"IsValid,omitempty"`
		IsAdditionalData                 bool   `json:"IsAdditionalData,omitempty"`
		Behavior                         int    `json:"Behavior,omitempty"`
		Name                             string `json:"Name,omitempty"`
		Value                            string `json:"Value,omitempty"`
		GroupHeader                      string `json:"GroupHeader,omitempty"`
		DisplayNegativeValueInBlue       bool   `json:"DisplayNegativeValueInBlue,omitempty"`
		CombinedName                     string `json:"CombinedName,omitempty"`
		DateTimeForData                  string `json:"DateTimeForData,omitempty"`
		DisplayName                      string `json:"DisplayName,omitempty"`
		DisplayNameOrDominatingDirection string `json:"DisplayNameOrDominatingDirection,omitempty"`
		IsOfficial                       bool   `json:"IsOfficial,omitempty"`
		UseDashDisplayStyle              bool   `json:"UseDashDisplayStyle,omitempty"`
	}{
		{Name: "SE2", Value: "50.5"},
	}
	
	data.Data.Rows[1].Columns = []struct {
		Index                            int    `json:"Index,omitempty"`
		Scale                            int    `json:"Scale,omitempty"`
		SecondaryValue                   any    `json:"SecondaryValue,omitempty"`
		IsDominatingDirection            bool   `json:"IsDominatingDirection,omitempty"`
		IsValid                          bool   `json:"IsValid,omitempty"`
		IsAdditionalData                 bool   `json:"IsAdditionalData,omitempty"`
		Behavior                         int    `json:"Behavior,omitempty"`
		Name                             string `json:"Name,omitempty"`
		Value                            string `json:"Value,omitempty"`
		GroupHeader                      string `json:"GroupHeader,omitempty"`
		DisplayNegativeValueInBlue       bool   `json:"DisplayNegativeValueInBlue,omitempty"`
		CombinedName                     string `json:"CombinedName,omitempty"`
		DateTimeForData                  string `json:"DateTimeForData,omitempty"`
		DisplayName                      string `json:"DisplayName,omitempty"`
		DisplayNameOrDominatingDirection string `json:"DisplayNameOrDominatingDirection,omitempty"`
		IsOfficial                       bool   `json:"IsOfficial,omitempty"`
		UseDashDisplayStyle              bool   `json:"UseDashDisplayStyle,omitempty"`
	}{
		{Name: "SE2", Value: "75.0"},
	}

	format := "2006-01-02T15:04:05"
	t1, _ := time.Parse(format, "2026-04-02T10:30:00")
	p1, err := ps.getPriceForTime(data, t1)
	assert.NoError(t, err)
	assert.Equal(t, 50.5, p1)

	t2, _ := time.Parse(format, "2026-04-02T11:05:00")
	p2, err := ps.getPriceForTime(data, t2)
	assert.NoError(t, err)
	assert.Equal(t, 75.0, p2)
}
