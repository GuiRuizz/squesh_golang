# Estágio 1: Build da aplicação Go
FROM golang:1.27-alpine AS builder

WORKDIR /app

# Instala o tzdata (para o fuso horário) e o Air (para o hot-reload)
RUN apk add --no-cache tzdata && \
    go install github.com/air-verse/air@latest

# Copia dependências primeiro para aproveitar o cache de camadas
COPY go.mod go.sum* ./
RUN go mod download

# Copia o código fonte
COPY . .

EXPOSE 8080

# Roda o Air em vez de rodar o binário estático
CMD ["air", "-c", ".air.toml"]