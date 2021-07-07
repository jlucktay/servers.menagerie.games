package main

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Initialise will set up routes and middlware on the given server.
func (s *Server) Initialise() {
	s.Router.Use(middleware.RequestID)
	// s.router.Use(middleware.RealIP) // TODO: look into security implications
	s.Router.Use(middleware.Logger) // look at https://github.com/goware/httplog as well
	s.Router.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed
	// out and further processing should be stopped.
	s.Router.Use(middleware.Timeout(time.Second * 10))

	s.Router.Use(middleware.Heartbeat("/ping"))
	s.Router.Use(middleware.Throttle(100))

	s.Router.Get("/", s.rootPageHandler(s.Config.Audience)) // GET /
	s.Router.Get("/favicon.ico", s.faviconHandler)          // GET /favicon.ico
	s.Router.Post("/tokensignin", s.tokenSignInHandler)     // POST /tokensignin

	s.Router.Route("/manage", func(r chi.Router) {
		r.Use(s.authorisedOnly)

		r.Get("/", s.manageGetHandler()) // GET /manage

		r.Route("/", func(r chi.Router) {
			// Only one request will be processed at a time.
			r.Use(middleware.Throttle(1))

			r.Post("/", s.managePostHandler) // POST /manage
		})
	})
}
