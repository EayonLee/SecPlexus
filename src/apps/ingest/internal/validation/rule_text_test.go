package validation_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

func TestRuleText(t *testing.T) {
	raw, err := os.ReadFile("../../../../../docs/plans/ingest-rule-validation/validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Field      string
		Value      string
		Repeat     int
		Normalized *string
		Code       string
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Field+"/"+tc.Code, func(t *testing.T) {
			field := validation.Field(tc.Field)
			if tc.Field == "description" {
				field = validation.Description
			}
			input := strings.Repeat(tc.Value, max(1, tc.Repeat))
			got, err := validation.RuleText(field, input)
			if tc.Code != "" {
				var issue *validation.Issue
				if !errors.As(err, &issue) || issue.Code != tc.Code || issue.Field != field {
					t.Fatalf("error = %v, want %s for %s", err, tc.Code, field)
				}
				return
			}
			want := input
			if tc.Normalized != nil {
				want = *tc.Normalized
			}
			if err != nil || got != want {
				t.Fatalf("rule text did not normalize as expected: %v", err)
			}
		})
	}
}
