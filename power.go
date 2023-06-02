package main

import (
	"context"
	"log"

	"github.com/tuomaz/gohaws"
)

type PowerService struct {
	ctx    context.Context
	phase1 string
	phase2 string
	phase3 string

	haChannel    chan *gohaws.Message
	eventChannel chan *event
}

func newPowerService(ctx context.Context, eventChannel chan *event, ha haService, phase1 string, phase2 string, phase3 string) *PowerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{phase1, phase2, phase3}, haChannel)

	powerService := &PowerService{
		ctx:          ctx,
		eventChannel: eventChannel,
		phase1:       phase1,
		phase2:       phase2,
		phase3:       phase3,
		haChannel:    haChannel,
	}

	powerService.run()

	return powerService
}

func (ps *PowerService) run() {
	for {
		select {
		case <-ps.ctx.Done():
			break
		case message, ok := <-ps.haChannel:
			if ok {
				powerEvent := &powerEvent{}

				event := &event{
					powerEvent: *powerEvent,
				}

				ps.eventChannel <- event

				log.Printf("Power service received message: %v", message)
			} else {
				break
			}
		}
	}
}
