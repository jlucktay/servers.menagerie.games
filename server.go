package main

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
	"google.golang.org/api/idtoken"
)

// Server holds all of the pertinent components needed for running the app.
type Server struct {
	Router *chi.Mux

	// TokenVerifier describes a func signature that will verify tokens (for specified audiences) passed in, and return
	// an initialised/parsed ID token.
	TokenVerifier func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)

	Config Config
}

// Config holds configuration for Server.
type Config struct {
	Manage             ManageConfig
	Audience           string
	AuthorisedSubjects []string
}

// ManageConfig is scoped to the /manage sub-router.
type ManageConfig struct {
	Bucket, Object string
}

// new sets up and returns a new server.
func new() Server {
	s := Server{
		Config: Config{
			Audience:           viper.GetString("google_client_id") + ".apps.googleusercontent.com",
			AuthorisedSubjects: viper.GetStringSlice("auth_sub"),
			Manage: ManageConfig{
				Bucket: viper.GetString("manage_bucket"),
				Object: viper.GetString("manage_object"),
			},
		},
		Router:        chi.NewRouter(),
		TokenVerifier: verifyIntegrity,
	}

	s.Initialise()

	return s
}
