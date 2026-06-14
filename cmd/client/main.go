package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

	cfg := mqttclient.Config{
		Broker:   os.Getenv("MQTT_BROKER"),
		Username: os.Getenv("MQTT_USERNAME"),
		Password: os.Getenv("MQTT_PASSWORD"),
		ClientID: getenv("MQTT_CLIENT_ID", "raspberry-pi-vehicle-001"),
	}
	telemetryTopic := getenv("MQTT_TOPIC", "vehicle/"+vehicleID+"/telemetry")
	accidentTopic := "vehicle/" + vehicleID + "/accident"

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

	generator := telemetry.NewGenerator()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	logger.Info("telemetry client started", "telemetry_topic", telemetryTopic, "accident_topic", accidentTopic)
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
