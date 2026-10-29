package model

import (
	"encoding/json"
	"testing"

	"github.com/aarondl/opt/omit"
)

func TestPatchEncodingOmitsAbsentAndKeepsExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		input   APIUserUpdate
		present bool
	}{
		{APIUserUpdate{DisplayName: "Name", Status: "active", AuthVersion: 1}, false},
		{APIUserUpdate{DisplayName: "Name", Status: "active", AuthVersion: 1, Email: omit.From("")}, true},
	} {
		data, err := json.Marshal(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		email, present := fields["email"]
		if present != tc.present || (present && string(email) != `""`) {
			t.Fatalf("optional field encoding: %s", data)
		}
	}
}
