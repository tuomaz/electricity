package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDawnConsumer_MinExport(t *testing.T) {
	service := &dawnConsumerService{
		exports: make(map[string]float64),
	}

	// Case 1: All 3 phases exporting
	service.exports["phase1"] = 10.5
	service.exports["phase2"] = 15.2
	service.exports["phase3"] = 8.0
	assert.Equal(t, 8.0, service.getMinExport())

	// Case 2: Only 2 phases (missing one)
	delete(service.exports, "phase3")
	assert.Equal(t, 0.0, service.getMinExport(), "Should return 0 if any phase is not exporting")

	// Case 3: One phase is zero
	service.exports["phase1"] = 10.5
	service.exports["phase2"] = 15.2
	service.exports["phase3"] = 0.0
	assert.Equal(t, 0.0, service.getMinExport())
}

func TestDawnConsumer_PVStartCondition(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:   false,
		pvOnlyMode:   true,
		minimumAmps:  6.0,
		maximumAmps:  16.0,
		setpoint:     20.0,
		exports:      make(map[string]float64),
		currents:     make(map[string]float64),
		haService:    &haService{}, // Minimal service (client=nil)
		pid:          &PIDController{},
	}

	// 1. Surplus detected (7A per phase)
	service.exports["phase1"] = 7.0
	service.exports["phase2"] = 7.0
	service.exports["phase3"] = 7.0
	
	service.calculateAndSetAmps()
	assert.False(t, service.isCharging, "Should not start immediately")
	assert.NotNil(t, service.pvSurplusStartTime)

	// 2. Fast forward time (6 minutes later)
	service.pvSurplusStartTime = time.Now().Add(-6 * time.Minute)
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging, "Should start after 5 minutes of sustained surplus")
}

func TestDawnConsumer_PVStopCondition(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:      true,
		pvOnlyMode:      true,
		minimumAmps:     6.0,
		currentAmps:     6.0,
		exports:         make(map[string]float64),
		currents:        make(map[string]float64),
		haService:       &haService{},
		connectorStatus: "charging",
		pid:             &PIDController{},
	}

	// 1. Grid import detected (0.5A on phase 1)
	service.currents["phase1"] = 0.5
	service.exports["phase1"] = 0
	service.exports["phase2"] = 0
	service.exports["phase3"] = 0
	
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging, "Should not stop immediately")
	assert.NotNil(t, service.pvShortageStartTime)

	// 2. Fast forward time (6 minutes later)
	service.pvShortageStartTime = time.Now().Add(-6 * time.Minute)
	service.calculateAndSetAmps()
	assert.False(t, service.isCharging, "Should stop after 5 minutes of sustained grid import")
}
