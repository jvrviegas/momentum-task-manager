package backend

import (
	"os"
	"strings"
	"time"
)

// DeviceZone returns the IANA name of the zone time.Local follows, or "" when
// nothing names one, such as an /etc/localtime that is a copy, not a link.
func DeviceZone() string { return deviceZone(os.LookupEnv, os.Readlink) }

// deviceZone reads TZ as Go does when it sets time.Local: unset, the zone is
// the one /etc/localtime links to; empty, UTC, or unknown, it is UTC; and an
// absolute path names a zoneinfo file.
func deviceZone(lookupEnv func(string) (string, bool), readlink func(string) (string, error)) string {
	name, ok := lookupEnv("TZ")
	name = strings.TrimPrefix(name, ":")
	if !ok || name == "/etc/localtime" {
		target, err := readlink("/etc/localtime")
		if err != nil {
			return ""
		}
		return zoneFromPath(target)
	}
	if strings.HasPrefix(name, "/") {
		return zoneFromPath(name)
	}
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" {
		return "UTC"
	}
	return name
}

// zoneFromPath returns the zone a zoneinfo file such as
// /usr/share/zoneinfo/Europe/Lisbon holds.
func zoneFromPath(path string) string {
	_, name, ok := strings.Cut(path, "zoneinfo/")
	if _, err := time.LoadLocation(name); !ok || err != nil || name == "" {
		return ""
	}
	return name
}
