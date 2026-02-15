# Electricity Management Service

This application manages residential electricity consumption by dynamically adjusting EV charging load based on real-time phase current monitoring.

## Core Functionality

### Load Balancing (Dawn Charger)
The primary purpose is to protect main fuses while maximizing EV charging speed. It monitors three-phase current sensors via Home Assistant and adjusts the **Dawn EV charger** accordingly.

**Control Logic (Hybrid PID + Safety Override):**
- **Max Phase Current:** Configured to **20A**.
- **Safety Layer:** Instant response if current exceeds 20A.
- **Emergency Stop:** If overcurrent persists for more than **10 seconds** while the charger is already at its minimum (**6A**), the service will turn off the charger via the configured `DAWN_SWITCH`.
- **Restart Logic:** The charger will only restart once there is at least **8A** of headroom available on the most loaded phase (e.g., max phase current drops below 12A).
- **PID Optimization:** A PID controller manages charging when within safe limits:
    - **Proportional (Kp=0.5):** Immediate small adjustments to errors.
    - **Integral (Ki=0.05):** Slowly builds up headroom to allow for increases, acting as a natural, math-based cooldown.
    - **Derivative (Kd=0.1):** Dampens the response to prevent oscillations from spikey household loads.
- **Hysteresis:** Internal floating-point tracking ensures commands are only sent to HA when an integer boundary is crossed.
- **Range:** Charging is maintained within the standard **6A to 16A** range.

### Price Monitoring
The service periodically (every 30 minutes) fetches electricity prices from **Nordpool**. 
- *Note: While prices are fetched and stored, they are currently unused in the charging logic. This provides a foundation for future "Smart Charging" (charging only during low-price hours).*

## Architecture

- **`main.go`**: Orchestrates the services and contains the environment configuration.
- **`ha.go`**: WebSocket client for Home Assistant. Handles real-time event subscriptions and service calls.
- **`power.go`**: Processes phase current updates and detects overcurrent events.
- **`consumer_dawn.go`**: Implements the load balancing algorithm for the Dawn charger.
- **`prices.go`**: Fetches and manages Nordpool electricity price data.

## Configuration (Environment Variables)

| Variable | Description |
|----------|-------------|
| `HAURI` | Home Assistant WebSocket URI (e.g., `ws://homeassistant.local:8123/api/websocket`) |
| `HATOKEN` | Home Assistant Long-Lived Access Token |
| `AREA` | Nordpool Price Area (e.g., `SE2`) |
| `DAWN` | Home Assistant Entity ID for the Dawn charger's current setting (e.g., `number.dawn_amps`) |
| `DAWN_SWITCH` | Home Assistant Entity ID for the Dawn charger's on/off switch (e.g., `switch.dawn_charging`) |
| `DAWN_CURRENT` | HA Entity ID for the actual charging current sensor (e.g., `sensor.dawn_actual_current`) |
| `NOTIFY_DEVICE` | Home Assistant notification service name (e.g., `mobile_app_my_phone`) |

## Technical Stack
- **Language:** Go
- **Integrations:** 
    - Home Assistant (via `gohaws`)
    - Nordpool API
- **Scheduling:** `gocron` for periodic price updates.
