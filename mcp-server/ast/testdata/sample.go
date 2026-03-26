// Package sample provides a small synthetic Go file for unit tests.
package sample

import "fmt"

// StatusCode represents an HTTP status code.
type StatusCode int

// DefaultTimeout is the default timeout in seconds.
const DefaultTimeout = 30

// Greet returns a greeting for the given name.
func Greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// helper is an unexported function used internally.
func helper(x int) int {
	return x * 2
}
