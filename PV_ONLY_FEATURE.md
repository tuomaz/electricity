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

#### Start Condition (PV-Only Mode)
- The charger is currently **OFF**.
- **All three phases** must show an export of at least **6A** (the charger's minimum):
    - `Phase 1 Export >= 6A` AND `Phase 2 Export >= 6A` AND `Phase 3 Export >= 6A`.
- This condition must be met for a sustained period of **5 minutes** to ensure steady solar production before starting.

#### Dynamic Adjustment (Optimization)
- While charging in PV-Only mode:
    - The goal is to keep **Import at 0A** on all phases.
    - The charging current is limited by the **least exporting phase**.
    - `Available Amps = min(Phase 1 Export, Phase 2 Export, Phase 3 Export) + Current Charging Amps`.
    - The PID controller will target keeping the surplus as close to 0A as possible without crossing into import.

#### Stop Condition (PV-Only Mode)
- The charger is currently **ON** and at its minimum setting (**6A**).
- If **any phase** export drops below a safety threshold (e.g., **0.5A**) for a sustained period of **5 minutes**, the charger is turned **OFF**.

## Technical Implementation Steps
1. **HA Service**: Update to subscribe to the `PV_ONLY_SWITCH` and the three export sensors.
2. **Power Service**: 
    - Process the new export sensor data.
    - Emit events containing export values for all three phases.
3. **Dawn Consumer Service**:
    - Track the state of the `PV_ONLY_SWITCH`.
    - Implement the logic to switch between "Fuse Protection" (Setpoint 20A) and "PV-Only" (Setpoint 0A surplus).
    - Implement the "All-Phases-Surplus" start/stop timers.
