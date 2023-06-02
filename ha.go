package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/tuomaz/gohaws"
)

/*
haClient.Add("sensor.momentary_active_export_phase_1")
	haClient.SubscribeToUpdates(ctx)
*/

func newHaService(ctx context.Context, uri string, token string) *haService {
	client := gohaws.New(ctx, uri, token)
	haService := &haService{client: client, uri: uri, token: token, context: ctx}
	go haService.run()
	return haService
}

type haService struct {
	context       context.Context
	client        *gohaws.HaClient
	uri           string
	token         string
	eventChannel  chan event
	subscriptions []subscription
}

type subscription struct {
	entities []string
	channel  chan *gohaws.Message
}

func (ha *haService) subscribe(enitity string, channel chan *gohaws.Message) {
	ha.subscribeMulti([]string{enitity}, channel)
}

func (ha *haService) subscribeMulti(entities []string, channel chan *gohaws.Message) {
	for _, entity := range entities {
		ha.client.Add(entity)
		found := false
		for _, subscription := range ha.subscriptions {
			if subscription.channel == channel {
				log.Printf("Added entity " + entity + " to subscription, existing channel")
				subscription.entities = append(subscription.entities, entity)
				found = true
			}
		}

		if !found {
			subscription := &subscription{
				channel:  channel,
				entities: make([]string, 0),
			}
			subscription.entities = append(subscription.entities, entity)
			ha.subscriptions = append(ha.subscriptions, *subscription)
			log.Printf("Added entity " + entity + " to subscription, new channel")
		}
	}
}

func (ha *haService) updateAmps(amps int, id string) {
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
	ha.client.CallService(ha.context, "tesla_custom", "api", td)
}

func (ha *haService) sendNotification(message string, device string) {
	/*data := &Notification{
		Title:   "Electricity",
		Message: message,
	}*/
	sd := map[string]string{"title": "Electricity", "message": message}
	ha.client.CallService(ha.context, "notify", device, sd)
}

func (ha *haService) run() {
	log.Printf("Start listening to message from HA")
	ha.client.SubscribeToUpdates(ha.context)
Loop:
	for {
		select {
		case <-ha.context.Done():
			break Loop
		case message, ok := <-ha.client.EventChannel:
			if ok {
				currentExport := ha.parse(message.Event.Data.NewState.State)
				log.Printf("Event received %v\n", currentExport)
			} else {
				break Loop
			}
		}
	}
	log.Printf("Stop listening to message from HA")
}

func (ha *haService) parse(fs interface{}) float64 {
	ff, err := strconv.ParseFloat(fmt.Sprintf("%v", fs), 64)
	if err != nil {
		return 0
	}

	return ff
}

/*
	if currentExport > 0.3 && currentAmps < 13 {
		currentAmps = currentAmps + 1
		//updateAmps(ctx, haClient, currentAmps, id)
	}

	if currentExport < 0.05 && currentAmps > 5 {
		currentAmps = currentAmps - 1
		//updateAmps(ctx, haClient, currentAmps, id)
	}

*/
