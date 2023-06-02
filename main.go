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
	<-sigs
	cancel()

}

func main() {
	baseCtx := context.Background()
	ctx, cancel := context.WithCancel(baseCtx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go signalHandler(cancel, sigs)

	haUri, haToken, _, area, _ := readEnv()

	events := make(chan *event)

	haService := newHaService(baseCtx, haUri, haToken)

	_ = newPowerService(ctx, events, *haService, "momentary_active_import_phase_1", "momentary_active_import_phase_2", "momentary_active_import_phase_3")

	priceService := newPriceService(area)

	/*
		priceService.updatePrices()
		time.Sleep(5000)
		priceService.updatePrices()
	*/

	// TODO: move this inside service
	s := gocron.NewScheduler(time.UTC)
	job, err := s.Every(10).Minutes().Do(func() {
		updated, _ := priceService.updatePrices()
		if updated {
			log.Printf("Prices updated!")
		}
	})
	if err != nil {
		log.Fatalf("error setting up cron: %v", err)
	}
	s.StartAsync()

	//currentAmps := 5

	/*
		td := &Tesla{
			Command: "CHARGING_AMPS",
			Parameters: &Parameters{
				PathVars: &PathVars{
					VehicleID: id,
				},
				ChargingAmps: currentAmps,
			},
		}

	*/

	//haClient.CallService(ctx, "tesla_custom", "api", td)

MainLoop:
	for {
		select {
		case <-ctx.Done():
			break MainLoop
		case event, ok := <-events:
			if ok {
				log.Printf("Recieved event: %v", event)
			} else {
				break MainLoop
			}
		}
	}
	s.Remove(job)
}

func readEnv() (string, string, string, string, string) {
	var haURI, haToken, id, area, notifyDevice string
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

	value, ok = os.LookupEnv("ID")
	if ok {
		id = value
	} else {
		log.Fatalf("no ID found")
	}

	value, ok = os.LookupEnv("AREA")
	if ok {
		area = value
	} else {
		log.Fatalf("no ID found")
	}

	value, ok = os.LookupEnv("NOTIFY_DEVICE")
	if ok {
		notifyDevice = value
	} else {
		log.Fatalf("no notify device found")
	}

	return haURI, haToken, id, area, notifyDevice
}
