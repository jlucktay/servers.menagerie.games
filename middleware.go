package main

import (
	"log"
	"net/http"
	"sort"
)

func (s *server) authorisedOnly(next http.Handler) http.Handler {
	// Allowlist based on Google account ID
	if !sort.StringsAreSorted(s.cfg.authorisedSubjects) {
		sort.Strings(s.cfg.authorisedSubjects)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get cookie containing JWT
		cToken, err := r.Cookie("token")
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			log.Printf("could not get token cookie: %v", err)

			return
		}

		// Run it through verifyIntegrity
		idtp, err := verifyIntegrity(cToken.Value, s.cfg.audience)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			log.Printf("error verifying token integrity: %v", err)

			return
		}

		i := sort.SearchStrings(s.cfg.authorisedSubjects, idtp.Subject)

		if i >= len(s.cfg.authorisedSubjects) || s.cfg.authorisedSubjects[i] != idtp.Subject {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			log.Printf("subject is not authorised: %s", idtp.Subject)

			return
		}

		next.ServeHTTP(w, r)
	})
}
