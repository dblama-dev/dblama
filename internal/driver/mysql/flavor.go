package mysql

import (
	"regexp"
	"strings"
)

// Flavor values reported by ParseFlavor.
const (
	FlavorMySQL   = "mysql"
	FlavorMariaDB = "maria"
)

// versionRe extracts the leading semver-like version from a raw
// SELECT VERSION() string such as "11.4.3-MariaDB-log".
var versionRe = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)

// ParseFlavor splits a raw version string from SELECT VERSION() into a
// flavor ("mysql" or "maria") and a bare "major.minor.patch" version.
func ParseFlavor(versionRaw string) (flavor, version string) {
	if strings.Contains(versionRaw, "MariaDB") {
		flavor = FlavorMariaDB
	} else {
		flavor = FlavorMySQL
	}
	if m := versionRe.FindStringSubmatch(versionRaw); m != nil {
		version = m[1]
	}
	return flavor, version
}
