package ingest

import (
	"io"
	"log/slog"
	"net/http"

	fixrv1 "fixr.in/svc/gen/fixr/v1"
	"fixr.in/svc/internal/platform"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const maxBatchBytes = 64 << 10
const maxPoints = 200

// Handler accepts application/x-protobuf (the app) or JSON in protojson form (curl, k6, debugging).
func (s *Service) Handler(w http.ResponseWriter, r *http.Request) {
	claims := platform.ClaimsFrom(r.Context())
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBatchBytes+1))
	if err != nil || len(body) > maxBatchBytes {
		platform.WriteError(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "Batch too large")
		return
	}
	batch := &fixrv1.LocationBatch{}
	isProto := r.Header.Get("Content-Type") == "application/x-protobuf"
	if isProto {
		err = proto.Unmarshal(body, batch)
	} else {
		err = protojson.Unmarshal(body, batch)
	}
	if err != nil || len(batch.GetPoints()) == 0 || len(batch.GetPoints()) > maxPoints {
		platform.WriteError(w, http.StatusBadRequest, "BAD_BATCH", "Expected 1-200 points")
		return
	}
	ack, err := s.Ingest(r.Context(), claims.Subject, batch)
	if err != nil {
		slog.Error("ingest failed", "err", err, "pro", claims.Subject)
		platform.WriteError(w, http.StatusServiceUnavailable, "RETRY", "Try again shortly")
		return
	}
	if isProto {
		out, _ := proto.Marshal(ack)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	out, _ := protojson.Marshal(ack)
	_, _ = w.Write(out)
}
