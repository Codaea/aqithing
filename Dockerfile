FROM golang:1.23.2 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o aqithing .

FROM alpine:3.18

RUN apk add --no-cache libc6-compat

COPY --from=builder /app/aqithing /aqithing

ENTRYPOINT ["/aqithing"]