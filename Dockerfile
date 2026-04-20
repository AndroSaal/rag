FROM golang:1.23-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/rag-server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app

COPY --from=builder /out/rag-server /app/rag-server

EXPOSE 8080
ENTRYPOINT ["/app/rag-server"]
