package models

// Telemetry represents a single vehicle telemetry payload published to MQTT.
// The struct tags define the public JSON contract consumed by cloud services.
type Telemetry struct {
	VehicleID        string  `json:"vehicle_id"`
	Timestamp        string  `json:"timestamp"`
	Speed            float64 `json:"speed"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	Acceleration     float64 `json:"acceleration"`
	Heading          float64 `json:"heading"`
	EngineRPM        int     `json:"engine_rpm"`
	AccidentDetected bool    `json:"accident_detected"`
}
