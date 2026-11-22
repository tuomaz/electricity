package main

import (
	"testing"
	"time"

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
		currents:           make(map[string]float64),
		hasDirectionalData: make(map[string]bool),
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

func TestDawnConsumer_SoftLimit(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:      true,
		currentAmps:     6.0,
		actualAmps:      6.0,
		minimumAmps:     6.0,
		maximumAmps:     16.0,
		userLimit:          8.0,
		setpoint:           20.0,
		currents:           make(map[string]float64),
		hasDirectionalData: make(map[string]bool),
		exports:            make(map[string]float64),
		haService:       &haService{},
		connectorStatus: "charging",
		lastExecution:   time.Now().Add(-1 * time.Minute),
		pid: &PIDController{
			Setpoint: 20.0,
			Kp:       2.0,
		},
	}

	// 1. Case: Plenty of headroom (10A on each phase). PID should want to increase to 16A.
	// But soft limit is 8A.
	service.currents["phase1"] = 10.0
	service.currents["phase2"] = 10.0
	service.currents["phase3"] = 10.0

	service.calculateAndSetAmps() // First call initializes PID LastTime
	time.Sleep(200 * time.Millisecond)
	service.lastExecution = time.Now().Add(-1 * time.Minute)
	service.calculateAndSetAmps() // Second call calculates adjustment

	assert.Equal(t, 8.0, service.currentAmps, "Should cap current at userLimit (8A) even with headroom")
}

func TestParseFloat(t *testing.T) {
	assert.Equal(t, 10.5, parseFloat("10.5"))
	assert.Equal(t, 0.0, parseFloat("invalid"))
	assert.Equal(t, 10.0, parseFloat(10))
}

func TestDawnConsumer_SafetyOverride_NoDoubleReductionOnStalePhase(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:         true,
		currentAmps:        16.0,
		actualAmps:         15.2,
		minimumAmps:        6.0,
		maximumAmps:        16.0,
		setpoint:           20.0,
		currents:           make(map[string]float64),
		hasDirectionalData: make(map[string]bool),
		phaseUpdatedAt:     make(map[string]time.Time),
		exports:            make(map[string]float64),
		haService:          &haService{},
		pid: &PIDController{
			Setpoint: 20.0,
		},
	}

	// 1. Phase 2 spikes to 24.33A -> should reduce 16A -> 11A (ceil(4.33) = 5A reduction from 15.2A/16A)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 2, value: 24.33})
	assert.Equal(t, 11, int(service.currentAmps), "First overcurrent should reduce 16A -> 11A")

	// 2. 10 seconds later (within 12s cooldown), Phase 1 reports a normal reading while Phase 2 is still stale at 24.33A
	service.lastHardSafetyEvent = time.Now().Add(-10 * time.Second)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 10.0})
	assert.Equal(t, 11, int(service.currentAmps), "Should NOT double-reduce on stale Phase 2 value before 12s cooldown and fresh Phase 2 reading")

	// 3. Even after 13 seconds, if Phase 1 updates before Phase 2 has reported a new post-reduction reading, do not re-trigger on stale Phase 2
	staleTime := time.Now().Add(-13 * time.Second)
	service.lastHardSafetyEvent = staleTime
	service.phaseUpdatedAt["phase2"] = staleTime.Add(-10 * time.Millisecond)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 10.5})
	assert.Equal(t, 11, int(service.currentAmps), "Should NOT double-reduce when Phase 2 reading is older than lastHardSafetyEvent")

	// 4. Once Phase 2 reports a genuinely new overcurrent reading (22.5A) after the 12s cooldown, it should reduce further (11A -> 8A)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 2, value: 22.5})
	assert.Equal(t, 8, int(service.currentAmps), "Should reduce once Phase 2 reports a fresh overcurrent reading after cooldown")
}

func TestDawnConsumer_SafetyOverride_StaleActualAmpsAt6ADoesNotSkipReduction(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:          true,
		currentAmps:         14.0,
		actualAmps:          6.0, // Stale 60s OCPP reading from when charger was at 6A
		actualAmpsUpdatedAt: time.Now().Add(-30 * time.Second),
		lastAmpsChange:      time.Now().Add(-10 * time.Second), // Charger was ramped to 14A 10s ago
		minimumAmps:         6.0,
		maximumAmps:         16.0,
		setpoint:            20.0,
		currents:            make(map[string]float64),
		hasDirectionalData:  make(map[string]bool),
		phaseUpdatedAt:      make(map[string]time.Time),
		exports:             make(map[string]float64),
		haService:           &haService{},
		pid: &PIDController{
			Setpoint: 20.0,
		},
	}

	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 23.86})
	assert.Equal(t, 10.0, service.currentAmps, "Should reduce from 14A -> 10A using currentAmps baseline when actualAmps is stale")
	assert.True(t, service.overcurrentStartTime.IsZero(), "Should not start emergency stop timer when currentAmps > minimumAmps")
}
