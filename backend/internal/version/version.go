// Package version holds the server version string, stamped at build time via
//
//	go build -ldflags "-X github.com/aitjcize/esp32-photoframe-server/backend/internal/version.Version=v1.2.3"
//
// Local builds without the flag report "dev".
package version

var Version = "dev"
