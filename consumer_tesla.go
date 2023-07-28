package main

import (
	"context"
	"log"

	"github.com/tuomaz/gohaws"
)

type teslaConsumerService struct {
	ctx          context.Context
	haService    *haService
	vehicleID    string
	currentAmps  float64
	haChannel    chan *gohaws.Message
	eventChannel chan *event
}

func newteslaConsumerService(ctx context.Context, eventChannel chan *event, haService *haService, vehicleID string) *teslaConsumerService {
	haChannel := make(chan *gohaws.Message)
	teslaConsumerService := &teslaConsumerService{
		ctx:         ctx,
		haService:   haService,
		vehicleID:   vehicleID,
		currentAmps: 0,
		haChannel:   haChannel,
	}

	return teslaConsumerService
}

func (tc *teslaConsumerService) updateAmps(amps int) {
	if amps > 6 && amps < 17 {
		tc.haService.updateAmpsTesla(amps, tc.vehicleID)
	} else {
		log.Printf("TESLA: not setting new amps, not in range: %d\n", amps)
	}

}

func (tc *teslaConsumerService) decreaseAmps(amps float64) {
	if tc.currentAmps > 0 {
		tc.currentAmps = tc.currentAmps - amps
		log.Printf("TESLA: setting new amps: %v", tc.currentAmps)
		tc.updateAmps(int(tc.currentAmps))
	}
}
