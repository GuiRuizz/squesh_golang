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
| `ACCESS_TOKEN_EXPIRES` | Tempo de vida do token de acesso (ex.: `15m`, `1h`, `24h`) | `24h` |
| `REFRESH_TOKEN_EXPIRES` | Tempo de vida do refresh token (mantém login mesmo com o App fechado) | `720h` (30 dias) |
| `STORAGE_DRIVER` | Backend de arquivos: `local` (API serve os arquivos) ou `s3` (futuro) | `local` |
| `STORAGE_PUBLIC_BASE_URL` | Base pública das URLs de upload/arquivos geradas (use o endereço externo da API em produção) | `http://localhost:8080` |
| `UPLOAD_DIR` | Diretório onde os arquivos são salvos no driver local | `uploads` |
| `STORAGE_SECRET` | Segredo das assinaturas de upload (opcional; padrão = `JWT_SECRET`) | — |
| `PORT` | Porta onde a API será executada | `8080` |

---

## 📌 Principais Endpoints da API

### Autenticação (`/api/v1/auth`)

- `POST /api/v1/auth/register` - Registro de novo usuário (`role: "user"` por padrão)
- `POST /api/v1/auth/login` - Autenticação de usuário e retorno dos tokens
- `POST /api/v1/auth/refresh` - Renova a sessão usando o `refresh_token` (rotação de token)
- `POST /api/v1/auth/logout` - Revoga o `refresh_token` e encerra a sessão do dispositivo

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
  "refresh_token": "kYx3mP9vQ2xF8sA4dR5tG6hJ7nB1cV0eWzL3uQoPqIrNmZbXcVbNa",
  "expires_in": 86400,
  "token_type": "Bearer",
  "user": {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "name": "Guilherme Ruiz",
    "email": "guiruiz@squesh.com",
    "role": "admin"
  }
}
```

#### Fluxo de Refresh Token

O App recebe **dois** tokens: o `token` (acesso, curta duração) e o `refresh_token` (longa duração). Quando o access token expira, o App chama:

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "kYx3mP9vQ2xF8sA4dR5tG6hJ7nB1cV0eWzL3uQoPqIrNmZbXcVbNa"
}
```

A resposta tem o mesmo formato do login, com um **novo par** de tokens (o antigo refresh token é revogado). O App deve substituir o refresh token salvo pelo novo a cada refresh.

Para sair da conta, o App envia o refresh token em `POST /api/v1/auth/logout` — a partir daí ele não pode mais ser usado para renovar a sessão.

> ⚠️ **Importante (App):** o `refresh_token` deve ser armazenado de forma **segura** no dispositivo (Keychain no iOS, Keystore/EncryptedSharedPreferences no Android), **nunca** em `AsyncStorage`/`localStorage` sem criptografia.

---

### Posts (`/api/v1/posts`)

- `GET /api/v1/posts` - Lista todas as postagens (Público). Com token Bearer, cada post ganha `liked_by_me`; sempre traz `likes_count`
- `GET /api/v1/posts/feed` - **Feed personalizado**: posts de quem você segue + seus posts (*login*)
- `GET /api/v1/posts/:id/comments` - Lista os comentários de um post (Público)
- `GET /api/v1/posts/:id/likes` - Lista os usuários que curtiram o post (Público)
- `POST /api/v1/posts` - Cria uma nova postagem (login) — o `image_url` vem do fluxo de upload assinado
- `POST /api/v1/posts/:id/comments` - Adiciona um comentário ao post (login)
- `DELETE /api/v1/posts/:id/comments/:commentId` - Remove um comentário (apenas o autor ou admin)
- `POST /api/v1/posts/:id/like` - Curte um post (login, idempotente — devolve `likes_count`)
- `DELETE /api/v1/posts/:id/like` - Remove a curtida (login, idempotente)
- `PUT /api/v1/posts/:id` - Atualiza a legenda de uma postagem
- `DELETE /api/v1/posts/:id` - Remove uma postagem

> 💡 Todo post da resposta traz `likes_count`. Quando a requisição tem **Bearer token**, vem também `liked_by_me` (`true/false`) — igual à flag `completed` dos itens de trilha.

---

### Seguidores (`/api/v1/users`)

- `POST /api/v1/users/:id/follow` - Seguir um usuário (login; retorna 400 se for você mesmo; idempotente)
- `DELETE /api/v1/users/:id/follow` - Deixar de seguir (login, idempotente)
- `GET /api/v1/users/:id/followers` - Lista quem segue o usuário (Público)
- `GET /api/v1/users/:id/following` - Lista quem o usuário segue (Público)

---

### Trilhas (`/api/v1/trails`)

- `GET /api/v1/trails` - Lista todas as trilhas (suporta filtro por tipo: `?type=workout` ou `?type=nutrition`)
- `GET /api/v1/trails/:id` - Obtém os detalhes de uma trilha e seus itens; com Bearer token ganha `progress` e a flag `completed` em cada item
- `POST /api/v1/trails` - Cria uma nova trilha (*Requer perfil Admin*)
- `POST /api/v1/trails/:id/items` - Adiciona um novo item à trilha (*Requer perfil Admin*)
- `POST /api/v1/trails/generate` - Gera uma **trilha completa nova** remixando itens de trilhas existentes (*login*)
- `POST /api/v1/trails/:id/generate` - Adiciona N itens genéricos ao final de uma trilha (*login*)
- `GET /api/v1/trails/me/active` - Retorna a **trilha atual** do usuário com o próximo item a concluir e o % de progresso (*login*)
- `GET /api/v1/trails/me/completed` - Lista as trilhas **100% concluídas** pelo usuário (*login*)

#### Geração de Trilha Completa (`POST /api/v1/trails/generate`) — estilo Duolingo

Cria uma trilha nova (título, tipo, nível e itens) a partir do conteúdo já existente, permitindo gerar trilhas infinitas sem criar conteúdo manualmente. O corpo aceita:

```json
{
  "type": "workout",
  "level": "iniciante",
  "item_count": 6,
  "title": "Treino Personalizado"
}
```

Ou, usando uma trilha modelo (herda tipo/nível dela):

```json
{
  "source_trail_id": "d1a53ecf-39d6-42ad-8ccc-557d6e82b907",
  "item_count": 4
}
```

Regras: `source_trail_id` **ou** `type` são obrigatórios; `item_count` padrão 5 (máx. 20); os itens são copiados e embaralhados do pool de trilhas do mesmo tipo/nível (excluindo a trilha modelo); se faltar conteúdo, itens se repetem com "(Variação)" — nunca falta trilha para o usuário.

#### Exemplo de Requisição — Criar Trilha (`POST /api/v1/trails`):
```json
{
  "title": "Hipertrofia Iniciante - Peito e Tríceps",
  "description": "Trilha focada no ganho de massa muscular para iniciantes",
  "type": "workout",
  "level": "iniciante"
}
```

#### Progresso do Usuário (`GET /api/v1/trails/me/active`)

Retorna a trilha que o usuário está fazendo agora (style Duolingo) com o próximo item a concluir:

```json
{
  "active": { "id": "...", "title": "...", "type": "workout", "items": [ ... ] },
  "progress": { "completed_items": 3, "total_items": 6, "percent": 50 },
  "next_item": { "id": "...", "order": 4, "title": "..." }
}
```

Regras: prioriza a trilha **em progresso** (alguns itens concluídos); se nenhuma, devolve a primeira trilha **não iniciada**; se o usuário concluiu tudo, responde `404` orientando a usar `POST /trails/generate`.

---

## 📤 Upload de Imagens (URL Assinada / Presigned)

O fluxo é o mesmo que você usará com S3 depois — o servidor **nunca recebe o binário**:

1. **Pedir a URL assinada** (login):
   ```http
   POST /api/v1/uploads/presign
   Authorization: Bearer <token>
   Content-Type: application/json

   { "filename": "foto.jpg", "content_type": "image/jpeg" }
   ```
   Resposta:
   ```json
   {
     "upload_url": "http://localhost:8080/api/v1/uploads/posts/abc-123.jpg?exp=1690000000&sig=7f3a...",
     "image_url": "http://localhost:8080/api/v1/files/posts/abc-123.jpg",
     "key": "posts/abc-123.jpg",
     "expires_in": 900
   }
   ```
2. **Enviar o arquivo direto na `upload_url`** (sem header de auth — a assinatura é a autenticação):
   ```http
   PUT /api/v1/uploads/posts/abc-123.jpg?exp=1690000000&sig=7f3a...
   Content-Type: image/jpeg
   ```
3. **Criar o post** usando o `image_url` retornado:
   ```json
   { "user_id": "...", "image_url": "http://localhost:8080/api/v1/files/posts/abc-123.jpg", "caption": "Treino de hoje!" }
   ```

Formatos aceitos: `jpg`, `jpeg`, `png`, `webp`, `gif` (máx. 5MB). A URL assinada expira em **15 minutos**.

> 🔁 **Troca futura para S3/MinIO:** ao implementar o driver `s3`, apenas as URLs mudam (bucket + assinatura AWS) — o App continua com o mesmo fluxo de `presign → PUT direto → image_url`. Nenhuma mudança nos endpoints.

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