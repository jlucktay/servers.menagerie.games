package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/yosssi/gohtml"
	"google.golang.org/api/idtoken"
)

const audienceSuffix = ".apps.googleusercontent.com"

var (
	audience string
	tpl      *template.Template

	authorisedSubjects = make([]string, 0)
)

func main() {
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

	// Lock 'em in and bind 'em
	pflag.Parse()

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf("could not bind pflags: %v", err)
	}

	// SMG_AUTH_SUB should be set in the environment like so:
	// $ export SMG_AUTH_SUB="one two three"
	// This will authorise three different subjects.
	authorisedSubjects = viper.GetStringSlice("auth_sub")
	if len(authorisedSubjects) == 0 {
		log.Print("no authorised subjects defined; set SMG_AUTH_SUB in env")

		return
	}

	log.Printf("%d authorised subject(s): %+v", len(authorisedSubjects), authorisedSubjects)

	if viper.GetString("google_client_id") == "" {
		log.Print("missing Google Client ID; set SMG_GOOGLE_CLIENT_ID in env")

		return
	}

	// Prepare the login page template
	tpl = template.Must(template.New("gsifw.html").ParseFiles("gsifw.html"))
	audience = viper.GetString("google_client_id")

	if strings.HasSuffix(viper.GetString("google_client_id"), audienceSuffix) {
		viper.Set("google_client_id", strings.TrimSuffix(viper.GetString("google_client_id"), audienceSuffix))
	} else {
		audience += audienceSuffix
	}

	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      setupRouter(),
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
	}

	// Set up handling for interrupt signal (CTRL+C)
	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Print("interrupt signal received; server beginning shutdown")

		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("error during shutdown: %v", err)
		}

		close(idleConnsClosed)
	}()

	// Start server listening
	log.Printf("server listening on '%s'...", srv.Addr)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("error starting or closing listener: %v", err)
		return
	}

	// Wait for idle connections to close
	<-idleConnsClosed

	log.Print("server has been shutdown, and all (idle) connections closed")
}

func setupRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// r.Use(middleware.RealIP) // TODO: look into security implications
	r.Use(middleware.Logger) // look at https://github.com/goware/httplog as well
	r.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed
	// out and further processing should be stopped.
	r.Use(middleware.Timeout(time.Second * 10))

	r.Use(middleware.Heartbeat("/ping"))
	r.Use(middleware.Throttle(100))

	r.Get("/favicon.ico", faviconHandler) // GET /favicon.ico
	r.Get("/", rootPageHandler)           // GET /
	r.Post("/tokensignin", tokenSignIn)   // POST /tokensignin

	r.Route("/manage", func(r chi.Router) {
		r.Use(authorisedOnly)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) { // GET /manage
			_, err := w.Write([]byte("GET /manage"))
			if err != nil {
				log.Print(err)
			}
		})

		r.Post("/", func(w http.ResponseWriter, r *http.Request) { // POST /manage
			_, err := w.Write([]byte("POST /manage"))
			if err != nil {
				log.Print(err)
			}
		})
	})

	return r
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./favicon.ico")
}

func rootPageHandler(w http.ResponseWriter, _ *http.Request) {
	rootPage, err := prepareGSIFWBytes(tpl, viper.GetString("google_client_id"))
	if err != nil {
		return
	}

	if _, err := w.Write(rootPage); err != nil {
		resp := fmt.Errorf("%s: could not write page bytes to ResponseWriter: %w",
			http.StatusText(http.StatusInternalServerError), err)
		http.Error(w, resp.Error(), http.StatusInternalServerError)
		log.Println(resp)

		return
	}
}

// prepareGSIFWBytes will execute the given template to render the clientID into place, and return a byte-slice
// representation of the root page.
func prepareGSIFWBytes(tpl *template.Template, clientID string) ([]byte, error) {
	data := struct{ ClientID string }{ClientID: clientID}

	b := &bytes.Buffer{}
	if err := tpl.Execute(b, data); err != nil {
		log.Printf("could not execute template into buffer: %v", err)
		return nil, err
	}

	return gohtml.FormatBytes(b.Bytes()), nil
}

func tokenSignIn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		resp := fmt.Errorf("%s: could not parse request form: %w", http.StatusText(http.StatusBadRequest), err)
		http.Error(w, resp.Error(), http.StatusBadRequest)
		log.Println(resp)
		return
	}

	idToken, tokenPresent := r.Form["idtoken"]
	if !tokenPresent {
		resp := fmt.Sprintf("%s: no 'idtoken' in form", http.StatusText(http.StatusBadRequest))
		http.Error(w, resp, http.StatusBadRequest)
		log.Println(resp)
		return
	}

	if len(idToken) != 1 {
		resp := fmt.Sprintf("%s: idtoken slice contains incorrect number of elements",
			http.StatusText(http.StatusBadRequest))
		http.Error(w, resp, http.StatusBadRequest)
		log.Println(resp)
		return
	}

	idtp, err := verifyIntegrity(idToken[0])
	if err != nil {
		resp := fmt.Errorf("%s: could not verify integrity of the ID token: %w",
			http.StatusText(http.StatusBadRequest), err)
		http.Error(w, resp.Error(), http.StatusBadRequest)
		log.Println(resp)
		return
	}

	emailVerified, evOK := idtp.Claims["email_verified"]
	if !evOK {
		return
	}

	if bEmailVerified, ok := emailVerified.(bool); !ok || !bEmailVerified {
		return
	}

	email, ok := idtp.Claims["email"]
	if !ok {
		return
	}

	sEmail, ok := email.(string)
	if !ok || len(sEmail) == 0 {
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	http.SetCookie(w, &http.Cookie{
		Name:  "token",
		Value: idToken[0],

		// Google ID tokens last one hour
		Expires: time.Now().Add(time.Hour),
		MaxAge:  60 * 60,

		HttpOnly: true,
		Secure:   true,

		SameSite: http.SameSiteStrictMode,
	})

	if _, err := w.Write([]byte(sEmail)); err != nil {
		resp := fmt.Errorf("%s: could not write bytes to ResponseWriter: %w",
			http.StatusText(http.StatusInternalServerError), err)
		http.Error(w, resp.Error(), http.StatusInternalServerError)
		log.Println(resp)
		return
	}
}

// verifyIntegrity checks that the criteria specified at the following link are satisfied:
// https://developers.google.com/identity/sign-in/web/backend-auth#verify-the-integrity-of-the-id-token
func verifyIntegrity(idToken string) (*idtoken.Payload, error) {
	/*
		The ID token is properly signed by Google.
		Use Google's public keys (available in JWK or PEM format) to verify the token's signature.
		These keys are regularly rotated; examine the `Cache-Control` header in the response to determine when you should
		retrieve them again.
	*/
	idtPayload, err := idtoken.Validate(context.Background(), idToken, audience)
	if err != nil {
		return nil, fmt.Errorf("could not validate ID token: %w", err)
	}

	/*
		The value of `aud` in the ID token is equal to one of your app's client IDs.
		This check is necessary to prevent ID tokens issued to a malicious app being used to access data about the same
		user on your app's backend server.
	*/
	// This check should already have been made inside idtoken.Validate() above.
	if idtPayload.Audience != audience {
		return nil, fmt.Errorf("token audience '%s' does not match this app's client ID", idtPayload.Audience)
	}

	/*
		The value of `iss` in the ID token is equal to `accounts.google.com` or `https://accounts.google.com`.
	*/
	if !strings.HasSuffix(idtPayload.Issuer, "accounts.google.com") {
		return nil, fmt.Errorf("token was issued by '%s' and not by Google Accounts", idtPayload.Issuer)
	}

	/*
		The expiry time (`exp`) of the ID token has not passed.
	*/
	tokenExpires := time.Unix(idtPayload.Expires, 0)
	if tokenExpires.Before(time.Now()) {
		return nil, fmt.Errorf("token already expired at '%s'", tokenExpires)
	}

	// Make sure the ID token was issued in the past
	tokenIssuedAt := time.Unix(idtPayload.IssuedAt, 0)
	if tokenIssuedAt.After(time.Now()) {
		return nil, fmt.Errorf("token is issued in the future at '%s'", tokenIssuedAt)
	}

	/*
		If you want to restrict access to only members of your G Suite domain, verify that the ID token has an `hd` claim
		that matches your G Suite domain name.
	*/

	// Everything checks out!

	// Log the subject (and their email address) from the ID token
	log.Printf("verified token for subject '%s' (email: '%s')", idtPayload.Subject, idtPayload.Claims["email"])

	return idtPayload, nil
}

func authorisedOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get cookie containing JWT
		cToken, err := r.Cookie("token")
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			log.Printf("could not get token cookie: %v", err)

			return
		}

		// Run it through verifyIntegrity
		idtp, err := verifyIntegrity(cToken.Value)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			log.Printf("error verifying token integrity: %v", err)

			return
		}

		// Allowlist based on Google account ID
		if !sort.StringsAreSorted(authorisedSubjects) {
			sort.Strings(authorisedSubjects)
		}

		i := sort.SearchStrings(authorisedSubjects, idtp.Subject)

		if i >= len(authorisedSubjects) || authorisedSubjects[i] != idtp.Subject {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			log.Printf("subject is not authorised: %s", idtp.Subject)

			return
		}

		next.ServeHTTP(w, r)
	})
}
