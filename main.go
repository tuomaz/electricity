package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/tuomaz/gohaws"
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

	haUri, haToken, id, area, notifyDevice := readEnv()

	haClient := gohaws.New(ctx, haUri, haToken)
	haClient.Add("sensor.momentary_active_export_phase_1")
	haClient.SubscribeToUpdates(ctx)

	priceService := newPriceService(area)

	/*
		priceService.updatePrices()
		time.Sleep(5000)
		priceService.updatePrices()
	*/

	s := gocron.NewScheduler(time.UTC)
	job, err := s.Every(10).Minutes().Do(func() {
		updated, _ := priceService.updatePrices()
		if updated {
			log.Printf("Prices updated!")
			sendNotification(ctx, haClient, "Prices updated!", notifyDevice)
		}
	})
	if err != nil {
		log.Fatalf("error setting up cron: %v", err)
	}
	s.StartAsync()

	currentAmps := 5

	td := &Tesla{
		Command: "CHARGING_AMPS",
		Parameters: &Parameters{
			PathVars: &PathVars{
				VehicleID: id,
			},
			ChargingAmps: currentAmps,
		},
	}

	haClient.CallService(ctx, "tesla_custom", "api", td)

	sendNotification(ctx, haClient, "Electricity starting...", notifyDevice)

MainLoop:
	for {
		select {
		case <-ctx.Done():
			break MainLoop
		case message, ok := <-haClient.EventChannel:
			if ok {
				currentExport := parse(message.Event.Data.NewState.State)
				log.Printf("Event received %v\n", currentExport)
				if currentExport > 0.3 && currentAmps < 13 {
					currentAmps = currentAmps + 1
					//updateAmps(ctx, haClient, currentAmps, id)
				}

				if currentExport < 0.05 && currentAmps > 5 {
					currentAmps = currentAmps - 1
					//updateAmps(ctx, haClient, currentAmps, id)
				}
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

func parse(fs interface{}) float64 {
	ff, err := strconv.ParseFloat(fmt.Sprintf("%v", fs), 64)
	if err != nil {
		return 0
	}

	return ff
}

func updateAmps(ctx context.Context, haClient *gohaws.HaClient, amps int, id string) {
	td := &Tesla{
		Command: "CHARGING_AMPS",
		Parameters: &Parameters{
			PathVars: &PathVars{
				VehicleID: id,
			},
			ChargingAmps: amps,
		},
	}
	log.Printf("Updating charging amps, new value %v\n", amps)
	haClient.CallService(ctx, "tesla_custom", "api", td)
}

func sendNotification(ctx context.Context, haClient *gohaws.HaClient, message string, device string) {
	/*data := &Notification{
		Title:   "Electricity",
		Message: message,
	}*/
	sd := map[string]string{"title": "Electricity", "message": message}
	haClient.CallService(ctx, "notify", device, sd)

}
