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
	currents             map[string]float64
	pid                  *PIDController
	setpoint             float64
	overcurrentStartTime time.Time
	isCharging           bool
	connectorStatus      string
	lastExecution        time.Time
	lastHardSafetyEvent  time.Time
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string, dawnSwitch string, notifyDevice string, dawnCurrentId string, setpoint float64) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{statusSensor, dawnCurrentId}, haChannel)

	pid := &PIDController{
		Kp:       0.4,
		Ki:       0.01,
		Kd:       0.05,
		Setpoint: setpoint,
	}

	dawnConsumerService := &dawnConsumerService{
		ctx:           ctx,
		haService:     ha,
		minimumAmps:   6,
		maximumAmps:   16,
		currentAmps:   6,
		actualAmps:    0,
		haChannel:     haChannel,
		dawnId:        dawnId,
		dawnSwitch:    dawnSwitch,
		notifyDevice:  notifyDevice,
		dawnCurrentId: dawnCurrentId,
		currents:      make(map[string]float64),
		pid:           pid,
		setpoint:      setpoint,
		lastExecution: time.Now(),
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
					//log.Printf("DAWN: total draw %.2fA -> per-phase draw: %.2fA", totalAmps, ps.actualAmps)
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

func (tc *dawnConsumerService) updateCurrents(phase string, amps float64) {
	tc.currents[phase] = amps
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
	if maxPhaseCurrent == 0 {
		return
	}

	// 1. RESTART LOGIC
	if !tc.isCharging {
		if maxPhaseCurrent <= tc.setpoint-8.0 {
			msg := fmt.Sprintf("Sufficient headroom (%.2fA). Restarting EV charging.", tc.setpoint-maxPhaseCurrent)
			log.Printf("DAWN: %s", msg)
			tc.haService.sendNotification(msg, tc.notifyDevice)

			tc.isCharging = true
			tc.haService.setDawnSwitch(true, tc.dawnSwitch)
			tc.setAmps(tc.minimumAmps)
			tc.pid.Integral = 0
			tc.overcurrentStartTime = time.Time{}
		}
		return
	}

	// 2. HARD SAFETY OVERRIDE
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

				tc.isCharging = false
				tc.haService.setDawnSwitch(false, tc.dawnSwitch)
				tc.overcurrentStartTime = time.Time{}
				return
			}
		} else {
			overage := maxPhaseCurrent - tc.setpoint
			reduction := math.Ceil(overage)
			
			// We reduce from the ACTUAL draw, not the theoretical target
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

	// 3. THROTTLE & LOCKOUT
	if time.Since(tc.lastExecution) < 20*time.Second {
		return
	}
	if time.Since(tc.lastHardSafetyEvent) < 60*time.Second {
		return
	}

	// 4. CHECK IF ADJUSTMENT IS NEEDED
	if !tc.isActuallyCharging() {
		return
	}

	// 5. OPTIMIZATION LAYER (PID)
	adjustment := tc.pid.Update(maxPhaseCurrent)
	targetAmps := tc.currentAmps + adjustment

	if targetAmps < tc.minimumAmps {
		targetAmps = tc.minimumAmps
	}
	if targetAmps > tc.maximumAmps {
		targetAmps = tc.maximumAmps
	}

	if int(targetAmps) != int(tc.currentAmps) {
		log.Printf("DAWN: PID Adjustment %vA -> %vA (Max Phase: %.2fA, Actual Draw: %.2fA)", int(tc.currentAmps), int(targetAmps), maxPhaseCurrent, tc.actualAmps)
		tc.setAmps(targetAmps)
	} else {
		tc.currentAmps = targetAmps
	}
	tc.lastExecution = time.Now()
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
