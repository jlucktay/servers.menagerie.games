package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
	"google.golang.org/api/idtoken"
)

// Server holds all of the pertinent components needed for running the app.
type Server struct {
	Router *chi.Mux

	// TokenVerifier describes a func signature that will verify tokens (for specified audiences) passed in, and return
	// an initialised/parsed ID token.
	TokenVerifier func(idToken, audience string) (*idtoken.Payload, error)

	Config Config
}

// Config holds configuration for Server.
type Config struct {
	ClientID           string
	AuthorisedSubjects []string
}

// new sets up and returns a new server.
func new() Server {
	s := Server{
		Config: Config{
			AuthorisedSubjects: viper.GetStringSlice("auth_sub"),
			ClientID:           viper.GetString("google_client_id"),
		},
		Router:        chi.NewRouter(),
		TokenVerifier: verifyIntegrity,
	}

	s.Initialise()

	return s
}
