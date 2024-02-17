package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

var (
	ErrTokenInvalid          = errors.New("could not validate ID token")
	ErrTokenAudienceMismatch = errors.New("token audience does not match this app's client ID")
	ErrTokenIssuer           = errors.New("token was not issued by Google Accounts")
	ErrTokenExpired          = errors.New("token already expired")
	ErrTokenFromFuture       = errors.New("token is issued in the future")
)

// verifyIntegrity checks that the criteria specified at the following link are satisfied:
// https://developers.google.com/identity/sign-in/web/backend-auth#verify-the-integrity-of-the-id-token
func verifyIntegrity(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
	/*
		The ID token is properly signed by Google.
		Use Google's public keys (available in JWK or PEM format) to verify the token's signature.
		These keys are regularly rotated; examine the `Cache-Control` header in the response to determine when you should
		retrieve them again.
	*/
	idtPayload, err := idtoken.Validate(ctx, idToken, audience)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}

	/*
		The value of `aud` in the ID token is equal to one of your app's client IDs.
		This check is necessary to prevent ID tokens issued to a malicious app being used to access data about the same
		user on your app's backend server.
	*/
	// This check should already have been made inside idtoken.Validate() above.
	if idtPayload.Audience != audience {
		return nil, fmt.Errorf("%w: %s", ErrTokenAudienceMismatch, idtPayload.Audience)
	}

	/*
		The value of `iss` in the ID token is equal to `accounts.google.com` or `https://accounts.google.com`.
	*/
	if !strings.HasSuffix(idtPayload.Issuer, "accounts.google.com") {
		return nil, fmt.Errorf("%w: %s", ErrTokenIssuer, idtPayload.Issuer)
	}

	/*
		The expiry time (`exp`) of the ID token has not passed.
	*/
	tokenExpires := time.Unix(idtPayload.Expires, 0)
	if tokenExpires.Before(time.Now()) {
		return nil, fmt.Errorf("%w: %s", ErrTokenExpired, tokenExpires)
	}

	// Make sure the ID token was issued in the past
	tokenIssuedAt := time.Unix(idtPayload.IssuedAt, 0)
	if tokenIssuedAt.After(time.Now()) {
		return nil, fmt.Errorf("%w: %s", ErrTokenFromFuture, tokenIssuedAt)
	}

	/*
		If you want to restrict access to only members of your G Suite domain, verify that the ID token has an `hd` claim
		that matches your G Suite domain name.
	*/

	// Everything checks out!

	// Log the subject (and their email address) from the ID token
	log.Printf("verified token for subject '%s' (email: '%s')", idtPayload.Subject, idtPayload.Claims["email"])

	return idtPayload, nil
}
