package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tuomaz/gohaws"
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
	service.updateCurrents(&powerEvent{sensorType: SensorTypeExport, phaseIndex: 1, value: 7.0})
	service.updateCurrents(&powerEvent{sensorType: SensorTypeExport, phaseIndex: 2, value: 7.0})
	service.updateCurrents(&powerEvent{sensorType: SensorTypeExport, phaseIndex: 3, value: 7.0})
	
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
		setpoint:        20.0,
		exports:         map[string]float64{"phase1": 0, "phase2": 0, "phase3": 0},
		currents:        map[string]float64{"phase1": 0, "phase2": 0, "phase3": 0},
		haService:       &haService{},
		connectorStatus: "charging",
		pid:             &PIDController{},
	}

	// 1. Grid import detected (4.0A on phase 1 -> Net Export -4.0)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 4.0})
	
	assert.True(t, service.isCharging, "Should not stop immediately")
	assert.NotNil(t, service.pvShortageStartTime)

	// 2. Fast forward time (6 minutes later)
	service.pvShortageStartTime = time.Now().Add(-6 * time.Minute)
	service.calculateAndSetAmps()
	assert.False(t, service.isCharging, "Should stop after 5 minutes of sustained grid import")
}

func TestDawnConsumer_PVOnlySwitchTrigger(t *testing.T) {
	haSubChan := make(chan *gohaws.Message, 10)
	service := &dawnConsumerService{
		isCharging:      false,
		pvOnlyMode:      false,
		minimumAmps:     6.0,
		maximumAmps:     16.0,
		setpoint:        20.0,
		exports:         map[string]float64{"phase1": 7, "phase2": 7, "phase3": 7},
		currents:        map[string]float64{"phase1": 0, "phase2": 0, "phase3": 0},
		haService:       &haService{},
		haChannel:       haSubChan,
		pvOnlySwitchId:  "switch.pv_only",
		pid:             &PIDController{},
		connectorStatus: "connected",
	}

	// Start the runner in a goroutine so it can process the message
	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	defer cancel()
	go service.run()

	// 1. Simulate PV-Only switch turning ON
	haSubChan <- &gohaws.Message{
		Event: &gohaws.Event{
			Data: &gohaws.Data{
				EntityID: "switch.pv_only",
				NewState: &gohaws.State{State: "on"},
			},
		},
	}

	// Give it a moment to process
	time.Sleep(50 * time.Millisecond)

	// Check if PV surplus timer started
	service.mu.RLock()
	started := !service.pvSurplusStartTime.IsZero()
	mode := service.pvOnlyMode
	service.mu.RUnlock()

	assert.True(t, mode, "PV mode should be ON")
	assert.True(t, started, "PV surplus timer should have started immediately upon switch ON")
}
