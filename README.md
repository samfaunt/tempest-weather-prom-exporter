# tempest-weather-prom-exporter

Prometheus exporter for WeatherFlow Tempest weather stations. It listens for the
hub's [UDP local API](https://weatherflow.github.io/Tempest/api/udp/v171/)
broadcasts (port 50222) — no cloud API token or polling — and serves the latest
readings on `:7070/metrics`.

## Run

```sh
docker run -d --name tempest-exporter --network host --restart unless-stopped \
  ghcr.io/samfaunt/tempest-weather-prom-exporter:latest
curl localhost:7070/metrics
```

Or build locally with `docker compose up -d --build`. Images for `linux/amd64`
and `linux/arm64` are published on pushes to `main` (`latest`) and `v*` tags.

The container must use `network_mode: host`; the hub's broadcasts do not reach
containers on Docker's bridge network. Allow inbound UDP 50222 in the host
firewall.

Without Docker:

```sh
go build -o tempest-exporter . && ./tempest-exporter
```

The socket is opened with `SO_REUSEPORT`, so the exporter can run alongside
other listeners such as Home Assistant.

## Configuration

| Flag | Env | Default | |
|---|---|---|---|
| `--listen-address` | `TEMPEST_LISTEN_ADDRESS` | `:7070` | HTTP address for `/metrics` |
| `--udp-address` | `TEMPEST_UDP_ADDRESS` | `:50222` | UDP address for hub broadcasts |
| `--stale-after` | `TEMPEST_STALE_AFTER` | `5m` | Drop a device's or hub's metrics after this long without packets |

## Prometheus

```yaml
scrape_configs:
  - job_name: tempest
    static_configs:
      - targets: ["exporter-host:7070"]
```

## Metrics

Station metrics carry a `serial` label, hub metrics a `hub` label. Units follow
Prometheus conventions (Pascals, ratio for humidity, meters for distance)
except rain, which stays in millimeters.

- Observations (`obs_st`, `obs_air`, `obs_sky`): `tempest_air_temperature_celsius`,
  `tempest_station_pressure_pascals`, `tempest_relative_humidity_ratio`,
  `tempest_wind_{lull,avg,gust}_meters_per_second`, `tempest_wind_direction_degrees`,
  `tempest_illuminance_lux`, `tempest_uv_index`,
  `tempest_solar_radiation_watts_per_square_meter`, `tempest_battery_volts`,
  `tempest_precipitation_type{type}`, `tempest_rain_day_millimeters` (SKY only),
  `tempest_lightning_strike_avg_distance_meters`,
  `tempest_last_observation_timestamp_seconds`.
- Counters built from observations: `tempest_rain_millimeters_total`,
  `tempest_lightning_strikes_total`. Repeated observations (same or older
  timestamp) are ignored, so they are never counted twice. They reset when the
  exporter restarts, which `increase()` handles.
- `rapid_wind` (every ~3s): `tempest_rapid_wind_speed_meters_per_second`,
  `tempest_rapid_wind_direction_degrees`.
- Events: `tempest_lightning_strike_distance_meters` (histogram),
  `tempest_lightning_last_strike_{timestamp_seconds,distance_meters,energy}`,
  `tempest_rain_start_events_total`, `tempest_rain_last_start_timestamp_seconds`.
- `device_status`: `tempest_device_{uptime_seconds,voltage_volts,rssi_dbm,hub_rssi_dbm}`,
  `tempest_sensor_failed{sensor}`, `tempest_sensor_lightning_{noise,disturber}`,
  `tempest_power_booster{state}`, `tempest_device_info{hub,firmware}`.
- `hub_status`: `tempest_hub_{uptime_seconds,rssi_dbm,reboots,i2c_bus_errors,radio_status}`,
  `tempest_hub_info{firmware,reset_flags}`.
- Exporter: `tempest_udp_packets_total{type}`, `tempest_udp_decode_errors_total{type}`.

Dew point, feels-like and sea-level pressure are not computed; derive them with
recording rules or in Grafana.

### PromQL recipes

```promql
# Rain in the last 24 hours (mm)
increase(tempest_rain_millimeters_total[24h])

# Lightning strikes per hour
increase(tempest_lightning_strikes_total[1h])

# Strikes within 10 km in the last hour
increase(tempest_lightning_strike_distance_meters_bucket{le="10000"}[1h])

# Any failed sensor
tempest_sensor_failed == 1
```

## Development

```sh
go test ./...
```

`testdata/sample.jsonl` holds the example packets from the API docs. To feed
them to a running exporter without a hub:

```sh
while IFS= read -r l; do printf '%s' "$l" > /dev/udp/127.0.0.1/50222; done < testdata/sample.jsonl
```

To check that the hub is broadcasting: `tcpdump -n udp port 50222`.
