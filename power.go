package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/tuomaz/gohaws"
)

type PowerService struct {
	ctx    context.Context
	phase1 string
	phase2 string
	phase3 string
	max    float64

	haChannel    chan *gohaws.Message
	eventChannel chan *event
}

func newPowerService(ctx context.Context, eventChannel chan *event, ha *haService, phase1 string, phase2 string, phase3 string, max float64) *PowerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{phase1, phase2, phase3}, haChannel)

	powerService := &PowerService{
		ctx:          ctx,
		eventChannel: eventChannel,
		phase1:       phase1,
		phase2:       phase2,
		phase3:       phase3,
		max:          max,
		haChannel:    haChannel,
	}

	go powerService.run()

	return powerService
}

func (ps *PowerService) run() {
Loop:
	for {
		select {
		case <-ps.ctx.Done():
			break Loop
		case message, ok := <-ps.haChannel:
			if ok {
				current := parseFloat(message.Event.Data.NewState.State)
				log.Printf("POWER: current amps %f (%s)\n", current, message.Event.Data.EntityID)

				powerEvent := &powerEvent{
					phase:   message.Event.Data.EntityID,
					current: current,
				}

				if current > ps.max {
					log.Printf("POWER: overcurrent! %v vs %v, phase %s", current, ps.max, message.Event.Data.EntityID)
					powerEvent.overCurrent = current - ps.max
				}

				event := &event{
					powerEvent: powerEvent,
				}

				ps.eventChannel <- event
			} else {
				break Loop
			}
		}
	}
}

func parseFloat(fs interface{}) float64 {
	ff, err := strconv.ParseFloat(fmt.Sprintf("%v", fs), 64)
	if err != nil {
		return 0
	}

	return ff
}
