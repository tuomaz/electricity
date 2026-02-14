package main

import (
	"context"
	"fmt"
	"log"

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
	subscriptions []*subscription
}

type subscription struct {
	entities []string
	channel  chan *gohaws.Message
}

func (ha *haService) subscribe(entity string, channel chan *gohaws.Message) {
	ha.subscribeMulti([]string{entity}, channel)
}

func (ha *haService) subscribeMulti(entities []string, channel chan *gohaws.Message) {
	for _, entity := range entities {
		ha.client.Add(entity)
		found := false
		for _, subscription := range ha.subscriptions {
			if subscription.channel == channel {
				log.Printf("HA service: added entity " + entity + " to subscription, existing channel")
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
			ha.subscriptions = append(ha.subscriptions, subscription)
			log.Printf("HA service: added entity " + entity + " to subscription, new channel")
		}
	}
}

func (ha *haService) updateAmpsDawn(amps int, dawnID string) {
	if amps < 6 {
		amps = 6
	}

	if amps > 16 {
		amps = 16
	}

	data := map[string]string{"value": fmt.Sprintf("%d", amps)}

	ha.client.CallService(ha.context, "number", "set_value", data, dawnID)
}

func (ha *haService) setDawnSwitch(on bool, switchID string) {
	service := "turn_off"
	if on {
		service = "turn_on"
	}
	log.Printf("HA service: setting Dawn switch %s to %v", switchID, on)
	ha.client.CallService(ha.context, "switch", service, nil, switchID)
}

func (ha *haService) sendNotification(message string, device string) {
	/*data := &Notification{
		Title:   "Electricity",
		Message: message,
	}*/
	sd := map[string]string{"title": "Electricity", "message": message}
	ha.client.CallService(ha.context, "notify", device, sd, "")
}

func (ha *haService) run() {
	log.Printf("HA service: start listening to message from HA")
	ha.client.SubscribeToUpdates(ha.context)
Loop:
	for {
		select {
		case <-ha.context.Done():
			break Loop
		case message, ok := <-ha.client.EventChannel:
			if ok {
				//log.Printf("HA service: event received %v %v\n", message.Event.Data.EntityID, message.Event.Data.NewState.State)
				ha.sendEventToSubscribers(message)
			} else {
				break Loop
			}
		}
	}
	log.Printf("HA service: stop listening to message from HA")
}

func (ha *haService) sendEventToSubscribers(message *gohaws.Message) {
	for _, subscription := range ha.subscriptions {
		for _, entity := range subscription.entities {
			if message.Event.Data.EntityID == entity {
				subscription.channel <- message
			}
		}
	}
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
