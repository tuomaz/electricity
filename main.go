package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

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

	haUri, haToken, id := readEnv()

	haClient := gohaws.New(ctx, haUri, haToken)
	haClient.Add("sensor.momentary_active_export_phase_1")
	haClient.SubscribeToUpdates(ctx)

	currentAmps := 5

	/*
		sd := map[string]string{"title": "t1", "message": "t2"}
		haClient.CallService(ctx, "notify", "mobile_app_fredriks_zenfone_9", sd)
	*/
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

MainLoop:
	for {
		select {
		case <-ctx.Done():
			break MainLoop
		case message, ok := <-haClient.EventChannel:
			if ok {
				currentExport := parse(message.Event.Data.NewState.State)
				log.Printf("Event received %v\n", currentExport)
				if currentExport > 0.3 && currentAmps < 14 {
					currentAmps = currentAmps + 1
					updateAmps(ctx, haClient, currentAmps, id)
				}

				if currentExport < 0.05 && currentAmps > 5 {
					currentAmps = currentAmps - 1
					updateAmps(ctx, haClient, currentAmps, id)
				}
			} else {
				break MainLoop
			}
		}
	}
}

func readEnv() (string, string, string) {
	var haURI, haToken, id string
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

	return haURI, haToken, id
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

	haClient.CallService(ctx, "tesla_custom", "api", td)
}
