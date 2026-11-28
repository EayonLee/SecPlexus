package validation

import (
	"strings"
	"testing"
)

func TestSampleLimits(t *testing.T) {
	tests := []struct {
		name    string
		samples []string
		valid   bool
	}{
		{name: "no samples"},
		{name: "empty sample", samples: []string{"\t\n "}},
		{name: "twenty samples", samples: strings.Fields(strings.Repeat("sample ", 20)), valid: true},
		{name: "twenty-one samples", samples: strings.Fields(strings.Repeat("sample ", 21))},
		{name: "maximum decoded bytes", samples: []string{strings.Repeat("界", 21845) + "a"}, valid: true},
		{name: "excess decoded bytes", samples: []string{strings.Repeat("界", 21846)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if (CheckSamples(test.samples) == nil) != test.valid {
				t.Errorf("sample validity should be %t", test.valid)
			}
		})
	}
}
