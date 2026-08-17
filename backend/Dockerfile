# NOTE: this only builds once cmd/server/main.go exists. Today the backend
# module intentionally has no entrypoint (see internal/domain/doc.go and
# internal/ports/doc.go) - the orchestrator's Dev agent adds cmd/server as
# one of its first backlog tasks. Until then, `docker build` on this
# directory will fail with "no Go files in ./cmd/server", which is expected.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
