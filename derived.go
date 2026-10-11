package main

import "math"

// feelsLike returns the apparent temperature in °C: the NWS heat index when
// at or above 26.7 °C (80 °F), the NWS wind chill when at or below 10 °C with
// wind above 4.8 km/h, otherwise the air temperature. Returns nil without
// temperature and humidity; wind may be nil (no wind chill applied).
func feelsLike(tempC, humidityPct, windMS *float64) *float64 {
	if tempC == nil || humidityPct == nil {
		return nil
	}
	t := *tempC
	switch {
	case t >= 26.7:
		t = (heatIndexF(t*9/5+32, *humidityPct) - 32) * 5 / 9
	case t <= 10 && windMS != nil && *windMS*3.6 > 4.8:
		v := math.Pow(*windMS*3.6, 0.16)
		t = 13.12 + 0.6215*t - 11.37*v + 0.3965*t*v
	}
	return &t
}

// heatIndexF implements the NWS heat index algorithm
// (https://www.wpc.ncep.noaa.gov/html/heatindex_equation.shtml) in °F.
func heatIndexF(t, rh float64) float64 {
	hi := 0.5 * (t + 61 + (t-68)*1.2 + rh*0.094)
	if (hi+t)/2 < 80 {
		return hi
	}
	hi = -42.379 + 2.04901523*t + 10.14333127*rh - 0.22475541*t*rh -
		0.00683783*t*t - 0.05481717*rh*rh + 0.00122874*t*t*rh +
		0.00085282*t*rh*rh - 0.00000199*t*t*rh*rh
	switch {
	case rh < 13 && t >= 80 && t <= 112:
		hi -= (13 - rh) / 4 * math.Sqrt((17-math.Abs(t-95))/17)
	case rh > 85 && t >= 80 && t <= 87:
		hi += (rh - 85) / 10 * (87 - t) / 5
	}
	return hi
}

// seaLevelPressure reduces station pressure (mb) to sea level using the
// standard atmosphere, as in WeatherFlow's derived metrics. elevation is the
// station height above sea level in meters.
func seaLevelPressure(stationMB *float64, elevation float64) *float64 {
	if stationMB == nil {
		return nil
	}
	const (
		p0    = 1013.25 // mb, standard sea-level pressure
		rd    = 287.05  // J/(kg·K), dry air gas constant
		gamma = 0.0065  // K/m, standard lapse rate
		g     = 9.80665 // m/s²
		t0    = 288.15  // K, standard sea-level temperature
	)
	p := *stationMB
	x := rd * gamma / g
	slp := p * math.Pow(1+math.Pow(p0/p, x)*(gamma*elevation/t0), 1/x)
	return &slp
}
