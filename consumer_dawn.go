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
	overcurrentStartTime time.Time
	isCharging           bool // Tracks if the charger is technically "on"
	connectorStatus      string
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string, dawnSwitch string, notifyDevice string, setpoint float64) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribe(statusSensor, haChannel)

	pid := &PIDController{
		Kp:       0.5,
		Ki:       0.05,
		Kd:       0.1,
		Setpoint: setpoint,
	}

	dawnConsumerService := &dawnConsumerService{
		ctx:          ctx,
		haService:    ha,
		minimumAmps:  6,
		maximumAmps:  16,
		currentAmps:  6,
		haChannel:    haChannel,
		dawnId:       dawnId,
		dawnSwitch:   dawnSwitch,
		notifyDevice: notifyDevice,
		currents:     make(map[string]float64),
		pid:          pid,
		setpoint:     setpoint,
		isCharging:   true,
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
				log.Printf("DAWN: connector status updated: %s", state)
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
	// If the emergency stop triggered, we definitely aren't charging
	if !tc.isCharging {
		return false
	}

	// Logic for common EV charger statuses (strings or codes)
	// Add/adjust these based on your specific HA sensor values
	switch tc.connectorStatus {
	case "charging", "3", "busy":
		return true
	case "connected", "2", "awaiting start":
		// If connected but not charging yet, we still want to set the AMPS
		// so it starts at the correct level
		return true
	default:
		// disconnected, finishing, error, etc.
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
			tc.currentAmps = tc.minimumAmps
			tc.haService.updateAmpsDawn(int(tc.currentAmps), tc.dawnId)
			tc.pid.Integral = 0
			tc.overcurrentStartTime = time.Time{}
		}
		return
	}

	// 2. CHECK IF ADJUSTMENT IS NEEDED
	// If the car is not plugged in or charging, don't bother the HA API with updates
	if !tc.isActuallyCharging() {
		// We still keep the PID error fresh but don't execute
		tc.pid.Update(maxPhaseCurrent)
		return
	}

	// 3. SAFETY OVERRIDE & STOP LOGIC
	if maxPhaseCurrent > tc.setpoint {
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
			}
		}
		return
	}

	tc.overcurrentStartTime = time.Time{}

	// 4. OPTIMIZATION LAYER (PID)
	adjustment := tc.pid.Update(maxPhaseCurrent)
	targetAmps := tc.currentAmps + adjustment

	if targetAmps < tc.minimumAmps {
		targetAmps = tc.minimumAmps
	}
	if targetAmps > tc.maximumAmps {
		targetAmps = tc.maximumAmps
	}

	if int(targetAmps) != int(tc.currentAmps) {
		log.Printf("DAWN: PID Adjustment %vA -> %vA (Max Phase: %.2fA)", int(tc.currentAmps), int(targetAmps), maxPhaseCurrent)
		tc.setAmps(targetAmps)
	} else {
		tc.currentAmps = targetAmps
	}
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
