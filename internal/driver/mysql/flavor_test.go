package mysql

import "testing"

func TestParseFlavor(t *testing.T) {
	tests := []struct {
		name        string
		versionRaw  string
		wantFlavor  string
		wantVersion string
	}{
		{"mysql 8.4", "8.4.3", "mysql", "8.4.3"},
		{"mariadb 11.4 with suffixes", "11.4.3-MariaDB-log", "maria", "11.4.3"},
		{"mariadb 10.11", "10.11.7-MariaDB", "maria", "10.11.7"},
		{"mysql 8.0", "8.0.36", "mysql", "8.0.36"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlavor, gotVersion := ParseFlavor(tt.versionRaw)
			if gotFlavor != tt.wantFlavor {
				t.Errorf("ParseFlavor(%q) flavor = %q, want %q", tt.versionRaw, gotFlavor, tt.wantFlavor)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("ParseFlavor(%q) version = %q, want %q", tt.versionRaw, gotVersion, tt.wantVersion)
			}
		})
	}
}
