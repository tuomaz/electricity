package main

type Notification struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

type Tesla struct {
	Command    string      `json:"command,omitempty"`
	Parameters *Parameters `json:"parameters,omitempty"`
}

type Parameters struct {
	PathVars     *PathVars `json:"path_vars,omitempty"`
	ChargingAmps int       `json:"charging_amps,omitempty"`
}

type PathVars struct {
	VehicleID string `json:"vehicle_id,omitempty"`
}

type powerEvent struct {
	overcurrent float64
}

type priceEvent struct {
}

type event struct {
	powerEvent *powerEvent
	priceEvent *priceEvent
}
