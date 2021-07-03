package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

// location describes the structure of a JSON file that we use to denote which GCP regions and zones are in use by this
// project.
type location struct {
	// Location is a human friendly name for a location.
	Location string `json:"location"`

	// Zone is the Google Cloud name that aligns with Location.
	Zone string `json:"zone"`

	// Default will be true for only one location in the whole file.
	Default bool `json:"default"`
}

func (s *Server) manageGetHandler() http.HandlerFunc {
	var (
		init      sync.Once
		pageBytes []byte
	)

	return func(w http.ResponseWriter, r *http.Request) {
		init.Do(func() {
			locations, err := getLocationsFromStorage(s.Config.Manage.Bucket, s.Config.Manage.Object)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}

			data := struct{ Locations []location }{Locations: locations}
			if err := formatTemplate("manage.gohtml", data, &pageBytes); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

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

func (s *Server) managePostHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("POST /manage")); err != nil {
		log.Print(err)

		return
	}
}
