# syntax=docker/dockerfile:1.7
# One image recipe for every Go service: docker build --build-arg SVC=dispatch-service .
FROM golang:1.26.11-trixie AS build
ARG SVC
# only location-processor links h3 (C); the rest build static
ARG CGO=0
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=${CGO} go build -trimpath -ldflags="-s -w" -buildvcs=false -o /out/app ./cmd/${SVC}

FROM gcr.io/distroless/base-debian13:nonroot
COPY --from=build /out/app /app
ENV GOMEMLIMIT=40MiB
ENTRYPOINT ["/app"]
