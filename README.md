# Raspberry Pi MQTT Vehicle Telemetry Client

A production-style Go MQTT telemetry client intended to run on a Raspberry Pi and publish mock vehicle data to a cloud MQTT broker in real time.

The code is organized so the mock generator can later be replaced with real Raspberry Pi sensor readers and YOLO-based accident detection without changing the MQTT publishing layer.

## Architecture

```text
cmd/client/main.go              Process entrypoint, env config, signals, publish loop
internal/mqtt/client.go         Eclipse Paho MQTT wrapper, reconnects, QoS 1 publishing
internal/telemetry/generator.go Mock vehicle movement and accident event generation
internal/models/telemetry.go    JSON telemetry schema
```

## Features

- Eclipse Paho MQTT Go client library
- QoS 1 publishing
- Clean session disabled for durable client sessions
- Automatic reconnect and connect retry enabled
- JSON structured logs with `log/slog`
- Graceful shutdown on `SIGINT` and `SIGTERM`
- Telemetry published every second
- Mock speed, heading, GPS drift, acceleration, and RPM generation
- Random accident events every 30-60 seconds

## Installation

1. Install Go 1.21 or newer.
2. Clone the repository.
3. Download dependencies:

```bash
go mod download
```

4. Build the client:

```bash
go build -o bin/telemetry-client ./cmd/client
```

## Configuration

Copy the sample environment file and update values for your broker:

```bash
cp .env.example .env
```

Environment variables:

| Variable | Description | Example |
| --- | --- | --- |
| `MQTT_BROKER` | MQTT broker URL | `tcp://localhost:1883` |
| `MQTT_USERNAME` | Broker username, if required | `demo-user` |
| `MQTT_PASSWORD` | Broker password, if required | `demo-password` |
| `MQTT_CLIENT_ID` | Stable MQTT client ID | `raspberry-pi-vehicle-001` |
| `MQTT_TOPIC` | Regular telemetry topic | `vehicle/vehicle-001/telemetry` |

## Running locally

Load the environment and run the client:

```bash
set -a
source .env
set +a
go run ./cmd/client
```

The client logs JSON messages to stdout and publishes one telemetry payload per second.

## Connecting to Mosquitto

The client is broker-agnostic, but the examples below use Eclipse Mosquitto.

### Local Mosquitto broker

Install Mosquitto and the command-line clients:

```bash
# Debian / Ubuntu / Raspberry Pi OS
sudo apt-get update
sudo apt-get install -y mosquitto mosquitto-clients
```

Start Mosquitto locally if your system service is not already running:

```bash
mosquitto -p 1883 -v
```

Use this local configuration:

```env
MQTT_BROKER=tcp://localhost:1883
MQTT_USERNAME=
MQTT_PASSWORD=
MQTT_CLIENT_ID=raspberry-pi-vehicle-001
MQTT_TOPIC=vehicle/vehicle-001/telemetry
```

Subscribe to all messages for the vehicle with `mosquitto_sub`:

```bash
mosquitto_sub -h localhost -p 1883 -t 'vehicle/vehicle-001/#' -v
```

Then start the Go client in another terminal:

```bash
go run ./cmd/client
```

### Public Mosquitto test broker

For a quick remote test, point the client at the public Mosquitto test broker:

```env
MQTT_BROKER=tcp://test.mosquitto.org:1883
MQTT_USERNAME=
MQTT_PASSWORD=
MQTT_CLIENT_ID=raspberry-pi-vehicle-001
MQTT_TOPIC=vehicle/vehicle-001/telemetry
```

Subscribe with:

```bash
mosquitto_sub -h test.mosquitto.org -p 1883 -t 'vehicle/vehicle-001/#' -v
```

Because the public broker is shared, use a unique `MQTT_CLIENT_ID` and topic prefix for real testing.

## Example MQTT topics

Regular telemetry:

```text
vehicle/vehicle-001/telemetry
```

High-priority accident event:

```text
vehicle/vehicle-001/accident
```

## Example telemetry payload

```json
{
  "vehicle_id": "vehicle-001",
  "timestamp": "2026-06-14T12:00:00Z",
  "speed": 72.5,
  "latitude": -1.286389,
  "longitude": 36.817223,
  "acceleration": 0.45,
  "heading": 180.0,
  "engine_rpm": 2500,
  "accident_detected": false
}
```

## Example accident payload

Accident payloads use the same schema, but set `accident_detected` to `true` and are published immediately to `vehicle/vehicle-001/accident`.

```json
{
  "vehicle_id": "vehicle-001",
  "timestamp": "2026-06-14T12:00:45Z",
  "speed": 68.2,
  "latitude": -1.286512,
  "longitude": 36.816921,
  "acceleration": -1.35,
  "heading": 184.1,
  "engine_rpm": 3150,
  "accident_detected": true
}
```

## Raspberry Pi extension path

- Replace `internal/telemetry/generator.go` with sensor-backed GPS, accelerometer, speed, and RPM readers.
- Feed YOLO accident detection events into the same publish loop or expose an accident event channel.
- Keep `internal/models` stable so downstream cloud consumers do not need to change.
- Keep `internal/mqtt` responsible only for broker connectivity and message delivery.
