//go:build integration

package observation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

func TestObservationsDistinguishReceiptAndConfirmationAcrossInstances(t *testing.T) {
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		t.Fatal("isolated Redis configuration is required")
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	prefix := keyPrefix + "fixture:" + uuid.NewString() + ":"
	a, b := New(client), New(client)
	a.prefix, b.prefix = prefix, prefix
	now := time.Now().UTC().Truncate(time.Minute).Add(10 * time.Second)
	const source = "fixture"
	if err := a.Participate(ctx, time.Now().Add(presenceTTL)); err != nil {
		t.Fatal(err)
	}
	a.Receive(source, now)
	a.Normalize(source, now, pipeline.StatusOK)
	if err := a.Flush(ctx, now); err != nil {
		t.Fatal(err)
	}
	data, ready := b.Read(ctx, []string{source}, now)
	if !ready || data[source].Received != 1 || data[source].Confirmed != 0 {
		t.Fatalf("receipt was treated as broker confirmation: %+v", data)
	}
	if err := b.Participate(ctx, time.Now().Add(presenceTTL)); err != nil {
		t.Fatal(err)
	}
	if _, ready := b.Read(ctx, []string{source}, now); ready {
		t.Fatal("a different instance's flush hid the missing observation")
	}
	a.Confirm(source, now.Add(time.Second))
	b.Receive(source, now.Add(2*time.Second))
	for _, store := range []*Store{a, a, b} {
		if err := store.Flush(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	data, ready = b.Read(ctx, []string{source}, now)
	got := data[source]
	if !ready || got.Received != 2 || got.Confirmed != 1 || got.Parsed != 1 ||
		got.LastReceived != now.Add(2*time.Second).UnixMilli() || got.LastConfirmed != now.Add(time.Second).UnixMilli() {
		t.Fatalf("replicas or retry produced incorrect counts: %+v", got)
	}
}

func TestActivationWaitsForEveryUnexpiredCredentialGeneration(t *testing.T) {
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		t.Fatal("isolated Redis configuration is required")
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	prefix := keyPrefix + "fixture:" + uuid.NewString() + ":"
	a, b := New(client), New(client)
	a.prefix, b.prefix = prefix, prefix

	const id = "source"
	current := input.SourceVersion{Revision: 2, CredentialGeneration: 2, ParsingRuleVersion: 4, Enabled: true}
	old := current
	old.Revision, old.CredentialGeneration = 1, 1
	desired := map[string]input.SourceVersion{id: current}
	expires := time.Now().Add(10 * time.Second)
	acknowledge := func(store *Store, version input.SourceVersion, online bool, until time.Time) {
		t.Helper()
		if err := store.Participate(ctx, until); err != nil {
			t.Fatal(err)
		}
		if err := store.Acknowledge(ctx, map[string]input.SourceActivation{id: {SourceVersion: version, Online: online}}, until); err != nil {
			t.Fatal(err)
		}
	}
	acknowledge(a, current, true, expires)
	acknowledge(b, old, true, expires)
	if got := a.Activations(ctx, []string{id}, desired)[id].State; got != "applying" {
		t.Fatalf("old credential was still eligible but state was %q", got)
	}
	if err := b.Acknowledge(ctx, map[string]input.SourceActivation{id: {SourceVersion: current, Draining: true}}, expires); err != nil {
		t.Fatal(err)
	}
	if got := a.Activations(ctx, []string{id}, desired)[id].State; got != "applying" {
		t.Fatalf("a draining instance was reported as %q", got)
	}

	acknowledge(b, old, true, time.Now().Add(-time.Second))
	if got := a.Activations(ctx, []string{id}, desired)[id].State; got != "applied" {
		t.Fatalf("an expired node delayed activation: %q", got)
	}

	current.Enabled = false
	current.Revision++
	desired[id] = current
	if got := a.Activations(ctx, []string{id}, desired)[id].State; got != "applying" {
		t.Fatalf("disabled source was reported applied before confirmation: %q", got)
	}
	acknowledge(a, current, false, expires)
	if got := a.Activations(ctx, []string{id}, desired)[id].State; got != "applied" {
		t.Fatalf("confirmed disable did not apply: %q", got)
	}
}
