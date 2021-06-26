package main

import (
	"log"
	"net/http"
	"sort"
)

func (s *Server) authorisedOnly(next http.Handler) http.Handler {
	// Allowlist based on Google account ID
	if !sort.StringsAreSorted(s.Config.AuthorisedSubjects) {
		sort.Strings(s.Config.AuthorisedSubjects)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get cookie containing JWT
		cToken, err := r.Cookie("token")
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			log.Printf("could not get token cookie: %v", err)

			return
		}

		// Run it through verification
		idtp, err := s.TokenVerifier(cToken.Value, s.Config.ClientID)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			log.Printf("error verifying token integrity: %v", err)

			return
		}

		i := sort.SearchStrings(s.Config.AuthorisedSubjects, idtp.Subject)

		if i >= len(s.Config.AuthorisedSubjects) || s.Config.AuthorisedSubjects[i] != idtp.Subject {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			log.Printf("subject is not authorised: %s", idtp.Subject)

			return
		}

		next.ServeHTTP(w, r)
	})
}
