package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Verifier struct {
	kf     keyfunc.Keyfunc
	issuer string
}

// NewVerifier fetches order-service's JWKS and refreshes it in the background, so key rotation needs no restart.
func NewVerifier(ctx context.Context, jwksURL, issuer string) (*Verifier, error) {
	var kf keyfunc.Keyfunc
	var err error
	for attempt := 0; attempt < 30; attempt++ {
		kf, err = keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, err
	}
	return &Verifier{kf: kf, issuer: issuer}, nil
}

// NewStaticVerifier is for tests.
func NewStaticVerifier(kf keyfunc.Keyfunc, issuer string) *Verifier {
	return &Verifier{kf: kf, issuer: issuer}
}

func (v *Verifier) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, v.kf.Keyfunc,
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired(), jwt.WithLeeway(5*time.Second))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

var ErrNoToken = errors.New("missing bearer token")

func BearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok && t != "" {
		return t, nil
	}
	if t := r.URL.Query().Get("access_token"); t != "" {
		return t, nil
	}
	return "", ErrNoToken
}

type ctxKey struct{}

// RequireRole wraps a handler with JWT verification and an optional role check.
func (v *Verifier) RequireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, err := BearerToken(r)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in required")
			return
		}
		c, err := v.Verify(tok)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
			return
		}
		if role != "" && c.Role != role {
			WriteError(w, http.StatusForbidden, "FORBIDDEN", "Wrong account type")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
	}
}

func ClaimsFrom(ctx context.Context) *Claims {
	c, _ := ctx.Value(ctxKey{}).(*Claims)
	return c
}
