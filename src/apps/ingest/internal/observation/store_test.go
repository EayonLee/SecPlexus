package observation_test

import (
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
)

func TestTrafficStatusDoesNotTreatMissingOlderHistoryAsAFirstRecord(t *testing.T) {
	now := time.Now()
	if got := observation.TrafficStatus(true, true, 0, now, now.Add(-48*time.Hour)); got != "unknown" {
		t.Fatalf("missing historical observation was reported as %q", got)
	}
	if got := observation.TrafficStatus(true, true, 0, now, now.Add(-time.Second)); got != "waiting" {
		t.Fatalf("a new source was reported as %q", got)
	}
}
