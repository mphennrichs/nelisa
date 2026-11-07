# Build multi-stage: binário estático em distroless, sem shell, usuário sem
# privilégio. O mesmo binário roda os modos api, worker e migrate.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG CHANGELOG=""
RUN if [ -n "$CHANGELOG" ]; then printf '%s\n' "$CHANGELOG" > internal/platform/buildinfo/changelog.txt; fi
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/mphennrichs/nelisa/api/internal/platform/buildinfo.Version=${VERSION} -X github.com/mphennrichs/nelisa/api/internal/platform/buildinfo.Commit=${COMMIT} -X github.com/mphennrichs/nelisa/api/internal/platform/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/nelisa ./cmd/nelisa

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nelisa /nelisa
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/nelisa", "healthcheck"]
ENTRYPOINT ["/nelisa"]
CMD ["api"]
