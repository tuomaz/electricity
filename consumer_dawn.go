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
	currentAmps          float64
	minimumAmps          float64
	maximumAmps          float64
	haChannel            chan *gohaws.Message
	eventChannel         chan *event
	dawnId               string
	dawnSwitch           string
	notifyDevice         string
	currents             map[string]float64
	pid                  *PIDController
	setpoint             float64
	targetCurrent        float64 // The PID target (e.g. 18A for a 20A fuse)
	overcurrentStartTime time.Time
	isCharging           bool
	connectorStatus      string
	lastExecution        time.Time
	lastSafetyEvent      time.Time
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string, dawnSwitch string, notifyDevice string, setpoint float64) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribe(statusSensor, haChannel)

	// We set the PID target slightly below the fuse limit to provide a buffer
	targetCurrent := setpoint - 2.0

	pid := &PIDController{
		Kp:       0.4,  // Gentler response
		Ki:       0.01, // Much more patient integration
		Kd:       0.05,
		Setpoint: targetCurrent,
	}

	dawnConsumerService := &dawnConsumerService{
		ctx:           ctx,
		haService:     ha,
		minimumAmps:   6,
		maximumAmps:   16,
		currentAmps:   6,
		haChannel:     haChannel,
		dawnId:        dawnId,
		dawnSwitch:    dawnSwitch,
		notifyDevice:  notifyDevice,
		currents:      make(map[string]float64),
		pid:           pid,
		setpoint:      setpoint,
		targetCurrent: targetCurrent,
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
				state := strings.ToLower(fmt.Sprintf("%v", message.Event.Data.NewState.State))
				ps.connectorStatus = state
				log.Printf("DAWN: connector status: %s", state)
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

	// On app restart, the status might be empty until the first HA event arrives.
	// We assume it's charging so we can start managing load immediately.
	if tc.connectorStatus == "" {
		return true
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

	// 2. SAFETY OVERRIDE (Hard Fuse Protection)
	// We wait at least 5s between safety reductions to allow the charger to react
	if maxPhaseCurrent > tc.setpoint && time.Since(tc.lastSafetyEvent) > 5*time.Second {
		if tc.currentAmps <= tc.minimumAmps {
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
			newAmps := math.Max(tc.minimumAmps, tc.currentAmps-reduction)

			if int(newAmps) != int(tc.currentAmps) {
				log.Printf("DAWN: SAFETY REDUCTION! Phase current %.2fA exceeds limit. %vA -> %vA", maxPhaseCurrent, int(tc.currentAmps), int(newAmps))
				tc.setAmps(newAmps)
				tc.pid.Integral = 0
				tc.lastSafetyEvent = time.Now()
				tc.lastExecution = time.Now() // Also reset PID timer
			}
		}
		return
	}

	tc.overcurrentStartTime = time.Time{}

	// 3. THROTTLE & SAFETY LOCKOUT
	// PID decision only every 20 seconds for stability
	if time.Since(tc.lastExecution) < 20*time.Second {
		return
	}

	// If we just had a safety event, wait longer before increasing (Safety Lockout)
	if time.Since(tc.lastSafetyEvent) < 60*time.Second {
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
		log.Printf("DAWN: PID Adjustment %vA -> %vA (Max Phase: %.2fA, Target: %.1fA)", int(tc.currentAmps), int(targetAmps), maxPhaseCurrent, tc.targetCurrent)
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
