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
    PUBLIC_BASE_URL=http://localhost:8080 \
    ROUND_DURATION_SECONDS=90 \
    ROOM_CODE_LENGTH=6 \
    MAX_CHAT_MESSAGES=80
CMD ["/app/server"]
