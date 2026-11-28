package event

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFirstMigrationDoesNotReadCurrentProjection(t *testing.T) {
	want, err := EventTableMigration()
	if err != nil {
		t.Fatal(err)
	}
	original := projectionJSON
	t.Cleanup(func() { projectionJSON = original })
	projectionJSON = []byte(`{"contract_version":"future","columns":[]}`)
	got, err := EventTableMigration()
	if err != nil || got != want {
		t.Fatalf("first migration changed with the active projection: %v", err)
	}
}

func TestEventWritesToConfiguredDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("query")
		if got != "INSERT INTO \"deployment_fixture\".events FORMAT JSONEachRow" {
			t.Errorf("unexpected insert target: %s", got)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	store, err := NewStore(server.URL, "deployment_fixture", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Insert(context.Background(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaChecksDoNotReadCurrentProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "system.columns"):
			// 模拟首版表的列注释被误改。
			columns, err := migrationV1Columns()
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			columns[0].Comment = "unexpected"
			for _, col := range columns {
				_ = json.NewEncoder(w).Encode(map[string]string{"name": col.Name, "type": col.Type, "comment": col.Comment})
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	store, err := NewStore(server.URL, "spx", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	original := projectionJSON
	t.Cleanup(func() { projectionJSON = original })
	projectionJSON = []byte(`{"contract_version":"future","columns":[]}`)
	err = store.CheckSchema(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ClickHouse column event_id differs") {
		t.Errorf("deployed column drift was not identified: %v", err)
	}
}
