package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	viper.SetConfigFile(".env")
	viper.SetEnvPrefix("smg")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.BindEnv("auth_sub"); err != nil {
		log.Fatalf("could not bind to 'SMG_AUTH_SUB' env var: %v", err)
	}

	if err := viper.BindEnv("google_client_id"); err != nil {
		log.Fatalf("could not bind to 'SMG_GOOGLE_CLIENT_ID' env var: %v", err)
	}

	// Determine port for HTTP service.
	// https://cloud.google.com/run/docs/reference/container-contract#port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Set up address flag using port from above
	pflag.String("address", ":"+port, "Server address to listen on")

	// You plug in a new board and turn on the power.
	// If you see smoke coming from the board, turn off the power.
	// You don't have to do any more testing.
	pflag.Bool("smoke", false, "smoke test only; won't start a server")

	// Lock 'em in and bind 'em
	pflag.Parse()

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf("could not bind pflags: %v", err)
	}

	if err := viper.ReadInConfig(); err != nil {
		var (
			pathError  *os.PathError
			viperCFNFE viper.ConfigFileNotFoundError
		)

		if errors.As(err, &viperCFNFE) || errors.As(err, &pathError) {
			log.Printf("no config file found")
		} else {
			// Config file was found but another error was produced
			log.Fatalf("viper could not read in config: %v", err)
		}
	}

	// Make sure we have values in the necessary configs

	// AUTH_SUB should be set in the environment like so:
	//   $ export SMG_AUTH_SUB="one two three"
	// or in an .env file with the following line:
	//   AUTH_SUB="one two three"
	// In both cases, this would authorise three different subjects.
	if len(viper.GetStringSlice("auth_sub")) == 0 {
		log.Fatal("no authorised subjects defined; set SMG_AUTH_SUB in environment " +
			"or AUTH_SUB in the '.env' file")
	}

	log.Printf("%d authorised subject(s): %+v", len(viper.GetStringSlice("auth_sub")), viper.GetStringSlice("auth_sub"))

	// GOOGLE_CLIENT_ID should be set in the environment like so:
	//   $ export SMG_GOOGLE_CLIENT_ID="123456578901-abcdefghijklmnopqrstuvwxyz123456"
	// or in an .env file with the following line:
	//   GOOGLE_CLIENT_ID="123456578901-abcdefghijklmnopqrstuvwxyz123456"
	// Do not include the '.apps.googleusercontent.com' suffix as it is already part of the HTML template.
	if viper.GetString("google_client_id") == "" {
		log.Fatal("missing Google Client ID; set SMG_GOOGLE_CLIENT_ID in environment " +
			"or GOOGLE_CLIENT_ID in the '.env' file")
	}

	// If we're doing a smoke test, don't proceed past this point
	if viper.GetBool("smoke") {
		log.Println("smoke test OK! 👋")

		return
	}

	// Config is done, so set up the server next
	myServer := new()

	httpServer := http.Server{
		Addr:    viper.GetString("address"),
		Handler: myServer.Router,

		ReadTimeout:  time.Second * 5,
		WriteTimeout: time.Second * 10,
		IdleTimeout:  time.Second * 120,
	}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for signals to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	go func() {
		<-sig

		log.Print("interrupt signal received; server beginning graceful shutdown")

		shutdownCtx, shutdownCancel := context.WithTimeout(serverCtx, 30*time.Second)
		defer shutdownCancel()

		go func() {
			<-shutdownCtx.Done()

			if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
				log.Fatal("graceful shutdown timed out, forcing exit")
			}
		}()

		// Start graceful shutdown
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("error during shutdown: %v", err)
		}

		serverStopCtx()
	}()

	// Start server listening
	log.Printf("server listening on '%s'...", httpServer.Addr)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("error from listener: %v", err)

		return
	}

	// Wait for shutdown
	<-serverCtx.Done()

	log.Print("server has been shut down, and all (idle) connections closed")
}
