# Estágio 1: Build da aplicação Go
FROM golang:1.27-alpine AS builder

WORKDIR /app

# Copia dependências primeiro para aproveitar cache de camadas do Docker
COPY go.mod go.sum ./
RUN go mod download

# Copia todo o código fonte
COPY . .

# Compila o binário estático da aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/api/main.go

# Estágio 2: Imagem final enxuta para execução
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache tzdata

# Copia o binário gerado no estágio de build
COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]