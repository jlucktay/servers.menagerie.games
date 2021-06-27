package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

func (s *Server) manageGetHandler() http.HandlerFunc {
	var (
		init      sync.Once
		pageBytes []byte
	)

	return func(w http.ResponseWriter, r *http.Request) {
		init.Do(func() {
			log.Printf("Downloading blob '%s'...", path.Join(s.Config.Manage.Bucket, s.Config.Manage.Object))

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			client, err := storage.NewClient(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}
			defer func() {
				if errClose := client.Close(); errClose != nil {
					log.Print(errClose)
				}
			}()

			rc, err := client.Bucket(s.Config.Manage.Bucket).Object(s.Config.Manage.Object).NewReader(ctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}
			defer func() {
				if errClose := rc.Close(); errClose != nil {
					log.Print(errClose)
				}
			}()

			data, err := ioutil.ReadAll(rc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Print(err)

				return
			}

			log.Printf("Blob '%s' downloaded", path.Join(s.Config.Manage.Bucket, s.Config.Manage.Object))

			pageBytes = append(pageBytes, data...)
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
