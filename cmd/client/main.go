package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mqttclient "go-client/internal/mqtt"
	"go-client/internal/telemetry"
)

const vehicleID = "vehicle-001"

// main wires together the application layers:
//   - environment configuration at the process boundary
//   - telemetry generation for mock Raspberry Pi vehicle data
//   - MQTT publishing for cloud ingestion
//
// The generator is intentionally isolated behind internal/telemetry so future
// real sensors and YOLO accident detection can replace it without changing the
// MQTT client or process lifecycle code.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configuredVehicleID := getenv("VEHICLE_ID", vehicleID)
	cfg := mqttclient.Config{
		Broker:             getenv("MQTT_BROKER", "ssl://c8c2713f.ala.eu-central-1.emqxsl.com:8883"),
		Username:           os.Getenv("MQTT_USERNAME"),
		Password:           os.Getenv("MQTT_PASSWORD"),
		ClientID:           getenv("MQTT_CLIENT_ID", "raspberry-pi-"+configuredVehicleID),
		CACertPath:         os.Getenv("MQTT_CA_CERT"),
		InsecureSkipVerify: getenvBool("MQTT_INSECURE_SKIP_VERIFY", false),
	}
	telemetryTopic := getenv("MQTT_TOPIC", "vehicle/"+configuredVehicleID+"/telemetry")
	accidentTopic := getenv("MQTT_ACCIDENT_TOPIC", "vehicle/"+configuredVehicleID+"/accident")
	publishInterval := getenvDuration("PUBLISH_INTERVAL", time.Second)

	client, err := mqttclient.NewClient(cfg, logger)
	if err != nil {
		logger.Error("invalid mqtt configuration", "error", err)
		os.Exit(1)
	}
	if err := client.Connect(); err != nil {
		logger.Error("failed to connect to mqtt broker", "error", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	generator := telemetry.NewGenerator(telemetry.GeneratorConfig{
		VehicleID: configuredVehicleID,
		Latitude:  getenvFloat("START_LATITUDE", -1.286389),
		Longitude: getenvFloat("START_LONGITUDE", 36.817223),
	})
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()

	logger.Info("telemetry client started", "vehicle_id", configuredVehicleID, "telemetry_topic", telemetryTopic, "accident_topic", accidentTopic, "publish_interval", publishInterval.String())
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received")
			return
		case now := <-ticker.C:
			message := generator.Next(now)
			if generator.AccidentDue(now) {
				message.AccidentDetected = true
				publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := client.PublishJSON(publishCtx, accidentTopic, message); err != nil {
					logger.Error("failed to publish accident message", "error", err, "topic", accidentTopic)
				}
				cancel()
			}

			publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := client.PublishJSON(publishCtx, telemetryTopic, message); err != nil {
				logger.Error("failed to publish telemetry", "error", err, "topic", telemetryTopic)
			}
			cancel()
		}
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
