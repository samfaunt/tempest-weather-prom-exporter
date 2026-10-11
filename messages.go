package main

import "strings"

// message is the union of every field used from the Tempest UDP local API (v171).
// See https://weatherflow.github.io/Tempest/api/udp/v171/.
// Numeric fields are pointers because the hub sends null for missing readings.
type message struct {
	Type             string     `json:"type"`
	SerialNumber     string     `json:"serial_number"`
	HubSN            string     `json:"hub_sn"`
	FirmwareRevision flexString `json:"firmware_revision"`

	Obs [][]*float64 `json:"obs"` // obs_air, obs_sky, obs_st
	Ob  []*float64   `json:"ob"`  // rapid_wind
	Evt []*float64   `json:"evt"` // evt_precip, evt_strike

	// device_status and hub_status
	Uptime       *float64   `json:"uptime"`
	Voltage      *float64   `json:"voltage"`
	RSSI         *float64   `json:"rssi"`
	HubRSSI      *float64   `json:"hub_rssi"`
	SensorStatus *uint64    `json:"sensor_status"`
	ResetFlags   string     `json:"reset_flags"`
	RadioStats   []*float64 `json:"radio_stats"`
}

// flexString accepts a JSON string or number; hub_status sends firmware_revision
// as a string while every other message sends an integer.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		return nil
	}
	*f = flexString(strings.Trim(s, `"`))
	return nil
}

// observation is one row of an obs_* message in the units the hub reports.
// Fields a device type does not report stay nil.
type observation struct {
	time           *float64 // epoch seconds
	windLull       *float64 // m/s
	windAvg        *float64 // m/s
	windGust       *float64 // m/s
	windDirection  *float64 // degrees
	pressure       *float64 // mb
	temperature    *float64 // °C
	humidity       *float64 // %
	illuminance    *float64 // lux
	uv             *float64 // index
	solarRadiation *float64 // W/m²
	rain           *float64 // mm over the report interval
	rainDay        *float64 // mm, local day accumulation (SKY only)
	precipType     *float64 // 0 none, 1 rain, 2 hail, 3 rain + hail
	strikeDistance *float64 // km, average over the report interval
	strikeCount    *float64
	battery        *float64 // V
}

// at returns row[i], or nil when the row is too short.
func at(row []*float64, i int) *float64 {
	if i < len(row) {
		return row[i]
	}
	return nil
}

func decodeObsST(row []*float64) observation {
	return observation{
		time:           at(row, 0),
		windLull:       at(row, 1),
		windAvg:        at(row, 2),
		windGust:       at(row, 3),
		windDirection:  at(row, 4),
		pressure:       at(row, 6),
		temperature:    at(row, 7),
		humidity:       at(row, 8),
		illuminance:    at(row, 9),
		uv:             at(row, 10),
		solarRadiation: at(row, 11),
		rain:           at(row, 12),
		precipType:     at(row, 13),
		strikeDistance: at(row, 14),
		strikeCount:    at(row, 15),
		battery:        at(row, 16),
	}
}

func decodeObsAir(row []*float64) observation {
	return observation{
		time:           at(row, 0),
		pressure:       at(row, 1),
		temperature:    at(row, 2),
		humidity:       at(row, 3),
		strikeCount:    at(row, 4),
		strikeDistance: at(row, 5),
		battery:        at(row, 6),
	}
}

func decodeObsSky(row []*float64) observation {
	return observation{
		time:           at(row, 0),
		illuminance:    at(row, 1),
		uv:             at(row, 2),
		rain:           at(row, 3),
		windLull:       at(row, 4),
		windAvg:        at(row, 5),
		windGust:       at(row, 6),
		windDirection:  at(row, 7),
		battery:        at(row, 8),
		solarRadiation: at(row, 10),
		rainDay:        at(row, 11),
		precipType:     at(row, 12),
	}
}
