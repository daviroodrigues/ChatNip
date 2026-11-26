<div align="center">

# ChatNip

**Gateway de WhatsApp Multi-Tenant & Despachante de Eventos Orientado a Filas**

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)](https://www.docker.com/)
[![RabbitMQ](https://img.shields.io/badge/AMQP-RabbitMQ-FF6600?style=flat-square&logo=rabbitmq&logoColor=white)](https://www.rabbitmq.com/)
[![Auth](https://img.shields.io/badge/Auth-Argon2id%20%2B%20AES--256--GCM-critical?style=flat-square)](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
[![Backup](https://img.shields.io/badge/Backup-GFS%20%2B%20OpenGPG-blueviolet?style=flat-square&logo=gnupg&logoColor=white)](https://gnupg.org/)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue?style=flat-square)](LICENSE)

*Infraestrutura modular em Go sobre `whatsmeow` para integrar ERPs, CRMs e automações ao WhatsApp com isolamento de proxy por sessão, emulação comportamental (HSEM v3.1) e perímetro único via Nginx.*

[Visão Geral](#visão-geral) •
[Recursos](#recursos) •
[Arquitetura](#arquitetura) •
[Início Rápido](#início-rápido) •
[API](#referência-da-api) •
[RabbitMQ](#mensageria--eventos-rabbitmq) •
[Backup](#backup--disaster-recovery) •
[Contribuição](#contribuição)

</div>

---

## Visão Geral

O **ChatNip** é um gateway de WhatsApp corporativo desenvolvido em **Go** utilizando a biblioteca [`whatsmeow`](https://pkg.go.dev/go.mau.fi/whatsmeow). Ele abstrai a complexidade do protocolo multi-dispositivo do WhatsApp Web atrás de uma **API REST** documentada com OpenAPI/Swagger e de um **pipeline assíncrono via RabbitMQ (AMQP)**.

O projeto foi desenhado com foco em previsibilidade operacional, segurança e isolamento de rede:

- Apenas o **Nginx** expõe portas para o host.
- Toda sessão exige seu próprio **proxy HTTP/HTTPS dedicado** (`proxy_url`), evitando compartilhamento de reputação de IP.
- A configuração é **fail-fast** (sem fallbacks silenciosos): variáveis ausentes abortam a inicialização imediatamente.
- O envio de mensagens passa pelo motor **HSEM v3.1** (*Full Spectrum Bio-Emulation*) para serializar interações e simular cadência humana.

---

## Recursos

- **Perímetro Único (Single-Edge Nginx):** `chatnip-nginx` é o único contêiner com `ports:` mapeadas para o host; `chatnip-core`, `chatnip-backup` e serviços auxiliares operam em rede Docker isolada (`chatnip-network`).
- **Isolamento de Egressão por Sessão:** Conexões com o WhatsApp exigem `proxy_url` explícita no login e suportam rotação e validação em tempo de execução (`PUT /api/v1/proxy`, `POST /api/v1/proxy/validate`).
- **HSEM v3.1 (Bio-Emulação):** Serialização por sessão (`SessionSerializer` + `SessionGate` com preempção de *fast-lane*), agrupamento de rajadas e simulação estocástica de leitura, digitação e presença por persona (`PADRAO`, `APRESSADO`, `METICULOSO`, `DISTRAIDO`). Veja a especificação em [`backend/docs/guides/hsem.md`](backend/docs/guides/hsem.md).
- **Pipeline AMQP com Priorização e DLQ:** Filas separadas para baixa latência (`whatsapp_fast_lane`) e fluxo padrão (`whatsapp_standard_lane`), criação de grupos (`whatsapp_group_create`) e *Dead Letter Exchange* (`whatsapp_dlx`).
- **Segurança & Criptografia:**
  - Separação entre chave administrativa mestre (`X-Internal-Key`) e chaves de clientes (`X-API-Key` no formato `cn_live_*`).
  - Chaves de API exibidas apenas uma vez na criação e armazenadas com hash **Argon2id**.
  - Dados sensíveis de sessão (como credenciais de proxy) criptografados em repouso no SQLite com **AES-256-GCM**.
- **Backup Zero-Disk-Secret (GFS + OpenGPG):** Snapshots a quente do SQLite com retenção *Grandfather-Father-Son* (diário, semanal, mensal e anual), criptografia assimétrica OpenGPG usando apenas a chave pública no servidor e sincronização com **S3 / Cloudflare R2 / MinIO**.
- **Processamento de Mídia:** Conversão integrada com `ffmpeg` no contêiner core e ingestão assíncrona com retentativas.

---

## Arquitetura

```mermaid
flowchart TD
    subgraph External["Clientes e Serviços Externos"]
        Admin["Admin / DevOps"]
        Apps["ERPs / CRMs / Bots"]
        Broker[("RabbitMQ Externo\n(AMQP :5672 | Mgmt :15672)")]
        WhatsApp["Servidores WhatsApp"]
        S3[("Object Storage\n(AWS S3 / R2 / MinIO)")]
    end

    subgraph Network["Rede Docker: chatnip-network"]
        Nginx["chatnip-nginx\n(Única porta exposta: ${NGINX_PORT}:80)"]
        Core["chatnip-core\n(Go + whatsmeow + HSEM v3.1)"]
        SQLite[("SQLite\n/data/whatsapp.db")]
        Backup["chatnip-backup\n(Cron GFS + OpenGPG)"]
        Squid["chatnip-squid-proxy-mock\n(profile: dev | :3128)"]
    end

    Admin -->|"X-Internal-Key"| Nginx
    Apps -->|"X-API-Key"| Nginx

    Nginx -->|"/api/v1, /docs, /swagger, /health"| Core
    Nginx -.->|"/rabbitmq/ (dev)"| Broker

    Core --> SQLite
    Core <-->|"AMQP (Filas & Eventos)"| Broker
    Core -.->|"Egress Dev"| Squid
    Core -->|"Proxy Dedicado por Sessão"| WhatsApp

    Backup -.->|"Leitura (:ro)"| SQLite
    Backup -->|"Snapshot .sqlite.gz.gpg"| S3
```

### Componentes

| Serviço | Imagem / Base | Papel | Portas no Host |
| --- | --- | --- | --- |
| `chatnip-nginx` | `nginx:1.25-alpine` | Proxy reverso, CORS, headers de segurança e ponto único de entrada | `${NGINX_PORT}:80` |
| `chatnip-core` | Go `1.26.2` / Alpine `3.23.3` + `ffmpeg` | API REST, workers RabbitMQ, motor `whatsmeow` e HSEM v3.1 | Nenhuma |
| `chatnip-backup` | Alpine + `sqlite`, `gnupg`, `aws-cli` | Snapshots agendados do SQLite, rotação GFS e upload OpenGPG para S3/R2 | Nenhuma |
| `chatnip-squid-proxy-mock` | Squid *(profile `dev`)* | Proxy HTTP/HTTPS autenticado para testes locais de egressão | Nenhuma |

---

## Estrutura do Projeto

```text
chatnip/
├── backend/
│   ├── cmd/api/                 # Entrypoint da aplicação e anotações Swagger
│   ├── docs/                    # Especificação OpenAPI gerada e guia HSEM
│   ├── internal/
│   │   ├── kit/                 # Auth (Argon2id), config, SQLite, AES-256-GCM, RabbitMQ, HTTP
│   │   └── modules/
│   │       ├── hsem/            # Motor de bio-emulação (HSEM v3.1)
│   │       └── whatsapp/        # Domínio, handlers HTTP, workers AMQP e adaptador whatsmeow
│   ├── Dockerfile
│   └── go.mod
├── database/migrations/         # Migrações SQL versionadas
├── deployments/
│   ├── docker/                  # Dockerfile e entrypoint do serviço de backup
│   └── scripts/                 # Scripts de backup GFS, geração de chaves GPG, manager e restore
├── keys/                        # Chave pública OpenGPG (backup-public.asc)
├── nginx/nginx.conf             # Configuração do proxy reverso
├── proxy-mock/                  # Squid proxy autenticado para ambiente dev
├── compose.yml                  # Orquestração dos contêineres
├── Makefile                     # Automação de build, testes, swagger e backup
└── .env.example                 # Template de variáveis de ambiente
```

---

## Início Rápido

### Pré-requisitos

- **Docker** e **Docker Compose** (v2+)
- Instância **RabbitMQ** acessível pelo `chatnip-core` (via `RABBITMQ_URL`)
- **OpenSSL**, **GnuPG** e **Make** *(recomendados para geração de chaves e utilitários)*

### 1. Clonar e configurar o ambiente

```bash
git clone https://github.com/daviroodrigues/ChatNip.git
cd ChatNip

cp .env.example .env
chmod 600 .env
```

Gere duas chaves hexadecimais de 32 caracteres para `WHATSAPP_GO_INTERNAL_KEY` e `WHATSAPP_GO_ENCRYPTION_KEY`:

```bash
openssl rand -hex 16
openssl rand -hex 16
```

Edite o `.env` preenchendo as chaves geradas, a URL do RabbitMQ (`RABBITMQ_URL`) e as credenciais do bucket S3/R2/MinIO caso utilize backup remoto.

### 2. Gerar par de chaves OpenGPG (Backup)

```bash
make generate-keys
```

> **Importante:** Salve a **chave privada** exibida no terminal em um cofre seguro (1Password, Bitwarden, Vault). Apenas a chave pública (`./keys/backup-public.asc`) permanece no servidor.

### 3. Subir os serviços

**Produção (Nginx + Core + Backup):**

```bash
docker compose up -d --build
```

**Desenvolvimento (inclui `chatnip-squid-proxy-mock`):**

```bash
docker compose --profile dev up -d --build
```

Verifique a saúde da aplicação:

```bash
curl -s http://localhost:${NGINX_PORT:-80}/health
```

---

## Configuração (`.env`)

Todas as variáveis essenciais do `chatnip-core` são obrigatórias no startup (`config.Load()`):

| Variável | Obrigatória | Descrição |
| --- | :---: | --- |
| `COMPOSE_PROFILES` | Não | Perfis do Compose (`prod` ou `dev`) |
| `ENVIRONMENT` | Sim | `development` (logs legíveis em debug) ou `production` (JSON) |
| `TZ` | Não | Timezone dos contêineres (ex.: `America/Sao_Paulo`) |
| `NGINX_PORT` | Sim | Porta exposta no host pelo Nginx (ex.: `80` ou `8085`) |
| `WHATSAPP_GO_PORT` | Sim | Porta interna da API Go (padrão no Compose: `80`) |
| `WHATSAPP_GO_API_VERSION` | Sim | Prefixo de versão da API (ex.: `v1`) |
| `WHATSAPP_GO_INTERNAL_KEY` | Sim | Chave mestra administrativa (`X-Internal-Key`) |
| `WHATSAPP_GO_ENCRYPTION_KEY` | Sim | Chave de exatamente 32 bytes para criptografia AES-256-GCM |
| `WHATSAPP_GO_DB_PATH` | Sim | Caminho do arquivo SQLite dentro do contêiner (`/data/whatsapp.db`) |
| `RABBITMQ_URL` | Sim | URI AMQP do broker RabbitMQ |
| `BACKEND_API_URL` | Não | URL do backend consumidor para ingestão de mídia |
| `BACKEND_INTERNAL_API_KEY` | Não | Chave interna para comunicação com o backend consumidor |
| `MEDIA_TEMP_DIR` | Não | Diretório temporário para processamento de mídia |
| `SQUID_PROXY_USER` / `SQUID_PROXY_PASS` | Dev | Credenciais do `chatnip-squid-proxy-mock` |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION` / `AWS_ENDPOINT_URL` / `AWS_S3_BUCKET` | Backup | Configuração do Object Storage compatível com S3 |
| `BACKUP_GPG_PUBLIC_KEY_PATH` / `BACKUP_CRON_SCHEDULE` / `BACKUP_RETENTION_*` | Backup | Caminho da chave pública, agendamento cron e política de retenção GFS |

---

## Autenticação & Segurança

### 1. Plano Administrativo (`X-Internal-Key`)

Usado para criar, listar e revogar chaves de acesso de clientes:

```bash
curl -s -X POST http://localhost:80/api/v1/auth/keys \
  -H "X-Internal-Key: SUA_CHAVE_MESTRA" \
  -H "Content-Type: application/json" \
  -d '{"name": "ERP Produção"}'
```

A resposta retorna a chave `cn_live_...` **uma única vez**. Apenas o prefixo mascarado e o hash **Argon2id** são persistidos no SQLite.

### 2. Plano de Consumo (`X-API-Key`)

Usado pelas aplicações integradas para gerenciar sessões e enviar mensagens:

```bash
curl -s -X POST http://localhost:80/api/v1/login \
  -H "X-API-Key: cn_live_SuaChave..." \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "atendimento-01",
    "tenant_id": "empresa-alfa",
    "proxy_url": "http://usuario:senha@seu-proxy.com:3128"
  }'
```

---

## Referência da API

A documentação interativa **Swagger / OpenAPI** fica disponível em `/docs` ou `/swagger/index.html` através do Nginx.

### Rotas no Nginx

| Rota | Destino |
| --- | --- |
| `/api/v1/` | Endpoints REST autenticados da API |
| `/docs` / `/swagger/` | Documentação interativa Swagger UI |
| `/health` | Healthcheck da API e conexão SQLite |
| `/chatnip/` | Alias de proxy reverso com suporte a WebSocket/streaming |
| `/rabbitmq/` | Console de gerenciamento do RabbitMQ *(apenas dev, `host.docker.internal:15672`)* |

### Principais Endpoints

<details>
<summary><strong>Auth (X-Internal-Key)</strong></summary>

| Método | Endpoint | Descrição |
| :---: | --- | --- |
| `POST` | `/api/v1/auth/keys` | Cria uma nova `X-API-Key` |
| `GET` | `/api/v1/auth/keys` | Lista chaves cadastradas (mascaradas) |
| `DELETE` | `/api/v1/auth/keys/{id}` | Revoga uma chave pelo ID |

</details>

<details>
<summary><strong>Sessões & Proxy (X-API-Key)</strong></summary>

| Método | Endpoint | Descrição |
| :---: | --- | --- |
| `POST` | `/api/v1/login` | Inicia sessão com `proxy_url` obrigatória e retorna QR Code |
| `POST` | `/api/v1/login/pair-code` | Pareamento via código de 8 dígitos |
| `POST` | `/api/v1/logout` | Encerra e desconecta a sessão |
| `GET` | `/api/v1/status` | Consulta status de uma sessão |
| `GET` | `/api/v1/sessions` | Lista todas as sessões ativas |
| `PUT` | `/api/v1/proxy` | Atualiza o proxy da sessão em tempo de execução |
| `POST` | `/api/v1/proxy/validate` | Valida conectividade de uma URL de proxy |

</details>

<details>
<summary><strong>Mensagens & Status (X-API-Key)</strong></summary>

| Método | Endpoint | Descrição |
| :---: | --- | --- |
| `POST` | `/api/v1/send` | Envia mensagens (texto, imagem, áudio, vídeo, documento, etc.) |
| `POST` | `/api/v1/sendGroupMessage` | Envio de mensagens para grupos |
| `POST` | `/api/v1/messages/edit` | Edita mensagem de texto enviada |
| `POST` | `/api/v1/messages/reaction` | Adiciona ou remove reação com emoji |
| `POST` | `/api/v1/messages/revoke` | Apaga mensagem para todos |
| `POST` | `/api/v1/messages/read` | Envia recibo de leitura |
| `POST` | `/api/v1/status/send` | Publica Status / Stories |

</details>

<details>
<summary><strong>Grupos, Contatos, Chats & Canais (X-API-Key)</strong></summary>

- **Grupos:** `POST/GET /api/v1/groups`, `GET /api/v1/groups/{jid}`, `PUT /api/v1/groups/{jid}/name|topic|photo|settings/info`, `DELETE /api/v1/groups/{jid}/photo`, `PUT /api/v1/groups/settings/messages`, `POST /api/v1/groups/{jid}/participants|leave|invite/revoke`, `GET /api/v1/groups/{jid}/invite`, `GET /api/v1/groups/invites/{code}`, `POST /api/v1/groups/join|join-invite`.
- **Contatos, Chats & Bloqueio:** `POST /api/v1/contacts/check|resolve`, `GET /api/v1/contacts`, `GET /api/v1/contacts/{jid}/info`, `PUT /api/v1/chats/{jid}/pin|mute|archive`, `DELETE /api/v1/chats/{jid}`, `GET/POST /api/v1/blocklist`.
- **Canais (Newsletters):** `POST/GET /api/v1/channels`, `POST /api/v1/channels/{jid}/send|follow|unfollow`.

</details>

---

## Mensageria & Eventos (RabbitMQ)

O serviço declara as exchanges `whatsapp_events` (`topic`) e `whatsapp_dlx` (`direct`, routing key `msg.failed`).

### Filas Consumidas (Comandos)

| Fila | Routing Key | Workers | Prefetch | Uso |
| --- | --- | :---: | :---: | --- |
| `whatsapp_fast_lane` | `msg.send.priority` | 15 | 10 | Envios prioritários de baixa latência |
| `whatsapp_standard_lane` | `msg.send.normal` | 3 | 1 | Envios padrão serializados |
| `whatsapp_group_create` | `group.create` | 2 | - | Criação assíncrona de grupos |

### Eventos Publicados (`whatsapp_events`)

| Routing Key | Descrição |
| --- | --- |
| `whatsapp.message.received` | Mensagens recebidas (texto, mídia, áudio, stickers) — vinculada à fila `whatsapp_incoming_messages` |
| `whatsapp.message.sent` | Confirmação de despacho de mensagem enviada |
| `whatsapp.message.status` | Recibos de entrega e leitura (`DELIVERED`, `READ`, `PLAYED`) |
| `whatsapp.message.failed` | Falha definitiva após esgotar retentativas |
| `whatsapp.message.undecryptable` | Falha de decriptação do Signal Protocol |
| `whatsapp.chat.presence` | Atualizações de presença em tempo real (`composing`, `recording`) |
| `whatsapp.call.received` | Chamada de voz ou vídeo recebida |
| `whatsapp.call.terminated` | Chamada encerrada |
| `whatsapp.group.updated` | Alterações de metadados ou participantes de grupos |

---

## Backup & Disaster Recovery

Os backups são executados automaticamente pelo contêiner `chatnip-backup` conforme `BACKUP_CRON_SCHEDULE` e podem ser operados manualmente:

```bash
# Disparar backup manual imediato
make backup

# Abrir painel interativo de gerenciamento de backups
make backup-manager
```

### Restauração

```bash
# Modo interativo (solicita a chave privada OpenGPG no terminal)
./deployments/scripts/restore.sh daily/whatsapp-backup-atual.sqlite.gz.gpg

# Via pipe seguro
cat chave-privada.asc | ./deployments/scripts/restore.sh daily/whatsapp-backup-atual.sqlite.gz.gpg -
```

---

## Desenvolvimento Local & Testes

O [`Makefile`](Makefile) centraliza os comandos de desenvolvimento em Go:

```bash
# Baixar e organizar dependências
make deps

# Executar suíte de testes unitários e de integração
make test

# Regenerar documentação Swagger em backend/docs/
make swagger

# Compilar binário nativo em backend/bin/whatsapp-engine
make build

# Executar API localmente (com .env configurado)
make run
```

---

## Contribuição

Contribuições são bem-vindas! Para colaborar com o projeto:

1. Faça um **fork** do repositório e crie uma branch a partir de `main` (`git checkout -b feat/minha-melhoria`).
2. Mantenha o código limpo, aderente às convenções do Go e documentado quando necessário.
3. Execute `make test` garantindo que todos os testes passem antes de abrir o Pull Request.
4. Descreva claramente a motivação e o escopo da mudança no PR.

---

## Licença

Distribuído sob a licença **GNU Affero General Public License v3.0 (AGPL-3.0)**. Veja o arquivo [LICENSE](LICENSE) para mais detalhes. Qualquer modificação ou derivação executada em rede deve disponibilizar o código-fonte completo sob a mesma licença.
