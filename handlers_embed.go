package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
)

//go:embed favicon.ico
var faviconIco []byte

func (s *Server) faviconHandler(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedBytes(faviconIco, w, r)
}

//go:embed robots.txt
var robotsTxt []byte

func (s *Server) robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedBytes(robotsTxt, w, r)
}

func serveEmbeddedBytes(b []byte, w http.ResponseWriter, _ *http.Request) {
	if _, err := w.Write(b); err != nil {
		resp := fmt.Errorf("%s: could not write bytes to ResponseWriter: %w",
			http.StatusText(http.StatusInternalServerError), err)
		http.Error(w, resp.Error(), http.StatusInternalServerError)
		log.Println(resp)

		return
	}
}
