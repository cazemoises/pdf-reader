FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/server ./cmd/server

FROM build AS test
CMD ["go", "test", "./..."]

FROM gcr.io/distroless/static-debian12 AS production
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
