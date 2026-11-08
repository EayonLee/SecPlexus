package identity

import "testing"

func TestTimeZoneValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty means unset", value: "", valid: true},
		{name: "utc semantic value", value: "utc", valid: true},
		{name: "browser semantic value", value: "browser", valid: true},
		{name: "IANA zone", value: "Asia/Shanghai", valid: true},
		{name: "IANA zone with region", value: "America/Los_Angeles", valid: true},
		{name: "IANA UTC name", value: "UTC", valid: true},
		{name: "unknown zone", value: "Not/AZone", valid: false},
		{name: "bare name without region", value: "Shanghai", valid: false},
		{name: "path traversal attempt", value: "../../etc/passwd", valid: false},
		{name: "sql fragment", value: "UTC'; DROP TABLE iam.users; --", valid: false},
		{name: "padded zone", value: " Asia/Shanghai", valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ValidTimeZone(tc.value); got != tc.valid {
				t.Fatalf("ValidTimeZone(%q) = %v, want %v", tc.value, got, tc.valid)
			}
		})
	}
}
