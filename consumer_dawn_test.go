package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mocking HA service is a bit complex due to its structure, 
// let's create a minimal testable version of the logic.

func TestDawnConsumer_MaxCurrent(t *testing.T) {
	service := &dawnConsumerService{
		currents: make(map[string]float64),
	}

	service.currents["p1"] = 10.5
	service.currents["p2"] = 15.2
	service.currents["p3"] = 8.0

	assert.Equal(t, 15.2, service.getMaxCurrent())
}

func TestDawnConsumer_SafetyOverride(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:    true,
		currentAmps:   16.0,
		actualAmps:    16.0,
		minimumAmps:   6.0,
		maximumAmps:   16.0,
		setpoint:      20.0,
		currents:      make(map[string]float64),
		exports:       make(map[string]float64),
		haService:     &haService{},
		pid: &PIDController{
			Setpoint: 20.0,
		},
	}

	// 1. Case: 23A on phase 1 (3A over setpoint). Should reduce immediately.
	service.currents["phase1"] = 23.0
	service.calculateAndSetAmps()
	
	// Reduction = ceil(23 - 20) = 3A. 16 - 3 = 13A.
	assert.Equal(t, 13.0, service.currentAmps, "Should reduce current immediately on overcurrent")
}

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 10.5, parseFloat("10.5"))
	assert.Equal(t, 0.0, parseFloat("invalid"))
	assert.Equal(t, 10.0, parseFloat(10))
}
