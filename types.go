package main

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
