package telemetry

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"go-client/internal/models"
)

const (
	defaultVehicleID = "vehicle-001"
	minSpeed         = 0.0
	maxSpeed         = 120.0
)

// GeneratorConfig controls the initial mock telemetry state. On a Raspberry Pi,
// set the starting GPS coordinates close to the vehicle's real location until
// this generator is replaced by sensor-backed readers.
type GeneratorConfig struct {
	VehicleID string
	Latitude  float64
	Longitude float64
}

// Generator owns the mock vehicle state. In production this package can be
// replaced by Raspberry Pi sensor readers and YOLO accident-detection events
// while keeping the MQTT publishing layer unchanged.
type Generator struct {
	mu               sync.Mutex
	rng              *rand.Rand
	vehicleID        string
	speed            float64
	latitude         float64
	longitude        float64
	heading          float64
	lastSpeed        float64
	nextAccidentTime time.Time
}

// NewGenerator creates a generator seeded with the requested starting point.
func NewGenerator(cfg GeneratorConfig) *Generator {
	if cfg.VehicleID == "" {
		cfg.VehicleID = defaultVehicleID
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	g := &Generator{
		rng:       rng,
		vehicleID: cfg.VehicleID,
		speed:     72.5,
		latitude:  cfg.Latitude,
		longitude: cfg.Longitude,
		heading:   180.0,
	}
	g.scheduleNextAccident(time.Now())
	return g
}

// Next returns the next realistic telemetry sample. It gently changes speed,
// heading, and GPS position to simulate continuous travel.
func (g *Generator) Next(now time.Time) models.Telemetry {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastSpeed = g.speed
	g.speed = clamp(g.speed+g.rng.Float64()*12-6, minSpeed, maxSpeed)
	g.heading = math.Mod(g.heading+g.rng.Float64()*8-4+360, 360)

	// Approximate one second of travel using speed in km/h and heading in degrees.
	distanceKm := g.speed / 3600
	headingRad := g.heading * math.Pi / 180
	g.latitude += (distanceKm * math.Cos(headingRad)) / 111.0
	cosLat := math.Cos(g.latitude * math.Pi / 180)
	if math.Abs(cosLat) < 0.0001 {
		cosLat = 0.0001
	}
	g.longitude += (distanceKm * math.Sin(headingRad)) / (111.0 * cosLat)

	acceleration := (g.speed - g.lastSpeed) / 3.6
	return models.Telemetry{
		VehicleID:        g.vehicleID,
		Timestamp:        now.UTC().Format(time.RFC3339),
		Speed:            round(g.speed, 1),
		Latitude:         round(g.latitude, 6),
		Longitude:        round(g.longitude, 6),
		Acceleration:     round(acceleration, 2),
		Heading:          round(g.heading, 1),
		EngineRPM:        engineRPM(g.speed, g.rng),
		AccidentDetected: false,
	}
}

// AccidentDue reports whether an immediate high-priority accident payload
// should be published. Events are randomized every 30 to 60 seconds.
func (g *Generator) AccidentDue(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if now.Before(g.nextAccidentTime) {
		return false
	}
	g.scheduleNextAccident(now)
	return true
}

func (g *Generator) scheduleNextAccident(now time.Time) {
	g.nextAccidentTime = now.Add(time.Duration(30+g.rng.Intn(31)) * time.Second)
}

func engineRPM(speed float64, rng *rand.Rand) int {
	return int(clamp(800+speed*35+rng.Float64()*250-125, 700, 5200))
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func round(value float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}
