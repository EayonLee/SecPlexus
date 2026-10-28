package authentication

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/aarondl/authboss/v3"
	"github.com/alexedwards/scs/v2"
)

func TestAuthbossSessionStateEvents(t *testing.T) {
	manager := scs.New()
	ctx, err := manager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	sessions := &Sessions{Manager: manager}
	bridge := sessionBridge{sessions: sessions}
	state, err := bridge.ReadState(httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}

	if err = bridge.WriteState(httptest.NewRecorder(), state, []authboss.ClientStateEvent{{Kind: authboss.ClientStateEventPut, Key: "uid", Value: "user"}, {Kind: authboss.ClientStateEventPut, Key: "keep", Value: "allowed"}, {Kind: authboss.ClientStateEventDel, Key: "uid"}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := state.Get("uid"); ok {
		t.Fatal("deleted state survived")
	}

	manager.Put(ctx, "remove", "value")
	if err = bridge.WriteState(httptest.NewRecorder(), state, []authboss.ClientStateEvent{{Kind: authboss.ClientStateEventDelAll, Key: "keep"}}); err != nil {
		t.Fatal(err)
	}

	if manager.GetString(ctx, "keep") != "allowed" || manager.GetString(ctx, "remove") != "" {
		t.Fatal("whitelist was not respected")
	}
}
