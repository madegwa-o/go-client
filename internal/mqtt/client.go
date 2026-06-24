package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

const publishTimeout = 10 * time.Second

// Config contains all broker settings read by cmd/client from environment
// variables. Keeping configuration at the edge makes this package reusable in
// tests, Raspberry Pi services, and future sensor-backed binaries.
type Config struct {
	Broker             string
	Username           string
	Password           string
	ClientID           string
	CACertPath         string
	InsecureSkipVerify bool
}

// Client wraps the Eclipse Paho client and centralizes MQTT behavior such as
// QoS, reconnect policy, JSON encoding, and publish timeouts.
type Client struct {
	client paho.Client
	logger *slog.Logger
}

// NewClient builds a production-style MQTT client with durable sessions and
// automatic reconnect enabled for unreliable Raspberry Pi network links.
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	if cfg.Broker == "" {
		return nil, fmt.Errorf("MQTT_BROKER is required")
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("MQTT_CLIENT_ID is required")
	}

	tlsConfig, err := newTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second)
	if tlsConfig != nil {
		opts.SetTLSConfig(tlsConfig)
	}

	opts.OnConnect = func(_ paho.Client) {
		logger.Info("mqtt connected", "broker", cfg.Broker, "client_id", cfg.ClientID)
	}
	opts.OnConnectionLost = func(_ paho.Client, err error) {
		logger.Warn("mqtt connection lost", "error", err)
	}
	opts.OnReconnecting = func(_ paho.Client, _ *paho.ClientOptions) {
		logger.Info("mqtt reconnecting", "broker", cfg.Broker)
	}

	return &Client{client: paho.NewClient(opts), logger: logger}, nil
}

func newTLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.CACertPath == "" && !cfg.InsecureSkipVerify {
		return nil, nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify}
	if cfg.CACertPath == "" {
		return tlsConfig, nil
	}

	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}

	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("read MQTT_CA_CERT %q: %w", cfg.CACertPath, err)
	}
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("MQTT_CA_CERT %q does not contain a valid PEM certificate", cfg.CACertPath)
	}
	tlsConfig.RootCAs = certPool

	return tlsConfig, nil
}

// Connect establishes the broker connection and waits until Paho reports a
// successful connection or failure.
func (c *Client) Connect() error {
	token := c.client.Connect()
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// PublishJSON marshals payload as JSON and publishes it with QoS 1.
func (c *Client) PublishJSON(ctx context.Context, topic string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	token := c.client.Publish(topic, 1, false, body)
	if !waitToken(ctx, token) {
		return fmt.Errorf("publish to %s timed out or was canceled", topic)
	}
	if token.Error() != nil {
		return fmt.Errorf("publish to %s: %w", topic, token.Error())
	}

	c.logger.Info("published mqtt message", "topic", topic, "bytes", len(body))
	return nil
}

// Disconnect gracefully flushes in-flight messages before closing the session.
func (c *Client) Disconnect() {
	c.client.Disconnect(uint(publishTimeout / time.Millisecond))
}

func waitToken(ctx context.Context, token paho.Token) bool {
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return false
	case <-time.After(publishTimeout):
		return false
	case <-done:
		return true
	}
}
