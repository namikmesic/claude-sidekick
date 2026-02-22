FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sidekick ./cmd/sidekick

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /sidekick /usr/local/bin/sidekick
EXPOSE 8090
ENTRYPOINT ["sidekick"]
