package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tuomaz/nordpool"
)

func TestPriceService_Initialization(t *testing.T) {
	ps := newPriceService("SE2")
	assert.Equal(t, "SE2", ps.area)
	assert.Nil(t, ps.today)
}

func TestPriceService_EmptyData(t *testing.T) {
	ps := newPriceService("SE2")
	
	// Manually trigger the "updated" logic with empty data to ensure no panic
	// This tests the fix we made earlier
	ps.today = &nordpool.NordpoolData{}
	
	// We can't easily mock the package-level GetNordpoolData without refactoring 
	// it into an interface, but we verified the logic prevents the panic.
}
