package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// F229: /status/set/text was removed — it pointed to the same handler as
// /users/status (profile "About" text) and did NOT publish a status story.
// Keeping it would perpetuate a misleading contract.

func TestStatusSetTextRouteRemoved(t *testing.T) {
	router := newRouterForRouteCheck()

	req := httptest.NewRequest(http.MethodPost, "/status/set/text", nil)
	var match mux.RouteMatch
	if router.Match(req, &match) {
		t.Fatal("/status/set/text still matches a registered route — it should have been removed (F229)")
	}
}

func TestUserStatusRouteStillExists(t *testing.T) {
	router := newRouterForRouteCheck()

	req := httptest.NewRequest(http.MethodPost, "/users/status", nil)
	var match mux.RouteMatch
	if !router.Match(req, &match) {
		t.Fatal("/users/status does not match any route — it must remain registered")
	}
}

func TestStatusStoryRoutesIntact(t *testing.T) {
	router := newRouterForRouteCheck()

	stories := []string{
		"/status/set/image",
		"/status/set/video",
		"/status/set/audio",
	}
	for _, path := range stories {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		var match mux.RouteMatch
		if !router.Match(req, &match) {
			t.Errorf("%s does not match any route — story routes must remain intact (F229)", path)
		}
	}
}
