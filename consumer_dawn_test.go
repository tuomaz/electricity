package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tuomaz/gohaws"
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

	// 2. 5 seconds later (within 8s cooldown), Phase 1 reports a normal reading while Phase 2 is still stale at 24.33A
	service.lastHardSafetyEvent = time.Now().Add(-5 * time.Second)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 10.0})
	assert.Equal(t, 11, int(service.currentAmps), "Should NOT double-reduce on stale Phase 2 value before 8s cooldown")

	// 3. At 10 seconds (next P1 batch), if Phase 1 updates before Phase 2 has reported a new post-reduction reading, do not re-trigger on stale Phase 2
	staleTime := time.Now().Add(-10 * time.Second)
	service.lastHardSafetyEvent = staleTime
	service.phaseUpdatedAt["phase2"] = staleTime.Add(-10 * time.Millisecond)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 10.5})
	assert.Equal(t, 11, int(service.currentAmps), "Should NOT double-reduce when Phase 2 reading is older than lastHardSafetyEvent")

	// 4. Once Phase 2 reports a genuinely new overcurrent reading (22.5A) in the 10s batch (>8s cooldown), it should reduce immediately (11A -> 8A)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 2, value: 22.5})
	assert.Equal(t, 8, int(service.currentAmps), "Should reduce once Phase 2 reports a fresh overcurrent reading after 8s cooldown")
}

func TestDawnConsumer_SafetyOverride_EmergencyStopRequiresFreshOvercurrentReading(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:         true,
		currentAmps:        6.0,
		actualAmps:         6.0,
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

	// 1. Phase 2 spikes at minimum charging (6A) -> starts 10s shutdown timer
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 2, value: 23.0})
	assert.False(t, service.overcurrentStartTime.IsZero(), "Should start 10s emergency shutdown timer")
	assert.True(t, service.isCharging)

	// 2. 10.1s later, Phase 1 reports a normal reading (5A) just before Phase 2's new reading arrives
	t0 := time.Now().Add(-10100 * time.Millisecond)
	service.overcurrentStartTime = t0
	service.phaseUpdatedAt["phase2"] = t0.Add(-5 * time.Millisecond)
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 1, value: 5.0})
	assert.True(t, service.isCharging, "Should NOT emergency-stop on stale Phase 2 reading when Phase 1 updates at 10.1s")

	// 3. When Phase 2 reports a fresh overcurrent reading at 10.1s, emergency stop triggers
	service.updateCurrents(&powerEvent{sensorType: SensorTypeImport, phaseIndex: 2, value: 22.8})
	assert.False(t, service.isCharging, "Should emergency-stop once Phase 2 confirms sustained overcurrent after 10s")
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

func TestDawnConsumer_PIDStepUpCapAndResetAfterStart(t *testing.T) {
	service := &dawnConsumerService{
		isCharging:         false,
		pvOnlyMode:         false,
		minimumAmps:        6.0,
		maximumAmps:        16.0,
		setpoint:           20.0,
		currents:           map[string]float64{"phase1": 4.0},
		hasDirectionalData: make(map[string]bool),
		phaseUpdatedAt:     make(map[string]time.Time),
		exports:            make(map[string]float64),
		haService:          &haService{},
		connectorStatus:    "charging",
		pid: &PIDController{
			Setpoint: 20.0,
			Kp:       0.4,
			Ki:       0.01,
			Kd:       0.05,
			// Simulate stale LastTime from 2 hours ago before start
			LastTime: time.Now().Add(-2 * time.Hour),
			Integral: 40.0,
		},
	}

	// 1. Start charging: should reset PID (LastTime & Integral) and set lastExecution
	service.calculateAndSetAmps()
	assert.True(t, service.isCharging)
	assert.Equal(t, 6.0, service.currentAmps)
	assert.WithinDuration(t, time.Now(), service.pid.LastTime, time.Second, "PID LastTime should be refreshed on charging start")
	assert.Equal(t, 0.0, service.pid.Integral, "PID Integral should be reset on charging start")

	// 2. Immediate next P1 reading (5s later) should be throttled by the 30s start window
	service.calculateAndSetAmps()
	assert.Equal(t, 6.0, service.currentAmps, "Should not ramp up 5s after start before 30s throttle expires")

	// 3. After 30s throttle expires, the very first PID call adjusts immediately, capped at +2A (6A -> 8A)
	time.Sleep(150 * time.Millisecond)
	service.lastExecution = time.Now().Add(-31 * time.Second)
	service.calculateAndSetAmps() // large error (16A), but capped at +2A step
	assert.Equal(t, 8.0, service.currentAmps, "Single PID upward step should be capped at +2A (6A -> 8A)")
}

func TestDawnConsumer_ExternalStartEnforcesPriceLimitImmediately(t *testing.T) {
	haSubChan := make(chan *gohaws.Message, 10)
	haSvc := &haService{}
	service := &dawnConsumerService{
		isCharging:        false,
		pvOnlyMode:        false,
		minimumAmps:       6.0,
		maximumAmps:       16.0,
		currentAmps:       6.0,
		setpoint:          20.0,
		currentPriceLimit: 5.0,
		currentPrice:      7.59, // Over limit
		hasPrice:          true,
		exports:           make(map[string]float64),
		currents:          make(map[string]float64),
		haService:         haSvc,
		haChannel:         haSubChan,
		pid:               &PIDController{},
		connectorStatus:   "suspendedev",
		dawnId:            "number.dawn_amps",
		dawnSwitch:        "switch.dawn_charge_control",
	}

	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	defer cancel()
	go service.run()

	// Simulate external transition suspendedev -> charging while price (7.59) > limit (5.00)
	haSubChan <- &gohaws.Message{
		Event: &gohaws.Event{
			Data: &gohaws.Data{
				EntityID: "sensor.dawn_status_connector",
				NewState: &gohaws.State{State: "charging"},
			},
		},
	}

	time.Sleep(50 * time.Millisecond)

	service.mu.RLock()
	isCharging := service.isCharging
	service.mu.RUnlock()

	assert.False(t, isCharging, "External charging start should be stopped immediately if price limit is exceeded")
}
