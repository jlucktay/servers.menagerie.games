package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/spf13/viper"
	"google.golang.org/api/compute/v1"
)

var ErrNoInstTemplates = errors.New("no instance template(s) found")

var locations []location

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
			var err error
			locations, err = getLocationsFromStorage(r.Context(), s.Config.Manage.Bucket, s.Config.Manage.Object)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}

			svc, err := compute.NewService(r.Context())
			if err != nil {
				log.Printf("could not create new Compute service: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}

			template, err := getInstanceTemplate(r.Context(), svc)
			if err != nil {
				log.Printf("could not get latest template: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}

			data := struct {
				Template  string
				Locations []location
			}{
				Template:  template,
				Locations: locations,
			}
			if err := formatTemplate("manage_get.gohtml", data, &pageBytes); err != nil {
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
	var (
		init      sync.Once
		pageBytes []byte
	)

	init.Do(func() {
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

	svc, err := compute.NewService(r.Context())
	if err != nil {
		log.Printf("could not create new Compute service: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	if errDel := deleteRunningInstances(r.Context(), svc); errDel != nil {
		log.Printf("could not delete running instances: %v", errDel)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	template, err := getInstanceTemplate(r.Context(), svc)
	if err != nil {
		log.Printf("could not get latest template: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	ip, errCreate := createInstanceFromTemplate(r.Context(), svc, template, "TODO: location string; get default")
	if errCreate != nil {
		log.Printf("could not create new instance: %v", errCreate)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	buf := bytes.NewBufferString(fmt.Sprintf("POST /manage\nTemplate: %s\nIP: %s", template, ip))

	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("could not write bytes to response: %v", err)

		return
	}
}

func deleteRunningInstances(ctx context.Context, svc *compute.Service) error {
	wg := sync.WaitGroup{}

	for _, loc := range locations {
		instListCall := svc.Instances.List(viper.GetString("CLOUDSDK_CORE_PROJECT"), loc.Zone)
		instListCall.Context(ctx)

		list, err := instListCall.Do()
		if err != nil {
			return fmt.Errorf("could not list instances from Compute service: %w", err)
		}

		for _, item := range list.Items {
			wg.Add(1)

			go func(name string) {
				defer wg.Done()

				deleteCall := svc.Instances.Delete(viper.GetString("CLOUDSDK_CORE_PROJECT"), loc.Zone, name)
				deleteCall.Context(ctx)

				log.Printf("starting delete operation on instance '%s'", name)

				if _, err := deleteCall.Do(); err != nil {
					log.Printf("could not delete instance from Compute service: %v", err)

					return
				}
			}(item.Name)
		}
	}

	wg.Wait()

	return nil
}

// getInstanceTemplate will look up compute instance templates for the configured Google Cloud Project, and attempt to
// return the name of the most recently created one.
func getInstanceTemplate(ctx context.Context, svc *compute.Service) (string, error) {
	tmplListCall := svc.InstanceTemplates.List(viper.GetString("CLOUDSDK_CORE_PROJECT"))
	tmplListCall.Context(ctx)
	tmplListCall.MaxResults(1)
	tmplListCall.OrderBy("creationTimestamp desc")

	list, err := tmplListCall.Do()
	if err != nil {
		return "", fmt.Errorf("could not list instance templates from Compute service: %w", err)
	}

	if len(list.Items) < 1 {
		return "", ErrNoInstTemplates
	}

	return list.Items[0].Name, nil
}

// Returns the public IP address of the created instance.
func createInstanceFromTemplate(_ctx context.Context, _svc *compute.Service, _template, _location string) (string,
	error,
) {
	return "<placeholder IP address>", nil
}
