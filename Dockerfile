# syntax=docker/dockerfile:1.7
FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /app/web /app/web
EXPOSE 8080
ENV HTTP_ADDR=:8080 \
    STORAGE_DIR=/app/data/jobs \
    PUBLIC_BASE_URL=http://localhost:8080 \
    GEN_PROVIDER=mock
CMD ["/app/server"]
