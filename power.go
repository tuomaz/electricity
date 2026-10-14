package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/tuomaz/gohaws"
)

type PowerService struct {
	ctx      context.Context
	phase1   string
	phase2   string
	phase3   string
	export1  string
	export2  string
	export3  string
	import1  string
	import2  string
	import3  string
	voltage1 string
	voltage2 string
	voltage3 string
	max      float64

	haChannel    chan *gohaws.Message
	eventChannel chan *event
	voltages     map[int]float64
}

func newPowerService(ctx context.Context, eventChannel chan *event, ha *haService, phase1 string, phase2 string, phase3 string, export1 string, export2 string, export3 string, import1 string, import2 string, import3 string, voltage1 string, voltage2 string, voltage3 string, max float64) *PowerService {
	haChannel := make(chan *gohaws.Message)
	ha.subscribeMulti([]string{phase1, phase2, phase3, export1, export2, export3, import1, import2, import3, voltage1, voltage2, voltage3}, haChannel)

	powerService := &PowerService{
		ctx:          ctx,
		eventChannel: eventChannel,
		phase1:       phase1,
		phase2:       phase2,
		phase3:       phase3,
		export1:      export1,
		export2:      export2,
		export3:      export3,
		import1:      import1,
		import2:      import2,
		import3:      import3,
		voltage1:     voltage1,
		voltage2:     voltage2,
		voltage3:     voltage3,
		max:          max,
		haChannel:    haChannel,
		voltages:     make(map[int]float64),
	}

	go powerService.run()

	return powerService
}

func (ps *PowerService) run() {
Loop:
	for {
		select {
		case <-ps.ctx.Done():
			break Loop
		case message, ok := <-ps.haChannel:
			if ok {
				value := parseFloat(message.Event.Data.NewState.State)

				powerEvent := &powerEvent{
					phase: message.Event.Data.EntityID,
					value: value,
				}

				// Map to sensor type and phase index
				recognized := false
				switch message.Event.Data.EntityID {
				case ps.phase1:
					powerEvent.sensorType = SensorTypeCurrent
					powerEvent.phaseIndex = 1
					recognized = true
				case ps.phase2:
					powerEvent.sensorType = SensorTypeCurrent
					powerEvent.phaseIndex = 2
					recognized = true
				case ps.phase3:
					powerEvent.sensorType = SensorTypeCurrent
					powerEvent.phaseIndex = 3
					recognized = true
				case ps.export1:
					powerEvent.sensorType = SensorTypeExport
					powerEvent.phaseIndex = 1
					powerEvent.value = (value * 1000.0) / ps.getVoltage(1)
					recognized = true
				case ps.export2:
					powerEvent.sensorType = SensorTypeExport
					powerEvent.phaseIndex = 2
					powerEvent.value = (value * 1000.0) / ps.getVoltage(2)
					recognized = true
				case ps.export3:
					powerEvent.sensorType = SensorTypeExport
					powerEvent.phaseIndex = 3
					powerEvent.value = (value * 1000.0) / ps.getVoltage(3)
					recognized = true
				case ps.import1:
					powerEvent.sensorType = SensorTypeImport
					powerEvent.phaseIndex = 1
					powerEvent.value = (value * 1000.0) / ps.getVoltage(1)
					recognized = true
				case ps.import2:
					powerEvent.sensorType = SensorTypeImport
					powerEvent.phaseIndex = 2
					powerEvent.value = (value * 1000.0) / ps.getVoltage(2)
					recognized = true
				case ps.import3:
					powerEvent.sensorType = SensorTypeImport
					powerEvent.phaseIndex = 3
					powerEvent.value = (value * 1000.0) / ps.getVoltage(3)
					recognized = true
				case ps.voltage1:
					ps.voltages[1] = value
					powerEvent.sensorType = SensorTypeVoltage
					powerEvent.phaseIndex = 1
					recognized = true
				case ps.voltage2:
					ps.voltages[2] = value
					powerEvent.sensorType = SensorTypeVoltage
					powerEvent.phaseIndex = 2
					recognized = true
				case ps.voltage3:
					ps.voltages[3] = value
					powerEvent.sensorType = SensorTypeVoltage
					powerEvent.phaseIndex = 3
					recognized = true
				}

				if !recognized {
					continue
				}

				if (powerEvent.sensorType == SensorTypeCurrent || powerEvent.sensorType == SensorTypeImport) && powerEvent.value > ps.max {
					log.Printf("POWER: overcurrent! %.2f vs %.2f, phase %s (Type: %d)", powerEvent.value, ps.max, message.Event.Data.EntityID, powerEvent.sensorType)
					powerEvent.overCurrent = powerEvent.value - ps.max
				}


				event := &event{
					powerEvent: powerEvent,
				}

				ps.eventChannel <- event
			} else {
				break Loop
			}
		}
	}
}

func (ps *PowerService) getVoltage(phase int) float64 {
	v, ok := ps.voltages[phase]
	if !ok || v < 100 { // Basic sanity check
		return 230.0
	}
	return v
}

func parseFloat(fs interface{}) float64 {
	ff, err := strconv.ParseFloat(fmt.Sprintf("%v", fs), 64)
	if err != nil {
		return 0
	}

	return ff
}
