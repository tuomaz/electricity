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
	log.Printf("Exiting...")
	<-sigs
	cancel()

}

func main() {
	baseCtx := context.Background()
	ctx, cancel := context.WithCancel(baseCtx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go signalHandler(cancel, sigs)

	haUri, haToken, vehicleID, area, _ := readEnv()

	events := make(chan *event)

	haService := newHaService(ctx, haUri, haToken)
	_ = newPowerService(ctx, events, haService, "sensor.current_phase_1", "sensor.current_phase_2", "sensor.current_phase_3", 16)
	priceService := newPriceService(area)
	teslaService := newteslaConsumerService(ctx, events, haService, vehicleID)
	_ = newDawnConsumerService(ctx, events, haService, "sensor.dawn_status_connector")

	//go haService.run()

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

	log.Printf("Start main loop")

MainLoop:
	for {
		select {
		case <-ctx.Done():
			break MainLoop
		case event, ok := <-events:
			if ok {
				if event.powerEvent != nil {
					if event.powerEvent.overcurrent > 0 {
						log.Printf("MAIN: decrease amps: %v", event.powerEvent.overcurrent)
						teslaService.decreaseAmps(event.powerEvent.overcurrent)
					}
				}

			} else {
				break MainLoop
			}
		}
	}
	log.Printf("End main loop")
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
