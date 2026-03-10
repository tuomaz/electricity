package main

type Notification struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

type SensorType int

const (
	SensorTypeCurrent SensorType = iota
	SensorTypeExport
	SensorTypeVoltage
)

type powerEvent struct {
	sensorType  SensorType
	phase       string
	value       float64
	overCurrent float64
	phaseIndex  int // 1, 2, or 3
}

type priceEvent struct {
}

type event struct {
	powerEvent *powerEvent
	priceEvent *priceEvent
}
