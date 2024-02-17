package main_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
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
		TokenVerifier: func(_ctx context.Context, _idToken, _audience string) (*idtoken.Payload, error) {
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
		expectedStatus    int
	}{
		"Look for the client ID in the root page": {
			method:            http.MethodGet,
			path:              "/",
			bodyShouldContain: `<meta name="google-signin-client_id" content="` + s.Config.Audience + `" />`,
			expectedStatus:    200,
		},
		"Make sure managing servers requires auth": {
			method:            http.MethodPost,
			path:              "/manage",
			bodyShouldContain: "",
			expectedStatus:    400,
		},
	}
	for name, tC := range testCases {
		t.Run(name, func(t *testing.T) {
			u, err := url.Parse(ts.URL)
			is.NoErr(err)

			u.Path = path.Join(u.Path, tC.path)

			req, err := http.NewRequestWithContext(context.Background(), tC.method, u.String(), nil)
			is.NoErr(err)

			resp, err := http.DefaultClient.Do(req)
			is.NoErr(err)
			is.Equal(tC.expectedStatus, resp.StatusCode)

			respBody, err := io.ReadAll(resp.Body)
			is.NoErr(err)
			t.Cleanup(func() { is.NoErr(resp.Body.Close()) })

			is.True(strings.Contains(string(respBody), tC.bodyShouldContain))
		})
	}
}
