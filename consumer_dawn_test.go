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
	// We need to verify that if max current > setpoint, we reduce immediately
	service := &dawnConsumerService{
		currentAmps:  16.0,
		minimumAmps:  6.0,
		maximumAmps:  16.0,
		setpoint:     20.0,
		notifyDevice: "test_device",
		currents:     make(map[string]float64),
		pid: &PIDController{
			Setpoint: 20.0,
		},
	}

	// Case: 22A on phase 1. Overage = 2A.
	service.currents["p1"] = 22.0
	
	// We'll mock the haService later or just check the internal state change if we refactor setAmps
	// For now, let's verify calculateAndSetAmps logic if we can.
}

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 10.5, parseFloat("10.5"))
	assert.Equal(t, 0.0, parseFloat("invalid"))
	assert.Equal(t, 10.0, parseFloat(10))
}
