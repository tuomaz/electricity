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
	haChannel    chan *gohaws.Message
	eventChannel chan *event
}

func newDawnConsumerService(ctx context.Context, eventChannel chan *event, ha *haService, statusSensor string) *dawnConsumerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribe(statusSensor, haChannel)
	dawnConsumerService := &dawnConsumerService{
		ctx:         ctx,
		haService:   ha,
		currentAmps: 0,
		haChannel:   haChannel,
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
}

// sensor.dawn_status_connector
