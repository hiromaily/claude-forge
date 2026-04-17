package dashboard

import "testing"

// TestDashboardURL_FormatsLocalhostURL verifies the user-facing URL format.
// Pure function — safe to run in parallel.
func TestDashboardURL_FormatsLocalhostURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		port int
		want string
	}{
		{name: "common_port", port: 9876, want: "http://localhost:9876/"},
		{name: "low_port", port: 1, want: "http://localhost:1/"},
		{name: "high_port", port: 65535, want: "http://localhost:65535/"},
		// Documents current behaviour for an out-of-range value: the function
		// formats whatever it receives; range validation is the caller's job.
		// Start never reaches this case in practice because it reads from an
		// already-bound listener, so this case is documentation-only.
		{name: "zero_port_passthrough", port: 0, want: "http://localhost:0/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := dashboardURL(tc.port)
			if got != tc.want {
				t.Errorf("dashboardURL(%d) = %q, want %q", tc.port, got, tc.want)
			}
		})
	}
}
