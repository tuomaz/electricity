package main

type Notification struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

type powerEvent struct {
	phase       string
	overCurrent float64
	current     float64
}

type priceEvent struct {
}

type event struct {
	powerEvent *powerEvent
	priceEvent *priceEvent
}
