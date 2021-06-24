package main

import (
	"bytes"
	"context"
	"flag"
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
	"github.com/yosssi/gohtml"
	"google.golang.org/api/idtoken"
)

const audienceSuffix = ".apps.googleusercontent.com"

var (
	audience string
	clientID *string
	tpl      *template.Template
)

func main() {
	// default credential flag to env var
	clientID = flag.String("client-id", os.Getenv("GOOGLE_CLIENT_ID"), "Google Client ID")

	// Determine port for HTTP service.
	// https://cloud.google.com/run/docs/reference/container-contract#port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// default address to localhost for development
	address := flag.String("server-address", ":"+port, "Server address to listen on")

	// Lock 'em in
	flag.Parse()

	// Prepare the login page template
	tpl = template.Must(template.New("gsifw.html").ParseFiles("gsifw.html"))

	if *clientID == "" {
		log.Print("missing Google Client ID; set GOOGLE_CLIENT_ID in env or '--client-id' flag")
		return
	}

	audience = *clientID

	if strings.HasSuffix(*clientID, audienceSuffix) {
		*clientID = strings.TrimSuffix(*clientID, audienceSuffix)
	} else {
		audience += audienceSuffix
	}

	srv := http.Server{
		Addr:         *address,
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

	r.Get("/favicon.ico", faviconHandler)
	r.Get("/", rootPageHandler)
	r.Post("/tokensignin", tokenSignIn)

	return r
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./favicon.ico")
}

func rootPageHandler(w http.ResponseWriter, _ *http.Request) {
	rootPage, err := prepareGSIFWBytes(tpl, *clientID)
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

	// TODO: allowlist based on Google account ID

	// Everything checks out!

	// Log the subject and (alphabetised) claims from the ID token
	claimKeys := make([]string, 0)

	for key := range idtPayload.Claims {
		claimKeys = append(claimKeys, key)

		if !sort.StringsAreSorted(claimKeys) {
			sort.Strings(claimKeys)
		}
	}

	claimValues := make([]string, 0)

	for _, key := range claimKeys {
		claimValues = append(claimValues, fmt.Sprintf("%s=%v", key, idtPayload.Claims[key]))
	}

	log.Printf("verified token for subject '%s'; claims: %s", idtPayload.Subject, strings.Join(claimValues, ","))

	return idtPayload, nil
}
