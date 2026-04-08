package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDawnConsumer_PriceLimitStart(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:        false,
		pvOnlyMode:        false,
		minimumAmps:       6.0,
		maximumAmps:       16.0,
		setpoint:          20.0,
		currentPriceLimit: 100.0,
		currentPrice:      150.0, // Over limit
		currents:          make(map[string]float64),
		exports:           make(map[string]float64),
		haService:         &haService{},
		pid:               &PIDController{},
	}

	// 1. Sufficient headroom but price is too high
	service.currents["phase1"] = 5.0
	service.calculateAndSetAmps()
	assert.False(t, service.isCharging, "Should not start charging if price is over limit")

	// 2. Price becomes OK
	service.currentPrice = 80.0
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging, "Should start charging when price is OK")
}

func TestDawnConsumer_PriceLimitStop(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:        true,
		pvOnlyMode:        false,
		minimumAmps:       6.0,
		currentAmps:       10.0,
		currentPriceLimit: 100.0,
		currentPrice:      80.0,
		currents:          make(map[string]float64),
		exports:           make(map[string]float64),
		haService:         &haService{},
		pid:               &PIDController{},
	}

	// 1. Price is OK, keep charging
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging)

	// 2. Price exceeds limit
	service.currentPrice = 120.0
	service.calculateAndSetAmps()
	assert.False(t, service.isCharging, "Should stop charging if price exceeds limit")
}

func TestDawnConsumer_PriceLimitIgnoredInPVMode(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:        true,
		pvOnlyMode:        true,
		minimumAmps:       6.0,
		currentAmps:       6.0,
		currentPriceLimit: 100.0,
		currentPrice:      150.0, // Over limit
		currents:          make(map[string]float64),
		exports:           make(map[string]float64),
		haService:         &haService{},
		connectorStatus:   "charging",
		pid:               &PIDController{},
	}

	// Price is over limit, but we are in PV-only mode
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging, "Should NOT stop charging if price exceeds limit while in PV-only mode")
}

func TestDawnConsumer_PriceLimitDisabledIfZero(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:        false,
		pvOnlyMode:        false,
		minimumAmps:       6.0,
		setpoint:          20.0,
		currentPriceLimit: 0.0,   // Disabled
		currentPrice:      150.0, // High price
		currents:          make(map[string]float64),
		exports:           make(map[string]float64),
		haService:         &haService{},
		pid:               &PIDController{},
	}

	service.currents["phase1"] = 5.0
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging, "Should start charging if price limit is 0, regardless of current price")
}
