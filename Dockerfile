# ========================================
# BUFFS ETL Worker - Dockerfile (Go)
# ========================================
# Multi-stage build para imagem otimizada.
#
# Estágio 1 (builder): compila o binário Go
# Estágio 2 (runtime): imagem leve apenas com o binário
# ========================================

# ------ ESTÁGIO 1: BUILD ------
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Instala dependências para compilação
RUN apk add --no-cache git ca-certificates tzdata

# Copia go.mod e go.sum
COPY go.mod go.sum ./

# Download das dependências
RUN go mod download

# Copia código fonte
COPY cmd ./cmd
COPY internal ./internal

# Compila o binário
# - CGO_ENABLED=0: desabilita CGO para imagem scratch
# - GOOS=linux: compila para Linux
# - GOARCH=amd64: arquitetura x86_64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo \
  -o /build/worker ./cmd/worker

# ------ ESTÁGIO 2: RUNTIME ------
FROM alpine:3.19

WORKDIR /app

# Instala CA certificates para HTTPS/TLS
RUN apk add --no-cache ca-certificates tzdata

# Copia o binário compilado do estágio anterior
COPY --from=builder /build/worker /app/worker

# Usuário não-root para segurança
RUN addgroup -g 1000 worker && adduser -D -u 1000 -G worker worker
USER worker

# Comando para iniciar o worker
CMD ["/app/worker"]
