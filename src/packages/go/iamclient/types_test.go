package iamclient

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionsAreDecimalStrings(t *testing.T) {
	data, err := json.Marshal(Principal{AuthVersion: 9007199254740993})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"auth_version":"9007199254740993"`) {
		t.Fatal("version lost wire precision")
	}
}
