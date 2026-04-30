package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/tuomaz/gohaws"
)

type dawnConsumerService struct {
	ctx                  context.Context
	haService            *haService
	mu                   sync.RWMutex
	currentAmps          float64 // The setting we have requested
	actualAmps           float64 // What the car is actually drawing
	minimumAmps          float64
	maximumAmps          float64
	userLimit            float64
	haChannel            chan *gohaws.Message
	eventChannel         chan *event
	dawnId               string
	dawnSwitch           string
	notifyDevice         string
	dawnCurrentId        string
	pvOnlySwitchId       string
	userLimitId          string
	currents             map[string]float64
	exports              map[string]float64
	hasDirectionalData   map[string]bool
	pid                  *PIDController
	setpoint             float64
	overcurrentStartTime time.Time
	pvShortageStartTime  time.Time
	pvSurplusStartTime   time.Time
	isCharging           bool
	pvOnlyMode           bool
	connectorStatus      string
	lastExecution        time.Time
	lastHardSafetyEvent  time.Time
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string, dawnSwitch string, notifyDevice string, dawnCurrentId string, setpoint float64, pvOnlySwitchId string, userLimitId string) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{statusSensor, dawnCurrentId, pvOnlySwitchId, userLimitId}, haChannel)

	pid := &PIDController{
		Kp:       0.4,
		Ki:       0.01,
		Kd:       0.05,
		Setpoint: setpoint,
	}

	dawnConsumerService := &dawnConsumerService{
		ctx:                ctx,
		haService:          ha,
		minimumAmps:        6,
		maximumAmps:        16,
		userLimit:          16,
		currentAmps:        6,
		actualAmps:         0,
		haChannel:          haChannel,
		dawnId:             dawnId,
		dawnSwitch:         dawnSwitch,
		notifyDevice:       notifyDevice,
		dawnCurrentId:      dawnCurrentId,
		pvOnlySwitchId:     pvOnlySwitchId,
		userLimitId:        userLimitId,
		currents:           make(map[string]float64),
		exports:            make(map[string]float64),
		hasDirectionalData: make(map[string]bool),
		pid:                pid,
		setpoint:           setpoint,
		lastExecution:      time.Now(),
	}

	go dawnConsumerService.run()

	return dawnConsumerService
}

func (ps *dawnConsumerService) run() {
Loop:
	for {
		select {
		case <-ps.ctx.Done():
			break Loop
		case message, ok := <-ps.haChannel:
			if ok {
				if message.Event.Data.EntityID == ps.dawnCurrentId {
					totalAmps := parseFloat(message.Event.Data.NewState.State)
					ps.mu.Lock()
					// Divide by 3 because the sensor is a sum of all 3 phases
					ps.actualAmps = totalAmps / 3.0
					ps.mu.Unlock()
				} else if message.Event.Data.EntityID == ps.userLimitId {
					limit := parseFloat(message.Event.Data.NewState.State)
					ps.mu.Lock()
					if limit > 0 {
						ps.userLimit = limit
						log.Printf("DAWN: User limit updated: %.2fA", limit)
					}
					ps.mu.Unlock()
					ps.calculateAndSetAmps()
				} else if message.Event.Data.EntityID == ps.pvOnlySwitchId {
					state := strings.ToLower(fmt.Sprintf("%v", message.Event.Data.NewState.State))
					ps.mu.Lock()
					oldMode := ps.pvOnlyMode
					ps.pvOnlyMode = state == "on"
					if oldMode != ps.pvOnlyMode {
						log.Printf("DAWN: PV-only mode changed: %v -> %v. Resetting PID.", oldMode, ps.pvOnlyMode)
						ps.pid.Integral = 0
						ps.pid.LastError = 0
						ps.pid.LastTime = time.Time{}
					}
					ps.mu.Unlock()
					ps.calculateAndSetAmps()
				} else {
					state := strings.ToLower(fmt.Sprintf("%v", message.Event.Data.NewState.State))
					ps.mu.Lock()
					ps.connectorStatus = state
					// Sync isCharging state with reality
					switch state {
					case "charging", "3", "busy":
						if !ps.isCharging {
							log.Printf("DAWN: Detected external charging start. Enabling safety monitoring.")
							ps.isCharging = true
						}
					case "disconnected", "1", "finishing", "error":
						if ps.isCharging {
							log.Printf("DAWN: Detected charging stop (Status: %s).", state)
							ps.isCharging = false
						}
					}
					ps.mu.Unlock()
					log.Printf("DAWN: connector status: %s", state)
				}
			} else {
				break Loop
			}
		}
	}
}

func (tc *dawnConsumerService) updateCurrents(pe *powerEvent) {
	phaseKey := fmt.Sprintf("phase%d", pe.phaseIndex)

	tc.mu.Lock()
	if pe.sensorType == SensorTypeExport {
		tc.exports[phaseKey] = pe.value
		// If we are exporting, import current is 0
		tc.currents[phaseKey] = 0
		tc.hasDirectionalData[phaseKey] = true
	} else if pe.sensorType == SensorTypeImport {
		tc.currents[phaseKey] = pe.value
		// If we are importing, export current is 0
		tc.exports[phaseKey] = 0
		tc.hasDirectionalData[phaseKey] = true
	} else if pe.sensorType == SensorTypeCurrent {
		// Use absolute current sensor ONLY as a fallback if we haven't seen directional data.
		if !tc.hasDirectionalData[phaseKey] {
			tc.currents[phaseKey] = pe.value
		}
	}
	tc.mu.Unlock()

	tc.calculateAndSetAmps()
}

func (tc *dawnConsumerService) isActuallyCharging() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if !tc.isCharging {
		return false
	}

	switch tc.connectorStatus {
	case "charging", "3", "busy":
		return true
	case "connected", "2", "awaiting start":
		return true
	default:
		return false
	}
}

func (tc *dawnConsumerService) calculateAndSetAmps() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	maxPhaseCurrent := tc.getMaxCurrentInternal()
	netExport := tc.getNetExportInternal()

	// 1. RESTART LOGIC
	if !tc.isCharging {
		canStart := false

		if tc.pvOnlyMode {
			// PV-Only Start Condition: Total net export must be >= 18A (assuming 3-phase 6A start)
			if netExport >= tc.minimumAmps*3.0 {
				if tc.pvSurplusStartTime.IsZero() {
					tc.pvSurplusStartTime = time.Now()
					log.Printf("DAWN: PV surplus detected (Net: %.2fA). Starting 5m stabilization timer.", netExport)
				} else if time.Since(tc.pvSurplusStartTime) > 5*time.Minute {
					canStart = true
					log.Printf("DAWN: PV surplus sustained for 5m. Starting EV charging.")
				}
			} else if netExport < (tc.minimumAmps*3.0 - 3.0) {
				if !tc.pvSurplusStartTime.IsZero() {
					log.Printf("DAWN: PV surplus dropped below threshold (Net: %.2fA). Resetting stabilization timer.", netExport)
					tc.pvSurplusStartTime = time.Time{}
				}
			}
		} else {
			// Normal Start Condition: Sufficient headroom
			if maxPhaseCurrent > 0 && maxPhaseCurrent <= tc.setpoint-8.0 {
				canStart = true
				log.Printf("DAWN: Sufficient headroom (%.2fA). Starting EV charging.", tc.setpoint-maxPhaseCurrent)
			}
		}

		if canStart {
			tc.isCharging = true
			tc.haService.setDawnSwitch(true, tc.dawnSwitch)
			tc.setAmpsInternal(tc.minimumAmps)
			tc.pid.Integral = 0
			tc.overcurrentStartTime = time.Time{}
			tc.pvSurplusStartTime = time.Time{}
			tc.pvShortageStartTime = time.Time{}
		}
		return
	}

	// 2. HARD SAFETY OVERRIDE (Fuses)
	// IMPORTANT: Fuses are per-phase, so we still use maxPhaseCurrent here!
	hardSafetyThreshold := tc.setpoint + 2.0
	if maxPhaseCurrent > hardSafetyThreshold && time.Since(tc.lastHardSafetyEvent) > 5*time.Second {
		// BASELINE: Use Actual Draw if it's lower than our current setting
		baseline := math.Min(tc.currentAmps, tc.actualAmps)
		if baseline < tc.minimumAmps {
			baseline = tc.currentAmps // Fallback if actual is weirdly low (e.g. 0 during ramp)
		}

		if baseline <= tc.minimumAmps {
			if tc.overcurrentStartTime.IsZero() {
				tc.overcurrentStartTime = time.Now()
				log.Printf("DAWN: Overcurrent detected at minimum charging. Starting 10s shutdown timer.")
			} else if time.Since(tc.overcurrentStartTime) > 10*time.Second {
				msg := fmt.Sprintf("CRITICAL OVERCURRENT (%.2fA). Emergency stop of EV charger.", maxPhaseCurrent)
				log.Printf("DAWN: %s", msg)
				tc.haService.sendNotification(msg, tc.notifyDevice)

				tc.stopChargingInternal()
				return
			}
		} else {
			overage := maxPhaseCurrent - tc.setpoint
			reduction := math.Ceil(overage)

			newAmps := math.Max(tc.minimumAmps, baseline-reduction)

			if int(newAmps) != int(tc.currentAmps) {
				log.Printf("DAWN: HARD SAFETY REDUCTION! Max phase %.2fA. Car drawing %.2fA. Reducing setting %vA -> %vA", maxPhaseCurrent, tc.actualAmps, int(tc.currentAmps), int(newAmps))
				tc.setAmpsInternal(newAmps)
				tc.pid.Integral = 0
				tc.lastHardSafetyEvent = time.Now()
				tc.lastExecution = time.Now()
			}
		}
		return
	}
	tc.overcurrentStartTime = time.Time{}

	// 3. PV SHORTAGE STOP LOGIC
	if tc.pvOnlyMode && tc.isCharging {
		// Stop if net importing (> 3.0A) while at minimum charging
		// Using 3.0A as a buffer (1.0A per phase average)
		if netExport < -3.0 && tc.currentAmps <= tc.minimumAmps {
			if tc.pvShortageStartTime.IsZero() {
				tc.pvShortageStartTime = time.Now()
				log.Printf("DAWN: PV shortage (Net Import: %.2fA) at minimum charging. Starting 5m shutdown timer.", -netExport)
			} else if time.Since(tc.pvShortageStartTime) > 5*time.Minute {
				log.Printf("DAWN: PV shortage sustained for 5m. Stopping EV charging to avoid grid costs.")
				tc.stopChargingInternal()
				return
			}
		} else if netExport > -1.0 || tc.currentAmps > tc.minimumAmps {
			// Only reset the timer if we clearly have surplus or are no longer at minimum charging
			if !tc.pvShortageStartTime.IsZero() {
				log.Printf("DAWN: PV shortage resolved (Net Export: %.2fA). Resetting shutdown timer.", netExport)
				tc.pvShortageStartTime = time.Time{}
			}
		}
	}

	// 4. THROTTLE & LOCKOUT
	if time.Since(tc.lastExecution) < 30*time.Second {
		return
	}
	if time.Since(tc.lastHardSafetyEvent) < 60*time.Second {
		return
	}

	// 5. CHECK IF ADJUSTMENT IS NEEDED
	// Inline isActuallyCharging logic here to avoid deadlocks since we already hold the lock
	actuallyCharging := false
	if tc.isCharging {
		switch tc.connectorStatus {
		case "charging", "3", "busy", "connected", "2", "awaiting start":
			actuallyCharging = true
		}
	}

	if !actuallyCharging {
		return
	}

	// 6. OPTIMIZATION LAYER (PID)
	var input float64
	var currentSetpoint float64

	if tc.pvOnlyMode {
		currentSetpoint = 0.5
		// Input is "average per-phase export"
		input = netExport / 3.0
	} else {
		currentSetpoint = tc.setpoint
		input = maxPhaseCurrent
	}

	tc.pid.Setpoint = currentSetpoint
	adjustment := tc.pid.Update(input)

	if tc.pvOnlyMode {
		adjustment = -adjustment
	}

	targetAmps := tc.currentAmps + adjustment

	if targetAmps < tc.minimumAmps {
		targetAmps = tc.minimumAmps
	}
	if targetAmps > tc.maximumAmps {
		targetAmps = tc.maximumAmps
	}

	if tc.userLimit > 0 && targetAmps > tc.userLimit {
		targetAmps = tc.userLimit
	}

	if int(targetAmps) != int(tc.currentAmps) {
		modeStr := "NORMAL"
		if tc.pvOnlyMode {
			modeStr = "PV-ONLY"
		}
		log.Printf("DAWN: %s PID Adjustment %vA -> %vA (Max Phase: %.2fA, Net Export: %.2fA, Actual Draw: %.2fA)", modeStr, int(tc.currentAmps), int(targetAmps), maxPhaseCurrent, netExport, tc.actualAmps)
		tc.setAmpsInternal(targetAmps)
	} else {
		tc.currentAmps = targetAmps
	}
	tc.lastExecution = time.Now()
}

func (tc *dawnConsumerService) stopCharging() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.stopChargingInternal()
}

func (tc *dawnConsumerService) stopChargingInternal() {
	tc.isCharging = false
	tc.haService.setDawnSwitch(false, tc.dawnSwitch)
	tc.overcurrentStartTime = time.Time{}
	tc.pvShortageStartTime = time.Time{}
}

func (tc *dawnConsumerService) getMinExport() float64 {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.getMinExportInternal()
}

func (tc *dawnConsumerService) getMinExportInternal() float64 {
	min := 999.0
	if len(tc.exports) < 3 {
		return 0
	}
	for _, value := range tc.exports {
		if value < min {
			min = value
		}
	}
	return min
}

func (tc *dawnConsumerService) getNetExportInternal() float64 {
	net := 0.0
	for i := 1; i <= 3; i++ {
		phaseKey := fmt.Sprintf("phase%d", i)
		net += tc.exports[phaseKey]
		net -= tc.currents[phaseKey]
	}
	return net
}

func (tc *dawnConsumerService) setAmps(amps float64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.setAmpsInternal(amps)
}

func (tc *dawnConsumerService) setAmpsInternal(amps float64) {
	tc.currentAmps = amps
	tc.haService.updateAmpsDawn(int(tc.currentAmps), tc.dawnId)
}

func (tc *dawnConsumerService) getMaxCurrent() float64 {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.getMaxCurrentInternal()
}

func (tc *dawnConsumerService) getMaxCurrentInternal() float64 {
	max := 0.0
	for _, value := range tc.currents {
		if value > max {
			max = value
		}
	}
	return max
}
