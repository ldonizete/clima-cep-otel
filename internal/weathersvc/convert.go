package weathersvc

import "math"

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// ToFahrenheit converts Celsius to Fahrenheit: F = C × 1.8 + 32.
func ToFahrenheit(c float64) float64 {
	return round2(c*1.8 + 32)
}

// ToKelvin converts Celsius to Kelvin: K = C + 273.
func ToKelvin(c float64) float64 {
	return round2(c + 273)
}
