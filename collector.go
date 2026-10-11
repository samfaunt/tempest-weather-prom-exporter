package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var allDescs []*prometheus.Desc

func newDesc(name, help string, labels ...string) *prometheus.Desc {
	d := prometheus.NewDesc("tempest_"+name, help, labels, nil)
	allDescs = append(allDescs, d)
	return d
}

var (
	descDeviceInfo      = newDesc("device_info", "Station device metadata.", "serial", "hub", "firmware")
	descLastObservation = newDesc("last_observation_timestamp_seconds", "Station timestamp of the latest observation.", "serial")
	descTemperature     = newDesc("air_temperature_celsius", "Air temperature.", "serial")
	descPressure        = newDesc("station_pressure_pascals", "Station (not sea level) pressure.", "serial")
	descHumidity        = newDesc("relative_humidity_ratio", "Relative humidity (0-1).", "serial")
	descWindLull        = newDesc("wind_lull_meters_per_second", "Minimum 3 second wind sample over the report interval.", "serial")
	descWindAvg         = newDesc("wind_avg_meters_per_second", "Average wind speed over the report interval.", "serial")
	descWindGust        = newDesc("wind_gust_meters_per_second", "Maximum 3 second wind sample over the report interval.", "serial")
	descWindDirection   = newDesc("wind_direction_degrees", "Average wind direction over the report interval.", "serial")
	descIlluminance     = newDesc("illuminance_lux", "Illuminance.", "serial")
	descUV              = newDesc("uv_index", "UV index.", "serial")
	descSolarRadiation  = newDesc("solar_radiation_watts_per_square_meter", "Solar radiation.", "serial")
	descRain            = newDesc("rain_millimeters_total", "Rain accumulated from observations since the exporter first saw the device.", "serial")
	descRainDay         = newDesc("rain_day_millimeters", "Local day rain accumulation reported by the device (SKY only).", "serial")
	descPrecipType      = newDesc("precipitation_type", "Current precipitation type (1 for the active type).", "serial", "type")
	descStrikes         = newDesc("lightning_strikes_total", "Lightning strikes counted from observations since the exporter first saw the device.", "serial")
	descStrikeAvgDist   = newDesc("lightning_strike_avg_distance_meters", "Average lightning strike distance over the report interval; absent when no strikes.", "serial")
	descBattery         = newDesc("battery_volts", "Battery voltage from observations.", "serial")

	descRapidWindSpeed     = newDesc("rapid_wind_speed_meters_per_second", "Instantaneous wind speed (rapid_wind, ~3s).", "serial")
	descRapidWindDirection = newDesc("rapid_wind_direction_degrees", "Instantaneous wind direction (rapid_wind, ~3s).", "serial")

	descStrikeDistance     = newDesc("lightning_strike_distance_meters", "Distance of individual lightning strike events.", "serial")
	descLastStrike         = newDesc("lightning_last_strike_timestamp_seconds", "Time of the latest lightning strike event.", "serial")
	descLastStrikeDistance = newDesc("lightning_last_strike_distance_meters", "Distance of the latest lightning strike event.", "serial")
	descLastStrikeEnergy   = newDesc("lightning_last_strike_energy", "Energy of the latest lightning strike event (unitless).", "serial")
	descRainStarts         = newDesc("rain_start_events_total", "Rain start events.", "serial")
	descLastRainStart      = newDesc("rain_last_start_timestamp_seconds", "Time of the latest rain start event.", "serial")

	descDeviceUptime       = newDesc("device_uptime_seconds", "Device uptime.", "serial")
	descDeviceVoltage      = newDesc("device_voltage_volts", "Device voltage from device_status.", "serial")
	descDeviceRSSI         = newDesc("device_rssi_dbm", "Device radio signal strength.", "serial")
	descDeviceHubRSSI      = newDesc("device_hub_rssi_dbm", "Hub radio signal strength as seen by the device.", "serial")
	descSensorFailed       = newDesc("sensor_failed", "1 when the device reports the sensor failed.", "serial", "sensor")
	descLightningNoise     = newDesc("sensor_lightning_noise", "1 when the lightning sensor reports noise.", "serial")
	descLightningDisturber = newDesc("sensor_lightning_disturber", "1 when the lightning sensor reports a disturber.", "serial")
	descPowerBooster       = newDesc("power_booster", "Tempest power booster state flags.", "serial", "state")

	descHubInfo        = newDesc("hub_info", "Hub metadata.", "hub", "firmware", "reset_flags")
	descHubUptime      = newDesc("hub_uptime_seconds", "Hub uptime.", "hub")
	descHubRSSI        = newDesc("hub_rssi_dbm", "Hub WiFi signal strength.", "hub")
	descHubReboots     = newDesc("hub_reboots", "Hub reboot count.", "hub")
	descHubI2CErrors   = newDesc("hub_i2c_bus_errors", "Hub I2C bus error count.", "hub")
	descHubRadioStatus = newDesc("hub_radio_status", "Hub radio status (0 off, 1 on, 3 active, 7 BLE connected).", "hub")
)

var precipTypes = []string{"none", "rain", "hail", "rain_hail"}

// strikeBuckets are lightning distance histogram bounds in meters; the sensor
// reports whole kilometers up to about 40 km.
var strikeBuckets = []float64{5000, 10000, 15000, 20000, 25000, 30000, 35000, 40000}

// sensor_status bits; see "Sensor Status Bit Flags" in the API docs.
const (
	statusLightningFailed    = 1 << 0
	statusLightningNoise     = 1 << 1
	statusLightningDisturber = 1 << 2
	statusPressureFailed     = 1 << 3
	statusTemperatureFailed  = 1 << 4
	statusHumidityFailed     = 1 << 5
	statusWindFailed         = 1 << 6
	statusPrecipFailed       = 1 << 7
	statusLightUVFailed      = 1 << 8
	statusBoosterDetected    = 0x200
	statusBoosterDepleted    = 0x8000
	statusBoosterShorePower  = 0x10000
)

var knownTypes = map[string]bool{
	"obs_st": true, "obs_air": true, "obs_sky": true, "rapid_wind": true,
	"evt_strike": true, "evt_precip": true, "device_status": true, "hub_status": true,
}

type deviceState struct {
	hub      string
	firmware string
	lastSeen time.Time

	obs          *observation
	rainTotal    float64
	strikesTotal float64

	windSpeed, windDirection *float64

	strikeCounts []uint64 // cumulative, parallel to strikeBuckets
	strikeCount  uint64
	strikeSum    float64
	lastStrike   []*float64 // evt array: time, km, energy

	rainStarts    float64
	lastRainStart *float64

	status *message
}

type hubState struct {
	lastSeen time.Time
	status   *message
}

// Collector holds the latest state per device and hub, fed by Handle, and
// exposes it as Prometheus metrics. Devices and hubs silent for longer than
// staleAfter are dropped so dead stations do not report frozen values.
type Collector struct {
	staleAfter time.Duration
	now        func() time.Time

	mu      sync.Mutex
	devices map[string]*deviceState
	hubs    map[string]*hubState

	packets      *prometheus.CounterVec
	decodeErrors *prometheus.CounterVec
}

func NewCollector(staleAfter time.Duration) *Collector {
	return &Collector{
		staleAfter: staleAfter,
		now:        time.Now,
		devices:    map[string]*deviceState{},
		hubs:       map[string]*hubState{},
		packets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tempest_udp_packets_total",
			Help: "UDP packets received by message type.",
		}, []string{"type"}),
		decodeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tempest_udp_decode_errors_total",
			Help: "UDP packets that could not be decoded, by message type.",
		}, []string{"type"}),
	}
}

// Handle decodes one UDP packet and updates state.
func (c *Collector) Handle(packet []byte) error {
	var m message
	if err := json.Unmarshal(packet, &m); err != nil {
		c.decodeErrors.WithLabelValues("invalid").Inc()
		return fmt.Errorf("decode packet: %w", err)
	}
	typ := m.Type
	if !knownTypes[typ] {
		typ = "other" // keeps label cardinality bounded
	}
	c.packets.WithLabelValues(typ).Inc()
	if typ == "other" {
		return nil
	}
	if m.SerialNumber == "" {
		c.decodeErrors.WithLabelValues(typ).Inc()
		return errors.New(typ + ": missing serial_number")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()

	if typ == "hub_status" {
		c.hubs[m.SerialNumber] = &hubState{lastSeen: now, status: &m}
		return nil
	}

	d := c.devices[m.SerialNumber]
	if d == nil {
		d = &deviceState{strikeCounts: make([]uint64, len(strikeBuckets))}
		c.devices[m.SerialNumber] = d
	}
	d.lastSeen = now
	d.hub = m.HubSN
	if m.FirmwareRevision != "" {
		d.firmware = string(m.FirmwareRevision)
	}

	switch typ {
	case "obs_st":
		for _, row := range m.Obs {
			d.observe(decodeObsST(row))
		}
	case "obs_air":
		for _, row := range m.Obs {
			d.observe(decodeObsAir(row))
		}
	case "obs_sky":
		for _, row := range m.Obs {
			d.observe(decodeObsSky(row))
		}
	case "rapid_wind":
		d.windSpeed, d.windDirection = at(m.Ob, 1), at(m.Ob, 2)
	case "evt_strike":
		d.strike(m.Evt)
	case "evt_precip":
		d.rainStarts++
		d.lastRainStart = at(m.Evt, 0)
	case "device_status":
		d.status = &m
	}
	return nil
}

// observe records an observation, ignoring rows without a timestamp and
// rows not newer than the last one so counters are never added to twice.
func (d *deviceState) observe(o observation) {
	if o.time == nil || (d.obs != nil && *o.time <= *d.obs.time) {
		return
	}
	if o.rain != nil {
		d.rainTotal += *o.rain
	}
	if o.strikeCount != nil {
		d.strikesTotal += *o.strikeCount
	}
	d.obs = &o
}

func (d *deviceState) strike(evt []*float64) {
	d.lastStrike = evt
	km := at(evt, 1)
	if km == nil {
		return
	}
	m := *km * 1000
	for i, bound := range strikeBuckets {
		if m <= bound {
			d.strikeCounts[i]++
		}
	}
	d.strikeCount++
	d.strikeSum += m
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
	c.packets.Describe(ch)
	c.decodeErrors.Describe(ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for serial, d := range c.devices {
		if now.Sub(d.lastSeen) > c.staleAfter {
			delete(c.devices, serial)
			continue
		}
		d.collect(ch, serial)
	}
	for serial, h := range c.hubs {
		if now.Sub(h.lastSeen) > c.staleAfter {
			delete(c.hubs, serial)
			continue
		}
		h.collect(ch, serial)
	}
	c.packets.Collect(ch)
	c.decodeErrors.Collect(ch)
}

func gauge(ch chan<- prometheus.Metric, d *prometheus.Desc, v *float64, labels ...string) {
	if v != nil {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, *v, labels...)
	}
}

func scaled(v *float64, factor float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v * factor
	return &x
}

func bitSet(status, bit uint64) float64 {
	if status&bit != 0 {
		return 1
	}
	return 0
}

func (d *deviceState) collect(ch chan<- prometheus.Metric, serial string) {
	ch <- prometheus.MustNewConstMetric(descDeviceInfo, prometheus.GaugeValue, 1, serial, d.hub, d.firmware)

	if o := d.obs; o != nil {
		gauge(ch, descLastObservation, o.time, serial)
		gauge(ch, descTemperature, o.temperature, serial)
		gauge(ch, descPressure, scaled(o.pressure, 100), serial)
		gauge(ch, descHumidity, scaled(o.humidity, 0.01), serial)
		gauge(ch, descWindLull, o.windLull, serial)
		gauge(ch, descWindAvg, o.windAvg, serial)
		gauge(ch, descWindGust, o.windGust, serial)
		gauge(ch, descWindDirection, o.windDirection, serial)
		gauge(ch, descIlluminance, o.illuminance, serial)
		gauge(ch, descUV, o.uv, serial)
		gauge(ch, descSolarRadiation, o.solarRadiation, serial)
		gauge(ch, descRainDay, o.rainDay, serial)
		gauge(ch, descBattery, o.battery, serial)
		if o.rain != nil {
			ch <- prometheus.MustNewConstMetric(descRain, prometheus.CounterValue, d.rainTotal, serial)
		}
		if o.strikeCount != nil {
			ch <- prometheus.MustNewConstMetric(descStrikes, prometheus.CounterValue, d.strikesTotal, serial)
			// The device reports distance 0 when there were no strikes.
			if *o.strikeCount > 0 {
				gauge(ch, descStrikeAvgDist, scaled(o.strikeDistance, 1000), serial)
			}
		}
		if o.precipType != nil {
			for i, name := range precipTypes {
				v := 0.0
				if int(*o.precipType) == i {
					v = 1
				}
				ch <- prometheus.MustNewConstMetric(descPrecipType, prometheus.GaugeValue, v, serial, name)
			}
		}
	}

	gauge(ch, descRapidWindSpeed, d.windSpeed, serial)
	gauge(ch, descRapidWindDirection, d.windDirection, serial)

	if d.lastStrike != nil {
		buckets := make(map[float64]uint64, len(strikeBuckets))
		for i, bound := range strikeBuckets {
			buckets[bound] = d.strikeCounts[i]
		}
		ch <- prometheus.MustNewConstHistogram(descStrikeDistance, d.strikeCount, d.strikeSum, buckets, serial)
		gauge(ch, descLastStrike, at(d.lastStrike, 0), serial)
		gauge(ch, descLastStrikeDistance, scaled(at(d.lastStrike, 1), 1000), serial)
		gauge(ch, descLastStrikeEnergy, at(d.lastStrike, 2), serial)
	}

	if d.lastRainStart != nil {
		ch <- prometheus.MustNewConstMetric(descRainStarts, prometheus.CounterValue, d.rainStarts, serial)
		gauge(ch, descLastRainStart, d.lastRainStart, serial)
	}

	if s := d.status; s != nil {
		gauge(ch, descDeviceUptime, s.Uptime, serial)
		gauge(ch, descDeviceVoltage, s.Voltage, serial)
		gauge(ch, descDeviceRSSI, s.RSSI, serial)
		gauge(ch, descDeviceHubRSSI, s.HubRSSI, serial)
		if s.SensorStatus != nil {
			collectSensorStatus(ch, serial, *s.SensorStatus)
		}
	}
}

// collectSensorStatus decodes sensor_status, emitting only the flags that
// apply to the device type given by its serial prefix (AR- AIR, SK- SKY,
// otherwise Tempest).
func collectSensorStatus(ch chan<- prometheus.Metric, serial string, status uint64) {
	air := !strings.HasPrefix(serial, "SK-")
	sky := !strings.HasPrefix(serial, "AR-")
	tempest := air && sky

	failed := func(sensor string, bit uint64) {
		ch <- prometheus.MustNewConstMetric(descSensorFailed, prometheus.GaugeValue, bitSet(status, bit), serial, sensor)
	}
	if air {
		failed("lightning", statusLightningFailed)
		failed("pressure", statusPressureFailed)
		failed("temperature", statusTemperatureFailed)
		failed("humidity", statusHumidityFailed)
		ch <- prometheus.MustNewConstMetric(descLightningNoise, prometheus.GaugeValue, bitSet(status, statusLightningNoise), serial)
		ch <- prometheus.MustNewConstMetric(descLightningDisturber, prometheus.GaugeValue, bitSet(status, statusLightningDisturber), serial)
	}
	if sky {
		failed("wind", statusWindFailed)
		failed("precip", statusPrecipFailed)
		failed("light_uv", statusLightUVFailed)
	}
	if tempest {
		booster := func(state string, bit uint64) {
			ch <- prometheus.MustNewConstMetric(descPowerBooster, prometheus.GaugeValue, bitSet(status, bit), serial, state)
		}
		booster("detected", statusBoosterDetected)
		booster("depleted", statusBoosterDepleted)
		booster("shore_power", statusBoosterShorePower)
	}
}

func (h *hubState) collect(ch chan<- prometheus.Metric, serial string) {
	s := h.status
	ch <- prometheus.MustNewConstMetric(descHubInfo, prometheus.GaugeValue, 1, serial, string(s.FirmwareRevision), s.ResetFlags)
	gauge(ch, descHubUptime, s.Uptime, serial)
	gauge(ch, descHubRSSI, s.RSSI, serial)
	gauge(ch, descHubReboots, at(s.RadioStats, 1), serial)
	gauge(ch, descHubI2CErrors, at(s.RadioStats, 2), serial)
	gauge(ch, descHubRadioStatus, at(s.RadioStats, 3), serial)
}
