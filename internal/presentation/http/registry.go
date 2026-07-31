package http

import (
	"net/http"

	"github.com/gorilla/mux"
)

// HandlerRegistry gerencia o registro declarativo de handlers HTTP custom.
// Substitui o wiring individual em custom_routes.go, permitindo escala limpa
// para 5-15 endpoints sem explosão de boilerplate.
type HandlerRegistry struct {
	routes []routeEntry
}

type routeEntry struct {
	path    string
	handler http.Handler
	methods []string
}

// NewHandlerRegistry cria um registry vazio.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{}
}

// Register adiciona uma rota ao registry.
func (r *HandlerRegistry) Register(path string, handler http.Handler, methods ...string) {
	r.routes = append(r.routes, routeEntry{path: path, handler: handler, methods: methods})
}

// Apply registra todas as rotas no router.
func (r *HandlerRegistry) Apply(router *mux.Router) {
	for _, entry := range r.routes {
		route := router.Handle(entry.path, entry.handler)
		if len(entry.methods) > 0 {
			route.Methods(entry.methods...)
		}
	}
}
