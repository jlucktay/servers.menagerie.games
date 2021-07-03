package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

func (s *Server) manageGetHandler() http.HandlerFunc {
	var (
		init      sync.Once
		pageBytes []byte
	)

	return func(w http.ResponseWriter, r *http.Request) {
		init.Do(func() {
			if err := getBlob(s.Config.Manage.Bucket, s.Config.Manage.Object, &pageBytes); err != nil {
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
