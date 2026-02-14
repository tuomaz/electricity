package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tuomaz/gohaws"
)

func newHaService(ctx context.Context, uri string, token string) *haService {
	ha := &haService{
		uri:     uri,
		token:   token,
		context: ctx,
	}
	go ha.manageConnection()
	return ha
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

func (ha *haService) manageConnection() {
	for {
		select {
		case <-ha.context.Done():
			return
		default:
			log.Printf("HA service: attempting to connect to %s", ha.uri)
			client, err := gohaws.New(ha.context, ha.uri, ha.token)
			if err != nil {
				log.Printf("HA service: failed to create client: %v, retrying in 5 seconds...", err)
				time.Sleep(5 * time.Second)
				continue
			}
			ha.client = client

			// Re-subscribe existing entities if this is a reconnection
			for _, sub := range ha.subscriptions {
				for _, entity := range sub.entities {
					log.Printf("HA service: re-subscribing to %s", entity)
					ha.client.Add(entity)
				}
			}

			// Run the listener. run() will return if connection is lost.
			ha.run()

			log.Printf("HA service: connection lost, retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (ha *haService) subscribe(entity string, channel chan *gohaws.Message) {
	ha.subscribeMulti([]string{entity}, channel)
}

func (ha *haService) subscribeMulti(entities []string, channel chan *gohaws.Message) {
	for _, entity := range entities {
		// Add to internal tracking for reconnection
		found := false
		for _, sub := range ha.subscriptions {
			if sub.channel == channel {
				sub.entities = append(sub.entities, entity)
				found = true
				break
			}
		}

		if !found {
			sub := &subscription{
				channel:  channel,
				entities: []string{entity},
			}
			ha.subscriptions = append(ha.subscriptions, sub)
		}

		// Add to active client if it exists
		if ha.client != nil {
			log.Printf("HA service: adding entity %s to active client", entity)
			ha.client.Add(entity)
		}
	}
}

func (ha *haService) updateAmpsDawn(amps int, dawnID string) {
	if ha.client == nil {
		return
	}
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
	if ha.client == nil {
		return
	}
	service := "turn_off"
	if on {
		service = "turn_on"
	}
	log.Printf("HA service: setting Dawn switch %s to %v", switchID, on)
	ha.client.CallService(ha.context, "switch", service, nil, switchID)
}

func (ha *haService) sendNotification(message string, device string) {
	if ha.client == nil {
		return
	}
	sd := map[string]string{"title": "Electricity", "message": message}
	ha.client.CallService(ha.context, "notify", device, sd, "")
}

func (ha *haService) run() {
	log.Printf("HA service: start listening to message from HA")
	err := ha.client.SubscribeToUpdates(ha.context)
	if err != nil {
		log.Printf("HA service: failed to subscribe: %v", err)
		return
	}

Loop:
	for {
		select {
		case <-ha.context.Done():
			break Loop
		case message, ok := <-ha.client.EventChannel:
			if !ok {
				log.Printf("HA service: event channel closed")
				break Loop
			}
			if message.Event.Data != nil {
				ha.sendEventToSubscribers(message)
			}
		}
	}
	log.Printf("HA service: listener stopped")
}

func (ha *haService) sendEventToSubscribers(message *gohaws.Message) {
	for _, sub := range ha.subscriptions {
		for _, entity := range sub.entities {
			if message.Event.Data.EntityID == entity {
				// Non-blocking send to avoid hanging the HA listener if a service is slow
				select {
				case sub.channel <- message:
				default:
					//log.Printf("HA service: warning: subscriber channel full, dropping message for %s", entity)
				}
			}
		}
	}
}
