//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/service"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

func TestMappingMetadata(t *testing.T) {
	raw, err := os.ReadFile("../../../../../docs/plans/ingest-rule-validation/validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Field, Value, Code string
		Repeat             int
		Normalized         *string
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	pool := openPool(t)
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Field+"/"+tc.Code, func(t *testing.T) {
			input := service.SaveRuleInput{Name: uniqueName(t, "metadata"), Vendor: "Test", Product: "Edge", Definition: []byte(validRule)}
			value := strings.Repeat(tc.Value, max(1, tc.Repeat))
			switch tc.Field {
			case "name":
				input.Name = value
			case "vendor":
				input.Vendor = value
			case "product":
				input.Product = value
			case "description":
				var payload map[string]any
				if err := json.Unmarshal(input.Definition, &payload); err != nil {
					t.Fatal(err)
				}
				payload["description"] = value
				input.Definition, err = json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			row, err := createMapping(ctx, pool, input)
			if tc.Code != "" {
				var issue *validation.Issue
				if !errors.As(err, &issue) || issue.Code != tc.Code {
					t.Fatalf("invalid %s error = %v, want %s", tc.Field, err, tc.Code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.Normalized != nil {
				value = *tc.Normalized
			}
			got := map[string]string{"name": row.Name, "vendor": row.Vendor, "product": row.Product, "description": row.Description}
			if got[tc.Field] != value {
				t.Fatalf("stored %s incorrectly", tc.Field)
			}
		})
	}
}

func TestMappingNameConflictsOnConcurrentSaves(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	const name = "Concurrent rule"
	results := make(chan error, 2)
	var writers sync.WaitGroup
	for _, value := range []string{name, "　CONCURRENT RULE　"} {
		writers.Go(func() {
			_, err := createMapping(ctx, pool, service.SaveRuleInput{Name: value, Vendor: "Test", Product: "Edge", Definition: []byte(validRule)})
			results <- err
		})
	}
	writers.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, repository.ErrDuplicateName):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successful saves=%d, name conflicts=%d", successes, conflicts)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ingest.parsing_rule_revisions WHERE lower(name) = lower($1)", name).Scan(&count); err != nil || count != 1 {
		t.Fatalf("the name should have one revision, found %d: %v", count, err)
	}
}
