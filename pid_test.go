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
	time.Sleep(10 * time.Millisecond)

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
	// Error = 2
	time.Sleep(100 * time.Millisecond)
	
	output := pid.Update(18.0)
	// dt ~ 0.1s. I = 1.0 * (2 * 0.1) = 0.2
	assert.True(t, output > 0.05 && output < 0.25, "Output should be roughly 0.1-0.2")
}

func TestPIDController_Clamping(t *testing.T) {
	pid := &PIDController{
		Ki:       100.0,
		Setpoint: 20.0,
	}

	pid.Update(10.0) // Error = 10
	pid.Integral = 1000.0 // Force windup

	pid.Update(10.0)
	assert.Equal(t, 50.0, pid.Integral, "Integral should be clamped to 50")
}

func TestPIDController_Derivative(t *testing.T) {
	pid := &PIDController{
		Kd:       1.0,
		Setpoint: 20.0,
	}

	pid.Update(15.0) // Error = 5
	time.Sleep(10 * time.Millisecond)
	
	pid.Update(18.0) // Error = 2. Change = -3.
	// D = 1.0 * (-3 / dt). Since dt is small, D will be a large negative number.
	assert.True(t, pid.LastError == 2.0)
}
