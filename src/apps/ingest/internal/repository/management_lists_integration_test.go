//go:build integration

package repository_test

import (
	"context"
	"slices"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/service"
)

func TestManagementPagesReturnFilteredRowsAndRuleSummaries(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	store := repository.NewAdminStore(pool)

	rule, err := createMapping(ctx, pool, service.SaveRuleInput{
		Name: "Edge events", Vendor: "Test", Product: "Edge", Definition: []byte(validRule),
	})
	if err != nil {
		t.Fatal(err)
	}
	port := int32(5514)
	input, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: "Edge firewall", Vendor: "Acme", Product: "Shield", Protocol: "syslog", SyslogPort: &port, ParsingRuleID: &rule.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled, enabled := false, true
	if _, _, err := repository.UpdateInput(ctx, pool, input.ID, repository.UpdateInputParams{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, query, protocol string
		enabled               *bool
		want                  int64
	}{
		{"combined filters", "EDGE", "syslog", &disabled, 1},
		{"other protocol", "edge", "http", nil, 0},
		{"other state", "edge", "", &enabled, 0},
		{"product", "shield", "", nil, 1},
		{"no match", "router", "", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, err := store.ListInputPage(ctx, repository.InputListFilter{
				Limit: 1, Query: test.query, Protocol: test.protocol, Enabled: test.enabled,
			})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != test.want || int64(len(page.Items)) != test.want ||
				(test.want == 1 && page.Items[0].ID != input.ID) {
				t.Fatalf("unexpected page: %+v", page)
			}
		})
	}

	rules, err := store.ListMappingPage(ctx, repository.MappingListFilter{Limit: 1, IDs: []string{rule.ID}})
	if err != nil || rules.Total != 1 || len(rules.Items) != 1 || len(rules.Items[0].Spec) != 0 || rules.Items[0].ParseType != "json" {
		t.Fatalf("unexpected mapping summary: %+v, %v", rules, err)
	}
	for _, test := range []struct {
		query    string
		classUID int32
		want     int64
	}{
		{"Network activity", 4001, 1},
		{"Test", 4001, 1},
		{"Edge", 4001, 1},
		{"Network activity", 4002, 0},
	} {
		rules, err := store.ListMappingPage(ctx, repository.MappingListFilter{
			Limit: 20, IDs: []string{rule.ID}, Query: test.query, ClassUID: &test.classUID,
		})
		if err != nil || rules.Total != test.want {
			t.Fatalf("query %q with class %d: %+v, %v", test.query, test.classUID, rules, err)
		}
		if test.want == 1 && !slices.Equal(rules.Items[0].ClassUIDs, []int32{4001}) {
			t.Fatalf("unexpected event classes: %v", rules.Items[0].ClassUIDs)
		}
	}

	detail, err := store.ReadMapping(ctx, rule.ID)
	if err != nil || len(detail.Spec) == 0 {
		t.Fatalf("rule definition missing: %v", err)
	}
	counts, err := store.MappingInputCounts(ctx, []string{rule.ID})
	if err != nil || counts[rule.ID] != 1 {
		t.Fatalf("unexpected rule references: %v, %v", counts, err)
	}
	page, err := store.ListInputPage(ctx, repository.InputListFilter{Limit: 1, Offset: 999, ParsingRuleID: &rule.ID})
	if err != nil || page.Offset != 0 || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("invalid offset should return a navigable page: %+v, %v", page, err)
	}
}
