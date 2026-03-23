FROM golang:1.26.1 AS build

WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/mam-backend ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/mam-backend /usr/local/bin/mam-backend
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mam-backend"]
