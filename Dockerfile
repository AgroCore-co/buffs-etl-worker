# ========================================
# ETL BUFFS - Dockerfile (Go)
# ========================================
# Multi-stage build para imagem otimizada.
# Sem Redis — worker in-memory.
# ========================================

# ------ ESTÁGIO 1: BUILD ------
FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

# Compila o binário
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo \
  -o /build/etl-server ./cmd/server

# ------ ESTÁGIO 2: RUNTIME ------
FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/etl-server /app/etl-server

RUN mkdir -p /app/temp/uploads

RUN addgroup -g 1000 etl && adduser -D -u 1000 -G etl etl
RUN chown -R etl:etl /app
USER etl

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

CMD ["/app/etl-server"]
