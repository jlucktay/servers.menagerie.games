package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/spf13/viper"
	"google.golang.org/api/compute/v1"
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
			locations, err := getLocationsFromStorage(r.Context(), s.Config.Manage.Bucket, s.Config.Manage.Object)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}

			data := struct {
				Locations []location
			}{
				Locations: locations,
			}
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
	svc, err := compute.NewService(r.Context())
	if err != nil {
		log.Printf("could not create new Compute service: %v", err)

		return
	}

	tmplListCall := svc.InstanceTemplates.List(viper.GetString("CLOUDSDK_CORE_PROJECT"))

	list, err := tmplListCall.Do()
	if err != nil {
		log.Printf("could not list instance templates from Compute service: %v", err)

		return
	}

	if _, err := w.Write([]byte(fmt.Sprintf("POST /manage\n%#v", list.Items))); err != nil {
		log.Printf("could not write bytes to response: %v", err)

		return
	}
}
