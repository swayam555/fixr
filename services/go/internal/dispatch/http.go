package dispatch

import (
	"errors"
	"net/http"

	"fixr.in/svc/internal/platform"
)

func (e *Engine) AcceptHandler(w http.ResponseWriter, r *http.Request) {
	pro := platform.ClaimsFrom(r.Context()).Subject
	of, err := e.Accept(r.Context(), r.PathValue("id"), pro)
	switch {
	case errors.Is(err, ErrOfferGone):
		platform.WriteError(w, http.StatusGone, "OFFER_GONE", "This job was offered to someone else")
	case err != nil:
		platform.WriteError(w, http.StatusServiceUnavailable, "RETRY", "Could not confirm, try again")
	default:
		platform.WriteJSON(w, http.StatusOK, of)
	}
}

func (e *Engine) DeclineHandler(w http.ResponseWriter, r *http.Request) {
	pro := platform.ClaimsFrom(r.Context()).Subject
	err := e.Decline(r.Context(), r.PathValue("id"), pro)
	switch {
	case errors.Is(err, ErrOfferGone):
		platform.WriteError(w, http.StatusGone, "OFFER_GONE", "Offer already closed")
	case err != nil:
		platform.WriteError(w, http.StatusServiceUnavailable, "RETRY", "Try again")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
