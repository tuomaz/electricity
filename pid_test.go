package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPIDController_Proportional(t *testing.T) {
	pid := &PIDController{
		Kp:       1.0,
		Setpoint: 20.0,
	}

	// First update sets the time
	pid.Update(15.0)
	time.Sleep(150 * time.Millisecond) // Need > 100ms for our new floor

	// Second update should trigger P
	// Error = 20 - 15 = 5. Output = 1.0 * 5 = 5
	output := pid.Update(15.0)
	assert.Equal(t, 5.0, output)
}

func TestPIDController_Integral(t *testing.T) {
	pid := &PIDController{
		Ki:       1.0,
		Setpoint: 20.0,
	}

	pid.Update(18.0)
	time.Sleep(200 * time.Millisecond)
	
	output := pid.Update(18.0)
	// dt ~ 0.2s. I = 1.0 * (2 * 0.2) = 0.4
	assert.True(t, output > 0.1 && output < 0.6, "Output should be roughly 0.4")
}

func TestPIDController_Clamping(t *testing.T) {
	pid := &PIDController{
		Ki:       100.0,
		Setpoint: 20.0,
	}

	pid.Update(10.0) // sets time
	time.Sleep(150 * time.Millisecond)
	
	pid.Integral = 1000.0 // Force windup
	pid.Update(10.0)
	assert.Equal(t, 50.0, pid.Integral, "Integral should be clamped to 50")
}

func TestPIDController_Derivative(t *testing.T) {
	pid := &PIDController{
		Kd:       1.0,
		Setpoint: 20.0,
	}

	pid.Update(15.0) // sets time
	time.Sleep(150 * time.Millisecond)
	
	pid.Update(18.0) 
	assert.True(t, pid.LastError == 2.0)
}
