package backend

import (
	"errors"
	"testing"
)

func TestDeviceZoneFollowsGoLocalTime(t *testing.T) {
	for _, test := range []struct {
		name string
		tz   *string
		link string
		want string
	}{
		{name: "the macOS link", link: "/var/db/timezone/zoneinfo/Europe/Lisbon", want: "Europe/Lisbon"},
		{name: "a Linux link", link: "/usr/share/zoneinfo/America/Sao_Paulo", want: "America/Sao_Paulo"},
		{name: "a relative link", link: "../usr/share/zoneinfo/Pacific/Auckland", want: "Pacific/Auckland"},
		{name: "a copied file", link: "", want: ""},
		{name: "a link elsewhere", link: "/opt/zones/Lisbon", want: ""},
		{name: "a name", tz: ptr("Asia/Tokyo"), want: "Asia/Tokyo"},
		{name: "a name after a colon", tz: ptr(":Asia/Tokyo"), want: "Asia/Tokyo"},
		{name: "a zoneinfo path", tz: ptr("/usr/share/zoneinfo/Asia/Tokyo"), want: "Asia/Tokyo"},
		{name: "the localtime path", tz: ptr("/etc/localtime"), link: "/usr/share/zoneinfo/Europe/Lisbon", want: "Europe/Lisbon"},
		{name: "empty", tz: ptr(""), link: "/usr/share/zoneinfo/Europe/Lisbon", want: "UTC"},
		{name: "unknown", tz: ptr("Mars/Olympus_Mons"), want: "UTC"},
		{name: "Local", tz: ptr("Local"), want: "UTC"},
	} {
		lookupEnv := func(string) (string, bool) {
			if test.tz == nil {
				return "", false
			}
			return *test.tz, true
		}
		readlink := func(string) (string, error) {
			if test.link == "" {
				return "", errors.New("not a link")
			}
			return test.link, nil
		}
		if got := deviceZone(lookupEnv, readlink); got != test.want {
			t.Errorf("%s: zone = %q, want %q", test.name, got, test.want)
		}
	}
}
