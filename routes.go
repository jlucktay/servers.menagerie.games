package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

// setupRoutes will set up routes on the given server.
func (s *server) setupRoutes() {
	s.router.Get("/", s.rootPageHandler(viper.GetString("google_client_id"))) // GET /
	s.router.Get("/favicon.ico", s.faviconHandler)                            // GET /favicon.ico
	s.router.Post("/tokensignin", s.tokenSignInHandler)                       // POST /tokensignin

	// s.router.Mount(pattern string, handler http.Handler)

	s.router.Route("/manage", func(r chi.Router) {
		r.Use(s.authorisedOnly)

		r.Get("/", s.manageGetHandler)   // GET /manage
		r.Post("/", s.managePostHandler) // POST /manage
	})
}
