package testdata

import "fmt"

// DepCFunc calls fmt.Sprintf to demonstrate a qualified call site.
func DepCFunc() string {
	return fmt.Sprintf("dep_c: %d", 42)
}
