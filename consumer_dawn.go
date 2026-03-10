package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/tuomaz/gohaws"
)

type dawnConsumerService struct {
	ctx                  context.Context
	haService            *haService
	currentAmps          float64 // The setting we have requested
	actualAmps           float64 // What the car is actually drawing
	minimumAmps          float64
	maximumAmps          float64
	haChannel            chan *gohaws.Message
	eventChannel         chan *event
	dawnId               string
	dawnSwitch           string
	notifyDevice         string
	dawnCurrentId        string
	pvOnlySwitchId       string
	currents             map[string]float64
	exports              map[string]float64
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

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string, dawnSwitch string, notifyDevice string, dawnCurrentId string, setpoint float64, pvOnlySwitchId string) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{statusSensor, dawnCurrentId, pvOnlySwitchId}, haChannel)

	pid := &PIDController{
		Kp:       0.4,
		Ki:       0.01,
		Kd:       0.05,
		Setpoint: setpoint,
	}

	dawnConsumerService := &dawnConsumerService{
		ctx:            ctx,
		haService:      ha,
		minimumAmps:    6,
		maximumAmps:    16,
		currentAmps:    6,
		actualAmps:     0,
		haChannel:      haChannel,
		dawnId:         dawnId,
		dawnSwitch:     dawnSwitch,
		notifyDevice:   notifyDevice,
		dawnCurrentId:  dawnCurrentId,
		pvOnlySwitchId: pvOnlySwitchId,
		currents:       make(map[string]float64),
		exports:        make(map[string]float64),
		pid:            pid,
		setpoint:       setpoint,
		lastExecution:  time.Now(),
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
					// Divide by 3 because the sensor is a sum of all 3 phases
					ps.actualAmps = totalAmps / 3.0
				} else if message.Event.Data.EntityID == ps.pvOnlySwitchId {
					state := strings.ToLower(fmt.Sprintf("%v", message.Event.Data.NewState.State))
					oldMode := ps.pvOnlyMode
					ps.pvOnlyMode = state == "on"
					if oldMode != ps.pvOnlyMode {
						log.Printf("DAWN: PV-only mode changed: %v -> %v. Resetting PID.", oldMode, ps.pvOnlyMode)
						ps.pid.Integral = 0
						ps.pid.LastError = 0
						ps.pid.LastTime = time.Time{}
					}
				} else {
					state := strings.ToLower(fmt.Sprintf("%v", message.Event.Data.NewState.State))
					ps.connectorStatus = state
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
	
	if pe.sensorType == SensorTypeExport {
		tc.exports[phaseKey] = pe.value
		// If we are exporting, import current is 0
		tc.currents[phaseKey] = 0
	} else if pe.sensorType == SensorTypeCurrent {
		tc.currents[phaseKey] = pe.value
		// If we are importing, export current is 0
		tc.exports[phaseKey] = 0
	}

	tc.calculateAndSetAmps()
}

func (tc *dawnConsumerService) isActuallyCharging() bool {
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
	maxPhaseCurrent := tc.getMaxCurrent()
	minPhaseExport := tc.getMinExport()

	// 1. RESTART LOGIC
	if !tc.isCharging {
		canStart := false
		if tc.pvOnlyMode {
			// PV-Only Start Condition: All 3 phases must export >= 6A
			if minPhaseExport >= tc.minimumAmps {
				if tc.pvSurplusStartTime.IsZero() {
					tc.pvSurplusStartTime = time.Now()
					log.Printf("DAWN: PV surplus detected (%.2fA). Starting 5m stabilization timer.", minPhaseExport)
				} else if time.Since(tc.pvSurplusStartTime) > 5*time.Minute {
					canStart = true
					log.Printf("DAWN: PV surplus sustained for 5m. Starting EV charging.")
				}
			} else {
				tc.pvSurplusStartTime = time.Time{}
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
			tc.setAmps(tc.minimumAmps)
			tc.pid.Integral = 0
			tc.overcurrentStartTime = time.Time{}
			tc.pvSurplusStartTime = time.Time{}
			tc.pvShortageStartTime = time.Time{}
		}
		return
	}

	// 2. HARD SAFETY OVERRIDE (Fuses)
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

				tc.stopCharging()
				return
			}
		} else {
			overage := maxPhaseCurrent - tc.setpoint
			reduction := math.Ceil(overage)
			
			newAmps := math.Max(tc.minimumAmps, baseline-reduction)

			if int(newAmps) != int(tc.currentAmps) {
				log.Printf("DAWN: HARD SAFETY REDUCTION! Max phase %.2fA. Car drawing %.2fA. Reducing setting %vA -> %vA", maxPhaseCurrent, tc.actualAmps, int(tc.currentAmps), int(newAmps))
				tc.setAmps(newAmps)
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
		// Stop if importing on any phase (> 1.0A) while at minimum charging
		if maxPhaseCurrent > 1.0 && tc.currentAmps <= tc.minimumAmps {
			if tc.pvShortageStartTime.IsZero() {
				tc.pvShortageStartTime = time.Now()
				log.Printf("DAWN: PV shortage (grid import detected: %.2fA) at minimum charging. Starting 5m shutdown timer.", maxPhaseCurrent)
			} else if time.Since(tc.pvShortageStartTime) > 5*time.Minute {
				log.Printf("DAWN: PV shortage sustained for 5m. Stopping EV charging to avoid grid costs.")
				tc.stopCharging()
				return
			}
		} else {
			tc.pvShortageStartTime = time.Time{}
		}
	}

	// 4. THROTTLE & LOCKOUT
	if time.Since(tc.lastExecution) < 20*time.Second {
		return
	}
	if time.Since(tc.lastHardSafetyEvent) < 60*time.Second {
		return
	}

	// 5. CHECK IF ADJUSTMENT IS NEEDED
	if !tc.isActuallyCharging() {
		return
	}

	// 6. OPTIMIZATION LAYER (PID)
	var input float64
	var currentSetpoint float64

	if tc.pvOnlyMode {
		// Goal: Keep export at ~0.5A (to avoid import)
		// Input is the current export surplus. If surplus is 1A, and we want 0.5A, error is 0.5A.
		// Actually, let's use a simpler approach:
		// error = (minPhaseExport - 0.5)
		// Since PID expects error = setpoint - input:
		// setpoint = 0.5, input = minPhaseExport
		currentSetpoint = 0.5
		input = minPhaseExport
		// But minPhaseExport is 0 if we are importing. In that case, we should use negative value of import.
		if minPhaseExport == 0 {
			input = -maxPhaseCurrent
		}
	} else {
		currentSetpoint = tc.setpoint
		input = maxPhaseCurrent
	}

	tc.pid.Setpoint = currentSetpoint
	adjustment := tc.pid.Update(input)
	
	// If in PV mode, adjustment is positive if input > setpoint (meaning we have more surplus than 0.5A)
	// But standard PID: adjustment = Kp * (setpoint - input).
	// If input (maxPhaseCurrent) > setpoint, adjustment is negative (reduce charging). Correct for Normal Mode.
	// If input (minPhaseExport) > setpoint (0.5), adjustment is negative? No, if we have surplus, we want to INCREASE.
	if tc.pvOnlyMode {
		adjustment = -adjustment // Invert because higher input (export) should mean higher charging
	}

	targetAmps := tc.currentAmps + adjustment

	if targetAmps < tc.minimumAmps {
		targetAmps = tc.minimumAmps
	}
	if targetAmps > tc.maximumAmps {
		targetAmps = tc.maximumAmps
	}

	if int(targetAmps) != int(tc.currentAmps) {
		modeStr := "NORMAL"
		if tc.pvOnlyMode {
			modeStr = "PV-ONLY"
		}
		log.Printf("DAWN: %s PID Adjustment %vA -> %vA (Max Phase: %.2fA, Min Export: %.2fA, Actual Draw: %.2fA)", modeStr, int(tc.currentAmps), int(targetAmps), maxPhaseCurrent, minPhaseExport, tc.actualAmps)
		tc.setAmps(targetAmps)
	} else {
		tc.currentAmps = targetAmps
	}
	tc.lastExecution = time.Now()
}

func (tc *dawnConsumerService) stopCharging() {
	tc.isCharging = false
	tc.haService.setDawnSwitch(false, tc.dawnSwitch)
	tc.overcurrentStartTime = time.Time{}
	tc.pvShortageStartTime = time.Time{}
}

func (tc *dawnConsumerService) getMinExport() float64 {
	min := 999.0
	if len(tc.exports) < 3 {
		return 0 // We haven't received data for all phases yet or not exporting on all
	}
	for _, value := range tc.exports {
		if value < min {
			min = value
		}
	}
	return min
}

func (tc *dawnConsumerService) setAmps(amps float64) {
	tc.currentAmps = amps
	tc.haService.updateAmpsDawn(int(tc.currentAmps), tc.dawnId)
}

func (tc *dawnConsumerService) getMaxCurrent() float64 {
	max := 0.0
	for _, value := range tc.currents {
		if value > max {
			max = value
		}
	}
	return max
}
