FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/bin/server .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/server ./server
COPY config.yaml ./config.yaml
COPY migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["./server"]
