package main

import (
	"github.com/tuomaz/gohaws"
)

type PowerService struct {
	phase1  string
	channel chan gohaws.Message
}

func newPowerService(phase1 string) *PowerService {
	powerService := &PowerService{}
	return powerService
}

func (ps *PowerService) run() {

}
