package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tuomaz/gohaws"
)

type dawnConsumerService struct {
	ctx          context.Context
	haService    *haService
	currentAmps  float64
	minimumAmps  float64
	haChannel    chan *gohaws.Message
	eventChannel chan *event
	dawnId       string
	currents     map[string]float64
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string, dawnId string) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribe(statusSensor, haChannel)
	dawnConsumerService := &dawnConsumerService{
		ctx:         ctx,
		haService:   ha,
		minimumAmps: 6,
		currentAmps: 0,
		haChannel:   haChannel,
		dawnId:      dawnId,
		currents:    make(map[string]float64),
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
				state := fmt.Sprintf("%v", message.Event.Data.NewState.State)
				log.Printf("DAWN: new charging state: %s\n", state)
			} else {
				break Loop
			}
		}
	}
}

func (tc *dawnConsumerService) updateAmps(amps int) {
}

func (tc *dawnConsumerService) decreaseAmps(amps float64) {
	if tc.currentAmps > 0 && tc.currentAmps <= tc.minimumAmps+1 {
		// cannot do much
		log.Printf("Dawn amps already at lowest")

	} else {
		tc.currentAmps = tc.currentAmps - amps
		if tc.currentAmps < 6 {
			tc.currentAmps = 6
		}
		log.Printf("DAWN: setting new amps: %v", tc.currentAmps)
		tc.haService.updateAmpsDawn(int(tc.currentAmps), tc.dawnId)
	}
}

func (tc *dawnConsumerService) updateCurrents(phase string, amps float64) {
	//log.Printf("DAWN: saving %s : %f", phase, amps)
	tc.currents[phase] = amps
	tc.setCurrentCurrent()
}

func (tc *dawnConsumerService) setCurrentCurrent() {
	currentMaxAmp := tc.getMaxCurrent()
	//log.Printf("DAWN: max current %f", currentMaxAmp)
	if currentMaxAmp > 1 {
		change := 0.0
		if currentMaxAmp > MAX_PHASE_CURRENT {
			// need to lower
			change = currentMaxAmp - MAX_PHASE_CURRENT
			if tc.currentAmps-change > 6 {
				tc.currentAmps = tc.currentAmps - change
			} else {
				tc.currentAmps = 6
			}
			tc.haService.updateAmpsDawn(int(tc.currentAmps), tc.dawnId)
			log.Printf("DAWN: lowered amps to %d", int(tc.currentAmps))
		} else {
			if int(currentMaxAmp+1) < 20 && tc.currentAmps < 16 {
				tc.haService.updateAmpsDawn(int(tc.currentAmps+1), tc.dawnId)
				log.Printf("DAWN: increased amps to %d", int(tc.currentAmps+1))
				tc.currentAmps += 1
			}
		}
	}
}

func (tc *dawnConsumerService) getMaxCurrent() float64 {
	max := 0.0
	for _, value := range tc.currents {
		if value > max {
			max = value
			//log.Printf("DAWN: found max: %s : %f", key, value)
		}
	}
	return max
}

// sensor.dawn_status_connector
