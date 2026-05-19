FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/monitor-bot ./cmd/monitor-bot

FROM alpine:3.21
RUN adduser -D -H -u 10001 appuser && apk add --no-cache ca-certificates
COPY --from=builder /out/monitor-bot /monitor-bot
USER appuser
EXPOSE 8088
ENTRYPOINT ["/monitor-bot"]
