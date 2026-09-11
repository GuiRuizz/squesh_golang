# 🥬 Squesh API - Golang

API RESTful do ecossistema **Squesh**, desenvolvida em Go (Golang) com o framework Gin e ORM GORM. A aplicação gerencia autenticação de usuários, postagens, comentários e trilhas de aprendizagem.

---

## 🛠️ Tecnologias Utilizadas

- **Linguagem:** Go (Golang) 1.22+
- **Framework Web:** [Gin Web Framework](https://gin-gonic.com/)
- **ORM:** [GORM](https://gorm.io/)
- **Banco de Dados:** PostgreSQL 15
- **Autenticação:** JWT (JSON Web Tokens) & Bcrypt
- **Live Reload:** Air
- **Containerização:** Docker & Docker Compose

---

## 📂 Estrutura do Projeto

```text
.
├── cmd/
│   └── api/
│       └── main.go           # Ponto de entrada da aplicação
├── internal/
│   ├── database/             # Configuração e conexão do PostgreSQL via GORM
│   ├── domain/               # Entidades/Modelos do banco de dados (user.go, post.go, trail.go, etc.)
│   ├── dto/                  # Data Transfer Objects (Validação de entrada)
│   ├── handler/              # Controladores de rotas (Auth, Post, Trail, etc.)
│   ├── middleware/           # Middlewares de autenticação JWT, Admin e CORS
│   └── utils/                # Funções utilitárias (Geração e validação de tokens JWT)
├── .air.toml                 # Configuração do Live Reload com Air
├── .env / .env.example       # Arquivos de configuração de ambiente
├── docker-compose.yml        # Orquestração do banco e da API
├── Dockerfile                # Configuração do ambiente Go para desenvolvimento
└── go.mod / go.sum           # Dependências do módulo Go
```

---

## 🚀 Como Rodar a Aplicação

### Pré-requisitos

- [Docker](https://www.docker.com/) e [Docker Compose](https://docs.docker.com/compose/) instalados.

### Passos para Execução

1. **Clonar o Repositório:**
   ```bash
   git clone https://github.com/seu-usuario/squesh_golang.git
   cd squesh_golang
   ```

2. **Configurar as Variáveis de Ambiente:**
   Crie o arquivo `.env` na raiz do projeto copiado a partir do `.env.example`:
   ```bash
   cp .env.example .env
   ```

3. **Subir os Containers com Docker Compose:**
   ```bash
   docker compose up -d --build
   ```

4. **Verificar os Logs da Aplicação (Live Reload):**
   ```bash
   docker logs -f squesh_api
   ```

A API estará rodando em `http://localhost:8080`. Com a integração do **Air**, qualquer alteração salva em arquivos `.go` recompilará a aplicação automaticamente sem necessidade de reiniciar o container.

---

## 🔐 Variáveis de Ambiente

As configurações de ambiente são definidas no arquivo `.env`:

| Variável | Descrição | Valor Padrão |
| :--- | :--- | :--- |
| `DB_HOST` | Host do banco de dados | `postgres` (no Docker) / `localhost` |
| `DB_USER` | Usuário do PostgreSQL | `postgres` |
| `DB_PASSWORD` | Senha do PostgreSQL | `postgrespassword` |
| `DB_NAME` | Nome do banco de dados | `squesh_db` |
| `DB_PORT` | Porta do banco de dados | `5432` |
| `JWT_SECRET` | Chave secreta para assinatura dos tokens JWT | `sua_chave_secreta` |
| `PORT` | Porta onde a API será executada | `8080` |

---

## 📌 Principais Endpoints da API

### Autenticação (`/api/v1/auth`)

- `POST /api/v1/auth/register` - Registro de novo usuário (`role: "user"` por padrão)
- `POST /api/v1/auth/login` - Autenticação de usuário e retorno do token JWT

#### Exemplo de Requisição — Register (`POST /api/v1/auth/register`):
```json
{
  "name": "Guilherme Ruiz",
  "email": "guiruiz@squesh.com",
  "password": "suasenhasupersegura"
}
```

#### Exemplo de Resposta — Login (`POST /api/v1/auth/login`):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "name": "Guilherme Ruiz",
    "email": "guiruiz@squesh.com",
    "role": "admin"
  }
}
```

---

### Posts (`/api/v1/posts`) — *Requer Token JWT*

- `GET /api/v1/posts` - Lista todas as postagens (Público)
- `POST /api/v1/posts` - Cria uma nova postagem
- `PUT /api/v1/posts/:id` - Atualiza a legenda de uma postagem
- `DELETE /api/v1/posts/:id` - Remove uma postagem

---

### Trilhas (`/api/v1/trails`)

- `GET /api/v1/trails` - Lista todas as trilhas (suporta filtro por tipo: `?type=workout` ou `?type=nutrition`)
- `GET /api/v1/trails/:id` - Obtém os detalhes de uma trilha específica e seus itens
- `POST /api/v1/trails` - Cria uma nova trilha (*Requer perfil Admin*)
- `POST /api/v1/trails/:id/items` - Adiciona um novo item à trilha (*Requer perfil Admin*)

#### Exemplo de Requisição — Criar Trilha (`POST /api/v1/trails`):
```json
{
  "title": "Hipertrofia Iniciante - Peito e Tríceps",
  "description": "Trilha focada no ganho de massa muscular para iniciantes",
  "type": "workout",
  "level": "iniciante"
}
```

---

## 🛠️ Comandos Úteis

- **Parar os containers:**
  ```bash
  docker compose down
  ```
- **Parar e remover volumes (reseta o banco de dados):**
  ```bash
  docker compose down -v
  ```
- **Limpar travamento de recursos do WSL 2 / Docker Desktop (PowerShell Admin):**
  ```powershell
  wsl --shutdown
  ```
- **Rodar a API localmente (sem Docker):**
  ```bash
  go run cmd/api/main.go
  ```