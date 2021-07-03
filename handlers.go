package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

func (s *Server) faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "favicon.ico")
}

func (s *Server) rootPageHandler(audience string) http.HandlerFunc {
	var (
		init      sync.Once
		pageBytes []byte
	)

	return func(w http.ResponseWriter, r *http.Request) {
		init.Do(func() {
			data := struct {
				Audience string
			}{
				Audience: audience,
			}
			if err := formatTemplate("root.gohtml", data, &pageBytes); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Println(err)

				return
			}
		})

		if _, err := w.Write(pageBytes); err != nil {
			resp := fmt.Errorf("%s: could not write page bytes to ResponseWriter: %w",
				http.StatusText(http.StatusInternalServerError), err)
			http.Error(w, resp.Error(), http.StatusInternalServerError)
			log.Println(resp)

			return
		}
	}
}

// tokenSignInHandler will handle the parsing and verification of a login with a token from GSIFW.
// If successful, a cookie will be set containing the verified token.
func (s *Server) tokenSignInHandler(w http.ResponseWriter, r *http.Request) {
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

	idtp, err := s.TokenVerifier(idToken[0], s.Config.Audience)
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
