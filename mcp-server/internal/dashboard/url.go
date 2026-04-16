package dashboard

import "fmt"

// dashboardURL formats the user-facing URL printed at startup.
//
// Always uses "localhost" rather than the bind interface so the line is
// click-through in any modern terminal regardless of which host the
// listener bound to (commonly "[::]:<port>" on dual-stack systems).
//
// port is expected to be a TCP port the kernel actually accepted — in
// practice the value returned by net.Listener.Addr().(*net.TCPAddr).Port.
// No range validation is performed; callers control the input.
func dashboardURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/", port)
}
