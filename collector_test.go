package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newTestCollector returns a collector with a controllable clock.
func newTestCollector(t *testing.T) (*Collector, *time.Time) {
	t.Helper()
	now := time.Unix(1_700_000_000, 0)
	c := NewCollector(5 * time.Minute)
	c.now = func() time.Time { return now }
	return c, &now
}

func handle(t *testing.T, c *Collector, packets ...string) {
	t.Helper()
	for _, p := range packets {
		if err := c.Handle([]byte(p)); err != nil {
			t.Fatalf("Handle(%s): %v", p, err)
		}
	}
}

func loadSamples(t *testing.T, c *Collector) {
	t.Helper()
	f, err := os.Open("testdata/sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		handle(t, c, s.Text())
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSamples(t *testing.T) {
	c, _ := newTestCollector(t)
	loadSamples(t, c)

	problems, err := testutil.CollectAndLint(c)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, p := range problems {
		if strings.HasPrefix(p.Metric, "tempest_rain_") && strings.Contains(p.Text, "millimeters") {
			continue // rain stays in mm by design, the unit every weather tool uses
		}
		t.Errorf("lint %s: %s", p.Metric, p.Text)
	}

	want := `
# HELP tempest_air_temperature_celsius Air temperature.
# TYPE tempest_air_temperature_celsius gauge
tempest_air_temperature_celsius{serial="AR-00004049"} 10
tempest_air_temperature_celsius{serial="ST-00000512"} 22.37
# HELP tempest_station_pressure_pascals Station (not sea level) pressure.
# TYPE tempest_station_pressure_pascals gauge
tempest_station_pressure_pascals{serial="AR-00004049"} 83500
tempest_station_pressure_pascals{serial="ST-00000512"} 101757
# HELP tempest_rain_millimeters_total Rain accumulated from observations since the exporter first saw the device.
# TYPE tempest_rain_millimeters_total counter
tempest_rain_millimeters_total{serial="SK-00008453"} 0
tempest_rain_millimeters_total{serial="ST-00000512"} 0
# HELP tempest_precipitation_type Current precipitation type (1 for the active type).
# TYPE tempest_precipitation_type gauge
tempest_precipitation_type{serial="SK-00008453",type="hail"} 0
tempest_precipitation_type{serial="SK-00008453",type="none"} 1
tempest_precipitation_type{serial="SK-00008453",type="rain"} 0
tempest_precipitation_type{serial="SK-00008453",type="rain_hail"} 0
tempest_precipitation_type{serial="ST-00000512",type="hail"} 0
tempest_precipitation_type{serial="ST-00000512",type="none"} 1
tempest_precipitation_type{serial="ST-00000512",type="rain"} 0
tempest_precipitation_type{serial="ST-00000512",type="rain_hail"} 0
# HELP tempest_rapid_wind_speed_meters_per_second Instantaneous wind speed (rapid_wind, ~3s).
# TYPE tempest_rapid_wind_speed_meters_per_second gauge
tempest_rapid_wind_speed_meters_per_second{serial="SK-00008453"} 2.3
# HELP tempest_lightning_strike_distance_meters Distance of individual lightning strike events.
# TYPE tempest_lightning_strike_distance_meters histogram
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="5000"} 0
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="10000"} 0
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="15000"} 0
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="20000"} 0
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="25000"} 0
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="30000"} 1
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="35000"} 1
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="40000"} 1
tempest_lightning_strike_distance_meters_bucket{serial="AR-00004049",le="+Inf"} 1
tempest_lightning_strike_distance_meters_sum{serial="AR-00004049"} 27000
tempest_lightning_strike_distance_meters_count{serial="AR-00004049"} 1
# HELP tempest_rain_start_events_total Rain start events.
# TYPE tempest_rain_start_events_total counter
tempest_rain_start_events_total{serial="SK-00008453"} 1
# HELP tempest_sensor_failed 1 when the device reports the sensor failed.
# TYPE tempest_sensor_failed gauge
tempest_sensor_failed{sensor="humidity",serial="AR-00004049"} 0
tempest_sensor_failed{sensor="lightning",serial="AR-00004049"} 0
tempest_sensor_failed{sensor="pressure",serial="AR-00004049"} 0
tempest_sensor_failed{sensor="temperature",serial="AR-00004049"} 0
tempest_sensor_failed{sensor="humidity",serial="ST-00000512"} 0
tempest_sensor_failed{sensor="light_uv",serial="ST-00000512"} 0
tempest_sensor_failed{sensor="lightning",serial="ST-00000512"} 0
tempest_sensor_failed{sensor="precip",serial="ST-00000512"} 0
tempest_sensor_failed{sensor="pressure",serial="ST-00000512"} 1
tempest_sensor_failed{sensor="temperature",serial="ST-00000512"} 0
tempest_sensor_failed{sensor="wind",serial="ST-00000512"} 0
# HELP tempest_power_booster Tempest power booster state flags.
# TYPE tempest_power_booster gauge
tempest_power_booster{serial="ST-00000512",state="depleted"} 0
tempest_power_booster{serial="ST-00000512",state="detected"} 1
tempest_power_booster{serial="ST-00000512",state="shore_power"} 1
# HELP tempest_hub_info Hub metadata.
# TYPE tempest_hub_info gauge
tempest_hub_info{firmware="35",hub="HB-00000001",reset_flags="BOR,PIN,POR"} 1
# HELP tempest_hub_reboots Hub reboot count.
# TYPE tempest_hub_reboots gauge
tempest_hub_reboots{hub="HB-00000001"} 1
# HELP tempest_device_info Station device metadata.
# TYPE tempest_device_info gauge
tempest_device_info{firmware="129",hub="HB-00013030",serial="ST-00000512"} 1
tempest_device_info{firmware="17",hub="HB-00000001",serial="AR-00004049"} 1
tempest_device_info{firmware="29",hub="HB-00000001",serial="SK-00008453"} 1
# HELP tempest_udp_packets_total UDP packets received by message type.
# TYPE tempest_udp_packets_total counter
tempest_udp_packets_total{type="device_status"} 2
tempest_udp_packets_total{type="evt_precip"} 1
tempest_udp_packets_total{type="evt_strike"} 1
tempest_udp_packets_total{type="hub_status"} 1
tempest_udp_packets_total{type="obs_air"} 1
tempest_udp_packets_total{type="obs_sky"} 1
tempest_udp_packets_total{type="obs_st"} 1
tempest_udp_packets_total{type="rapid_wind"} 1
`
	err = testutil.CollectAndCompare(c, strings.NewReader(want),
		"tempest_air_temperature_celsius",
		"tempest_station_pressure_pascals",
		"tempest_rain_millimeters_total",
		"tempest_precipitation_type",
		"tempest_rapid_wind_speed_meters_per_second",
		"tempest_lightning_strike_distance_meters",
		"tempest_rain_start_events_total",
		"tempest_sensor_failed",
		"tempest_power_booster",
		"tempest_hub_info",
		"tempest_hub_reboots",
		"tempest_device_info",
		"tempest_udp_packets_total",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestNullAndAbsentFieldsSkipped(t *testing.T) {
	c, _ := newTestCollector(t)
	loadSamples(t, c)

	// obs_sky sample has null rain_day; no device reported strikes in its obs.
	for _, name := range []string{"tempest_rain_day_millimeters", "tempest_lightning_strike_avg_distance_meters"} {
		if n := testutil.CollectAndCount(c, name); n != 0 {
			t.Errorf("%s: got %d series, want 0", name, n)
		}
	}
	// AIR reports no rain, SKY no lightning.
	if n := testutil.CollectAndCount(c, "tempest_rain_millimeters_total"); n != 2 {
		t.Errorf("rain counter series: got %d, want 2 (SKY, ST)", n)
	}
	if n := testutil.CollectAndCount(c, "tempest_lightning_strikes_total"); n != 2 {
		t.Errorf("strike counter series: got %d, want 2 (AIR, ST)", n)
	}
}

func obsST(ts int, rain, strikes float64) string {
	return `{"serial_number":"ST-1","type":"obs_st","hub_sn":"HB-1","obs":[[` +
		strconv.Itoa(ts) + `,0,0,0,0,3,1000,20,50,0,0,0,` + strconv.FormatFloat(rain, 'f', -1, 64) + `,1,12,` + strconv.FormatFloat(strikes, 'f', -1, 64) + `,2.6,1]]}`
}

func TestCountersIgnoreDuplicateObservations(t *testing.T) {
	c, _ := newTestCollector(t)
	handle(t, c, obsST(100, 0.5, 2), obsST(100, 0.5, 2), obsST(90, 9, 9), obsST(160, 0.25, 1))

	want := `
# HELP tempest_rain_millimeters_total Rain accumulated from observations since the exporter first saw the device.
# TYPE tempest_rain_millimeters_total counter
tempest_rain_millimeters_total{serial="ST-1"} 0.75
# HELP tempest_lightning_strikes_total Lightning strikes counted from observations since the exporter first saw the device.
# TYPE tempest_lightning_strikes_total counter
tempest_lightning_strikes_total{serial="ST-1"} 3
# HELP tempest_lightning_strike_avg_distance_meters Average lightning strike distance over the report interval; absent when no strikes.
# TYPE tempest_lightning_strike_avg_distance_meters gauge
tempest_lightning_strike_avg_distance_meters{serial="ST-1"} 12000
# HELP tempest_last_observation_timestamp_seconds Station timestamp of the latest observation.
# TYPE tempest_last_observation_timestamp_seconds gauge
tempest_last_observation_timestamp_seconds{serial="ST-1"} 160
`
	err := testutil.CollectAndCompare(c, strings.NewReader(want),
		"tempest_rain_millimeters_total",
		"tempest_lightning_strikes_total",
		"tempest_lightning_strike_avg_distance_meters",
		"tempest_last_observation_timestamp_seconds",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestStaleDevicesDropped(t *testing.T) {
	c, now := newTestCollector(t)
	loadSamples(t, c)
	if n := testutil.CollectAndCount(c, "tempest_device_info"); n != 3 {
		t.Fatalf("device_info before: got %d, want 3", n)
	}

	*now = now.Add(4 * time.Minute)
	handle(t, c, obsST(100, 0, 0))
	*now = now.Add(2 * time.Minute) // samples 6m old, ST-1 2m old

	if n := testutil.CollectAndCount(c, "tempest_device_info"); n != 1 {
		t.Errorf("device_info after: got %d, want 1", n)
	}
	if n := testutil.CollectAndCount(c, "tempest_hub_info"); n != 0 {
		t.Errorf("hub_info after: got %d, want 0", n)
	}
}

func TestBadPackets(t *testing.T) {
	c, _ := newTestCollector(t)
	if err := c.Handle([]byte("not json")); err == nil {
		t.Error("invalid JSON: want error")
	}
	if err := c.Handle([]byte(`{"type":"obs_st","obs":[[1]]}`)); err == nil {
		t.Error("missing serial: want error")
	}
	if err := c.Handle([]byte(`{"type":"something_new","serial_number":"X"}`)); err != nil {
		t.Errorf("unknown type: got %v, want nil", err)
	}

	want := `
# HELP tempest_udp_decode_errors_total UDP packets that could not be decoded, by message type.
# TYPE tempest_udp_decode_errors_total counter
tempest_udp_decode_errors_total{type="invalid"} 1
tempest_udp_decode_errors_total{type="obs_st"} 1
# HELP tempest_udp_packets_total UDP packets received by message type.
# TYPE tempest_udp_packets_total counter
tempest_udp_packets_total{type="obs_st"} 1
tempest_udp_packets_total{type="other"} 1
`
	err := testutil.CollectAndCompare(c, strings.NewReader(want),
		"tempest_udp_decode_errors_total", "tempest_udp_packets_total")
	if err != nil {
		t.Error(err)
	}
}
