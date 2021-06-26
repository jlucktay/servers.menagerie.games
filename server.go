package main

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/viper"
)

type server struct {
	cfg    serverConfig
	router *chi.Mux
}

type serverConfig struct {
	audience           string
	authorisedSubjects []string
}

// new sets up and returns a new server.
func new() server {
	s := server{
		cfg: serverConfig{
			audience:           viper.GetString("google_client_id"),
			authorisedSubjects: viper.GetStringSlice("auth_sub"),
		},
		router: chi.NewRouter(),
	}

	s.router.Use(middleware.RequestID)
	// s.router.Use(middleware.RealIP) // TODO: look into security implications
	s.router.Use(middleware.Logger) // look at https://github.com/goware/httplog as well
	s.router.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed
	// out and further processing should be stopped.
	s.router.Use(middleware.Timeout(time.Second * 10))

	s.router.Use(middleware.Heartbeat("/ping"))
	s.router.Use(middleware.Throttle(100))

	s.setupRoutes()

	return s
}
