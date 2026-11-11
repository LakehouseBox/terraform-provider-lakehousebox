package provider

import (
	"context"

	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecondsReadsTheAPIsDurations(t *testing.T) {
	for in, want := range map[string]int64{"15m": 900, "1h": 3600, "90s": 90, "3600": 3600, "1d": 86400} {
		if got, err := seconds(in); err != nil || got != want {
			t.Fatalf("seconds(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := seconds("soon"); err == nil {
		t.Fatal("a word is not a duration")
	}
}

// The API's error envelope reaches the user with its code and remedy, and a 404 is told apart (a resource gone
// outside OpenTofu is removed from state, never an error).
func TestTheClientRelaysTheErrorEnvelopeAndKnowsA404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k1" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/sinks/gone" {
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "sink_not_found", "message": "no"}})
			return
		}
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "sink_exists", "message": "taken",
			"remedy": "delete it"}})
	}))
	defer srv.Close()
	c := newClient(srv.URL, "k1", "test")
	err := c.Do(context.Background(), "GET", "/v1/sinks/gone", nil, nil, nil)
	if !NotFound(err) {
		t.Fatalf("want a 404, got %v", err)
	}
	err = c.Do(context.Background(), "POST", "/v1/sinks", nil, map[string]string{"name": "x"}, nil)
	e, ok := err.(*APIError)
	if !ok || e.Status != 409 || e.Code != "sink_exists" || e.Remedy != "delete it" || NotFound(err) {
		t.Fatalf("want the 409 envelope, got %#v", err)
	}
}

// A catalog is deleted only when deletion_protection is explicitly false: true, unset (a state from before the
// attribute) or unknown keep it, so a mistaken destroy never starts the purge of every table and file.
func TestACatalogIsDeletedOnlyWithProtectionOff(t *testing.T) {
	for _, p := range []types.Bool{types.BoolValue(true), types.BoolNull(), types.BoolUnknown()} {
		if deleteAllowed(catalogModel{Name: types.StringValue("c"), DeletionProtection: p}) == nil {
			t.Fatalf("deletion_protection %v must refuse", p)
		}
	}
	if err := deleteAllowed(catalogModel{Name: types.StringValue("c"), DeletionProtection: types.BoolValue(false)}); err != nil {
		t.Fatalf("protection off must allow: %v", err) // positive control
	}
}

// A route the deployment does not serve (a bare `not_found`) is never read as the resource being gone: that would
// drop every resource from state on a refresh and plan to recreate them (high review of #3032). Control: a sink's own
// 404 is gone.
func TestOnlyAResourcesOwn404IsGone(t *testing.T) {
	if NotFound(&APIError{Status: 404, Code: "not_found"}) {
		t.Fatal("a bare not_found must not remove a resource from state")
	}
	if !NotFound(&APIError{Status: 404, Code: "sink_not_found"}) || NotFound(&APIError{Status: 409, Code: "sink_not_found"}) {
		t.Fatal("sink_not_found with 404 is gone; nothing else is")
	}
}

// The API keeps a prefix in its canonical form: a configured spelling that means the same is the same prefix (else the
// POST prefix would drop out of state on every refresh and be declared again, 409).
func TestAPrefixMatchesItsCanonicalForm(t *testing.T) {
	if !samePrefix("https://API.example.org:443/v1/", "https://api.example.org/v1/") {
		t.Fatal(":443 and the host's case are the same prefix")
	}
	if samePrefix("https://api.example.org:8443/v1/", "https://api.example.org/v1/") || samePrefix("https://a.example/x/", "https://a.example/y/") {
		t.Fatal("another port or path is another prefix")
	}
}
