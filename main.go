package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-co-op/gocron"
)

func signalHandler(cancel context.CancelFunc, sigs chan os.Signal) {
	sig := <-sigs
	log.Printf("Received signal %v, exiting...", sig)
	cancel()
}

const MAX_PHASE_CURRENT = 20

func main() {
	log.Print("Starting up alpha version 1")
	baseCtx := context.Background()
	ctx, cancel := context.WithCancel(baseCtx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go signalHandler(cancel, sigs)

	haUri, haToken, area, dawn, dawnSwitch, notifyDevice := readEnv()

	events := make(chan *event)

	haService := newHaService(ctx, haUri, haToken)
	_ = newPowerService(ctx, events, haService, "sensor.current_phase_1", "sensor.current_phase_2", "sensor.current_phase_3", MAX_PHASE_CURRENT)
	priceService := newPriceService(area)
	dawnService := newDawnConsumerService(ctx, events, haService, "sensor.dawn_status_connector", dawn, dawnSwitch, notifyDevice, MAX_PHASE_CURRENT)

	// TODO: move this inside service
	s := gocron.NewScheduler(time.UTC)
	job, err := s.Every(30).Minutes().Do(func() {
		updated, _ := priceService.updatePrices()
		if updated {
			log.Printf("Prices updated!")
		}
	})
	if err != nil {
		log.Fatalf("error setting up cron: %v", err)
	}
	s.StartAsync()

	log.Printf("Start main loop")

MainLoop:
	for {
		select {
		case <-ctx.Done():
			break MainLoop
		case event, ok := <-events:
			if ok {
				if event.powerEvent != nil {
					dawnService.updateCurrents(event.powerEvent.phase, event.powerEvent.current)
				}
			} else {
				break MainLoop
			}
		}
	}
	log.Printf("End main loop")
	s.Remove(job)
}

func readEnv() (string, string, string, string, string, string) {
	var haURI, haToken, area, dawn, dawnSwitch, notifyDevice string
	value, ok := os.LookupEnv("HAURI")
	if ok {
		haURI = value
	} else {
		log.Fatalf("no Home Assistant URI found")
	}

	value, ok = os.LookupEnv("HATOKEN")
	if ok {
		haToken = value
	} else {
		log.Fatalf("no Home Assistant auth token found")
	}

	value, ok = os.LookupEnv("AREA")
	if ok {
		area = value
	} else {
		log.Fatalf("no ID found")
	}

	value, ok = os.LookupEnv("DAWN")
	if ok {
		dawn = value
	} else {
		log.Fatalf("no Dawn device found")
	}

	value, ok = os.LookupEnv("DAWN_SWITCH")
	if ok {
		dawnSwitch = value
	} else {
		log.Fatalf("no Dawn switch found")
	}

	value, ok = os.LookupEnv("NOTIFY_DEVICE")
	if ok {
		notifyDevice = value
	} else {
		log.Fatalf("no notify device found")
	}

	return haURI, haToken, area, dawn, dawnSwitch, notifyDevice
}
