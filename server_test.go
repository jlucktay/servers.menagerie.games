package main_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/matryer/is"
	"google.golang.org/api/idtoken"

	main "go.jlucktay.dev/servers.menagerie.games"
)

func TestBodyContains(t *testing.T) {
	is := is.New(t)

	s := main.Server{
		Config: main.Config{
			AuthorisedSubjects: []string{"one", "two", "three"},
			Audience:           "client_id.apps.googleusercontent.com",
		},
		Router: chi.NewRouter(),
		TokenVerifier: func(idToken, audience string) (*idtoken.Payload, error) {
			return &idtoken.Payload{
				Claims: map[string]interface{}{
					"foo":   "bar",
					"baz":   "qux",
					"quux":  "garply",
					"waldo": "fred",
					"plugh": "xyzzy",
				},

				Audience: "audience",
				Issuer:   "issuer",
				Subject:  "subject",

				// 50 year token, from 2000 to 2050
				IssuedAt: 946684800,
				Expires:  4102444800,
			}, nil
		},
	}

	s.Initialise()

	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)

	testCases := map[string]struct {
		method            string
		path              string
		bodyShouldContain string
	}{
		"Look for the client ID in the root page": {
			method:            http.MethodGet,
			path:              "/",
			bodyShouldContain: `<meta name="google-signin-client_id" content="` + s.Config.Audience + `" />`,
		},
	}
	for name, tC := range testCases {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), tC.method, ts.URL+"/", nil)
			is.NoErr(err)

			resp, err := http.DefaultClient.Do(req)
			is.NoErr(err)

			respBody, err := ioutil.ReadAll(resp.Body)
			is.NoErr(err)
			t.Cleanup(func() { is.NoErr(resp.Body.Close()) })

			is.True(strings.Contains(string(respBody), tC.bodyShouldContain))
		})
	}
}
