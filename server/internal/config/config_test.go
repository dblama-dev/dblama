package config

import (
	"reflect"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults when env is empty",
			env:  map[string]string{},
			want: Config{
				Listen:         ":8080",
				SessionTTL:     30 * time.Minute,
				LoginRateLimit: Rate{Count: 10, Per: time.Minute},
			},
		},
		{
			name: "all four variables overridden",
			env: map[string]string{
				"LISTEN":           ":9000",
				"SESSION_TTL":      "1h15m",
				"LOGIN_RATE_LIMIT": "5/s",
				"TRUSTED_SERVERS":  "db1:3306",
			},
			want: Config{
				Listen:         ":9000",
				SessionTTL:     75 * time.Minute,
				LoginRateLimit: Rate{Count: 5, Per: time.Second},
				TrustedServers: []string{"db1:3306"},
			},
		},
		{
			name:    "invalid session ttl",
			env:     map[string]string{"SESSION_TTL": "abc"},
			wantErr: true,
		},
		{
			name:    "rate limit without unit",
			env:     map[string]string{"LOGIN_RATE_LIMIT": "10"},
			wantErr: true,
		},
		{
			name:    "rate limit with non-positive count",
			env:     map[string]string{"LOGIN_RATE_LIMIT": "0/min"},
			wantErr: true,
		},
		{
			name: "trusted servers split into elements",
			env:  map[string]string{"TRUSTED_SERVERS": "db1:3306,db2:5432"},
			want: Config{
				Listen:         ":8080",
				SessionTTL:     30 * time.Minute,
				LoginRateLimit: Rate{Count: 10, Per: time.Minute},
				TrustedServers: []string{"db1:3306", "db2:5432"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			got, err := Load(getenv)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
