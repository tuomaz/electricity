# Feature: Only Charge Using Solar Energy (PV)

This feature allows the service to dynamically adjust the EV charging load to match the available solar surplus (export), ensuring that the car is charged only when there is excess production on all three phases.

## Requirements

### 1. Configuration & Integration
- **`PV_ONLY_SWITCH`**: Home Assistant entity ID (e.g., `input_boolean.pv_only_charging`) used to toggle this mode.
- **Monitoring (Export Sensors - Unit: kW)**:
    - `sensor.momentary_active_export_phase_1`
    - `sensor.momentary_active_export_phase_2`
    - `sensor.momentary_active_export_phase_3`
- **Monitoring (Voltage Sensors - Unit: V)**:
    - `sensor.voltage_phase_1`
    - `sensor.voltage_phase_2`
    - `sensor.voltage_phase_3`
- **Conversion**: Since the charger is controlled in Amps, kW values must be converted:
    - `Amps = (kW * 1000) / Voltage`
    - **Fallback**: If voltage data is unavailable, use **230V**.

### 2. Control Logic

#### Mode Selection
- **PV-Only Mode (Switch ON)**: The charger only operates when there is a solar surplus.
- **Normal Mode (Switch OFF)**: The charger operates based on the maximum fuse limit (20A), as currently implemented.

## Start Condition (PV-Only Mode)
- The charger is currently **OFF**.
- The **net export across all three phases** (sum of exports - sum of imports) must be at least **18A** (assuming 3-phase charging at the minimum 6A):
    - `Net Export >= 18A`.
- This condition must be met for a sustained period of **5 minutes** to ensure steady solar production before starting.

#### Dynamic Adjustment (Optimization)
- While charging in PV-Only mode:
    - The goal is to keep **Net Import at 0A**.
    - The charging current is guided by the **total net surplus**.
    - `Available Amps (per phase) = (Net Export / 3.0) + Current Charging Amps`.
    - The PID controller will target keeping the net surplus as close to 0A as possible without crossing into net import.
    - **Note:** Individual phase limits (20A fuses) still apply as a hard safety override.

#### Stop Condition (PV-Only Mode)
- The charger is currently **ON** and at its minimum setting (**6A**).
- If the system starts **net importing** energy from the grid (i.e., `Net Import > 3.0A` sustained) for a period of **5 minutes**, the charger is turned **OFF**.
- This ensures that if household loads increase or solar production decreases beyond the charger's ability to throttle down, we stop charging to avoid grid costs.

## Technical Implementation Steps
1. **HA Service**: Update to subscribe to the `PV_ONLY_SWITCH` and the three export sensors.
2. **Power Service**: 
    - Process the new export sensor data.
    - Emit events containing export values for all three phases.
3. **Dawn Consumer Service**:
    - Track the state of the `PV_ONLY_SWITCH`.
    - Implement the logic to switch between "Fuse Protection" (Setpoint 20A) and "PV-Only" (Setpoint 0A surplus).
    - Implement the "All-Phases-Surplus" start/stop timers.
