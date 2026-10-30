package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin/binding"
)

func TestOrganizationInputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, slug string
		valid      bool
	}{
		{"  安全运营中心  ", "soc_team-01", true},
		{strings.Repeat("😀", 30), "soc", true},
		{strings.Repeat("中", 31), "soc", false},
		{"Ops\nTeam", "soc", false},
		{"Ops\u0085Team", "soc", false},
		{" ", "soc", false},
		{"Ops", strings.Repeat("a", 31), false},
		{"Ops", "1soc", false},
		{"Ops", "soc_", false},
		{"Ops", "SOC", false},
		{"Ops", "ab", false},
	} {
		t.Run(tc.slug+tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]string{"name": tc.name, "slug": tc.slug, "admin_user_id": "a"})
			if err != nil {
				t.Fatal(err)
			}
			var input APIOrganizationInput
			err = json.Unmarshal(data, &input)
			if err == nil {
				err = binding.Validator.ValidateStruct(input)
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
			if tc.valid && string(input.Name) != strings.TrimSpace(tc.name) {
				t.Fatal("name was not normalized")
			}
		})
	}
}
