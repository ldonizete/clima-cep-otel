// Package cep holds the zipcode format rule shared by both services.
package cep

// Length is the number of digits in a valid CEP.
const Length = 8

// Valid reports whether s is exactly eight ASCII digits.
func Valid(s string) bool {
	if len(s) != Length {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
