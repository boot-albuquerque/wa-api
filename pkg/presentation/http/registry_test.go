package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func okHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
}

// TestHandlerRegistryAppliesRoutes cobre o registry inteiro — construtor,
// Register e Apply — pela única coisa que importa dele: depois de Apply, o
// router serve o handler registrado no path registrado.
func TestHandlerRegistryAppliesRoutes(t *testing.T) {
	reg := NewHandlerRegistry()
	reg.Register("/session/profile", okHandler("profile"))
	reg.Register("/session/status", okHandler("status"))

	router := mux.NewRouter()
	reg.Apply(router)

	for path, want := range map[string]string{
		"/session/profile": "profile",
		"/session/status":  "status",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if rec.Body.String() != want {
			t.Errorf("GET %s body = %q, want %q", path, rec.Body.String(), want)
		}
	}
}

// TestHandlerRegistryMethodRestriction: uma rota registrada com métodos só
// responde a esses métodos. Sem esta asserção, `route.Methods(...)` poderia
// deixar de ser chamado e o registry passaria a expor toda rota a todo verbo
// — um GET-only virando um POST aceito silenciosamente.
func TestHandlerRegistryMethodRestriction(t *testing.T) {
	reg := NewHandlerRegistry()
	reg.Register("/session/profile", okHandler("profile"), http.MethodGet)
	reg.Register("/session/open", okHandler("open"))

	router := mux.NewRouter()
	reg.Apply(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/session/profile", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("POST to a GET-only route returned %d; the method restriction was not applied", rec.Code)
	}

	// A rota registrada SEM métodos é o outro ramo de Apply: ela continua
	// aceitando qualquer verbo.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/session/open", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("POST to an unrestricted route = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandlerRegistryEmptyRegistryAppliesNothing(t *testing.T) {
	router := mux.NewRouter()
	NewHandlerRegistry().Apply(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET on a router with no routes = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		val   string
		want  bool
	}{
		{"present at head", []string{"Message", "Receipt"}, "Message", true},
		{"present at tail", []string{"Message", "Receipt"}, "Receipt", true},
		{"absent", []string{"Message", "Receipt"}, "Presence", false},
		{"empty slice", nil, "Message", false},
		{"empty value absent", []string{"Message"}, "", false},
		{"empty value present", []string{""}, "", true},
		{"case sensitive", []string{"Message"}, "message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Find(tt.slice, tt.val); got != tt.want {
				t.Errorf("Find(%v, %q) = %v, want %v", tt.slice, tt.val, got, tt.want)
			}
		})
	}
}
