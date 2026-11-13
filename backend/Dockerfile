FROM golang:1.26.2-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git build-base

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o whatsapp-engine ./cmd/api/main.go

FROM alpine:3.23.3

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata ffmpeg

COPY --from=builder /app/whatsapp-engine .

EXPOSE 80

CMD ["./whatsapp-engine"]
