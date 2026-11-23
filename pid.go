package main

import (
	"math"
	"time"
)

// PIDController handles the math for smooth load balancing
type PIDController struct {
	Kp, Ki, Kd float64
	Setpoint   float64
	Integral   float64
	LastError  float64
	LastTime   time.Time
}

func (p *PIDController) Reset() {
	p.Integral = 0
	p.LastError = 0
	p.LastTime = time.Now()
}

func (p *PIDController) Update(measurement float64) float64 {
	now := time.Now()
	if p.LastTime.IsZero() {
		p.LastTime = now
		return 0
	}

	dt := now.Sub(p.LastTime).Seconds()
	// Prevent division by zero or near-zero which causes derivative explosion
	if dt < 0.1 {
		return 0
	}

	error := p.Setpoint - measurement

	// Proportional term
	P := p.Kp * error

	// Integral term (with basic anti-windup: only accumulate if error is significant,
	// and cap effective dt to 35s so lockouts/pauses don't cause an instant integral spike)
	if math.Abs(error) > 0.01 {
		integralDt := math.Min(dt, 35.0)
		p.Integral += error * integralDt
	}

	// Clamp integral to prevent massive windup
	if p.Integral > 50 {
		p.Integral = 50
	} else if p.Integral < -50 {
		p.Integral = -50
	}
	I := p.Ki * p.Integral

	// Derivative term
	D := p.Kd * (error - p.LastError) / dt

	p.LastError = error
	p.LastTime = now

	return P + I + D
}
