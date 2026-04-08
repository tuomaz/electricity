package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
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

	haUri, haToken, area, dawn, dawnSwitch, notifyDevice, dawnCurrent, pvOnlySwitchId, phase1, phase2, phase3, export1, export2, export3, voltage1, voltage2, voltage3, priceLimitEntity := readEnv()

	events := make(chan *event)

	haService := newHaService(ctx, haUri, haToken, notifyDevice)
	_ = newPowerService(ctx, events, haService,
		phase1, phase2, phase3,
		export1, export2, export3,
		voltage1, voltage2, voltage3,
		MAX_PHASE_CURRENT)
	priceService := newPriceService(area)
	dawnService := newDawnConsumerService(ctx, events, haService, "sensor.dawn_status_connector", dawn, dawnSwitch, notifyDevice, dawnCurrent, MAX_PHASE_CURRENT, pvOnlySwitchId, priceLimitEntity)

	// TODO: move this inside service
	s := gocron.NewScheduler(time.UTC)
	job, err := s.Every(15).Minutes().Do(func() {
		updated, _ := priceService.updatePrices()
		if updated {
			log.Printf("Prices updated!")
		}

		currentPrice, err := priceService.GetCurrentPrice()
		if err == nil {
			dawnService.UpdatePrice(currentPrice)
		} else {
			log.Printf("Error getting current price: %v", err)
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
					dawnService.updateCurrents(event.powerEvent)
				}
			} else {
				break MainLoop
			}
		}
	}
	log.Printf("End main loop")
	s.Remove(job)
}

func readEnv() (string, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string, string) {
	var haURI, haToken, area, dawn, dawnSwitch, notifyDevice, dawnCurrent, pvOnlySwitch string
	var phase1, phase2, phase3, export1, export2, export3, voltage1, voltage2, voltage3, priceLimitEntity string

	value, ok := os.LookupEnv("HAURI")
	if ok {
		haURI = strings.TrimSpace(value)
	} else {
		log.Fatalf("no Home Assistant URI found")
	}

	value, ok = os.LookupEnv("HATOKEN")
	if ok {
		haToken = strings.TrimSpace(value)
	} else {
		log.Fatalf("no Home Assistant auth token found")
	}

	value, ok = os.LookupEnv("AREA")
	if ok {
		area = strings.TrimSpace(value)
	} else {
		log.Fatalf("no ID found")
	}

	value, ok = os.LookupEnv("DAWN")
	if ok {
		dawn = strings.TrimSpace(value)
	} else {
		log.Fatalf("no Dawn device found")
	}

	value, ok = os.LookupEnv("DAWN_SWITCH")
	if ok {
		dawnSwitch = strings.TrimSpace(value)
	} else {
		log.Fatalf("no Dawn switch found")
	}

	value, ok = os.LookupEnv("NOTIFY_DEVICE")
	if ok {
		notifyDevice = strings.TrimSpace(value)
	} else {
		log.Fatalf("no notify device found")
	}

	value, ok = os.LookupEnv("DAWN_CURRENT")
	if ok {
		dawnCurrent = strings.TrimSpace(value)
	} else {
		log.Fatalf("no Dawn current sensor found")
	}

	value, ok = os.LookupEnv("PV_ONLY_SWITCH")
	if ok {
		pvOnlySwitch = strings.TrimSpace(value)
	} else {
		log.Fatalf("no PV only switch found")
	}

	value, ok = os.LookupEnv("PRICE_LIMIT_ENTITY")
	if ok {
		priceLimitEntity = strings.TrimSpace(value)
	} else {
		log.Fatalf("no price limit entity found")
	}

	// Phase Currents
	phase1 = getEnvOrDefault("PHASE_1_CURRENT", "sensor.current_phase_1")
	phase2 = getEnvOrDefault("PHASE_2_CURRENT", "sensor.current_phase_2")
	phase3 = getEnvOrDefault("PHASE_3_CURRENT", "sensor.current_phase_3")

	// Export Sensors
	export1 = getEnvOrDefault("PHASE_1_EXPORT", "sensor.momentary_active_export_phase_1")
	export2 = getEnvOrDefault("PHASE_2_EXPORT", "sensor.momentary_active_export_phase_2")
	export3 = getEnvOrDefault("PHASE_3_EXPORT", "sensor.momentary_active_export_phase_3")

	// Voltage Sensors
	voltage1 = getEnvOrDefault("PHASE_1_VOLTAGE", "sensor.voltage_phase_1")
	voltage2 = getEnvOrDefault("PHASE_2_VOLTAGE", "sensor.voltage_phase_2")
	voltage3 = getEnvOrDefault("PHASE_3_VOLTAGE", "sensor.voltage_phase_3")

	return haURI, haToken, area, dawn, dawnSwitch, notifyDevice, dawnCurrent, pvOnlySwitch,
		phase1, phase2, phase3, export1, export2, export3, voltage1, voltage2, voltage3, priceLimitEntity
}

func getEnvOrDefault(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	return strings.TrimSpace(value)
}
