<div align="center">

# ChatNip

### Gateway de WhatsApp & Despachante de Eventos de Nível Corporativo
#### Arquitetura Blindada | Ponto Único de Entrada Nginx | Zero Fallback

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![Docker Ready](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker)](https://www.docker.com/)
[![Security](https://img.shields.io/badge/Segurança-Caixa%20Blindada%20Zero--Trust-success?style=flat-square)](https://cheatsheetseries.owasp.org/)
[![Crypto](https://img.shields.io/badge/Backup-OpenGPG%20Assimétrico-blueviolet?style=flat-square&logo=gnupg)](https://gnupg.org/)
[![Auth](https://img.shields.io/badge/Auth-Argon2id%20OWASP-critical?style=flat-square)](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
[![Storage](https://img.shields.io/badge/Storage-S3%20%2F%20R2%20%2F%20MinIO-orange?style=flat-square&logo=amazon-s3)](https://aws.amazon.com/s3/)
[![License](https://img.shields.io/badge/Licença-MIT-blue?style=flat-square)](LICENSE)

*Infraestrutura de mensageria modular, blindada e de alto desempenho, projetada para integrar ERPs, CRMs e motores de automação em larga escala com isolamento perimetral absoluto.*

[Recursos Principais](#recursos-principais) | [Arquitetura](#arquitetura-do-sistema) | [Segurança Blindada](#segurança-blindada--zero-trust) | [Início Rápido](#início-rápido) | [Referência da API](#referência-da-api)

---

</div>

## Visão Geral

O **ChatNip** redefine a integração corporativa com o ecossistema do WhatsApp. Desenvolvido integralmente em Go e impulsionado pelo motor `whatsmeow`, o sistema opera como uma **caixa blindada autônoma** entre as aplicações de negócio e a complexidade do protocolo ponta a ponta do WhatsApp Web.

O sistema encapsula toda a camada de comunicação em uma API REST limpa e um pipeline de despacho de eventos via AMQP (RabbitMQ), garantindo latência mínima, resiliência máxima e perímetro defensivo inegociável.

---

## Recursos Principais

- 🛡️ **Arquitetura de Caixa Blindada (Single Ingress/Egress Edge):** O **Nginx é o único serviço que expõe portas para o host da máquina**. Nenhuma porta de banco de dados, mensageria ou serviços auxiliares é exposta para o host. Toda a comunicação externa entra e sai estritamente pelo Nginx.
- 🧱 **Isolamento de Egressão Rigoroso:** Sem conexões diretas padrão aos servidores do WhatsApp! Cada sessão exige e recebe seu próprio proxy dedicado durante a inicialização (`proxy_url`), blindando a reputação de IP da infraestrutura principal.
- 🧪 **Ambiente de Desenvolvimento Desacoplado:** RabbitMQ e Squid Proxy são instâncias auxiliares reservadas para desenvolvimento via perfil `--profile dev`. Em produção, o cluster roda enxuto com footprint mínimo.
- ⚙️ **Configuração Determinística (Zero Fallback):** As variáveis de ambiente foram simplificadas e unificadas no padrão universal (S3, AMQP). Não existem valores silenciosos de fallback nem cadeias ambíguas — a aplicação valida todas as dependências no startup.
- 🔐 **Criptografia e Autenticação OWASP:** Separação de privilégios. A chave mestra administrativa (`X-Internal-Key`) opera isolada para provisionamento, enquanto as chaves de clientes (`X-API-Key`) são expostas uma única vez e armazenadas exclusivamente com hashes Argon2id.
- 📦 **Backup Zero-Disk-Secret (GFS + OpenGPG):** Snapshots rotineiros do SQLite a quente, criptografados no servidor utilizando estritamente uma chave pública. A chave privada jamais toca o disco do servidor.
- ⚡ **Despacho Orientado a Eventos (HSEM v3.1):** Pipeline de envio de mensagens desacoplado com suporte a tópicos de fila RabbitMQ, absorvendo picos colossais de demanda.

---

## Arquitetura do Sistema

```mermaid
flowchart TD
    subgraph External["Rede Externa e Clientes"]
        Admin["Administração TI / DevOps"]
        ERP["Sistemas ERP / CRM / Bots"]
        WhatsAppNetwork["Servidores WhatsApp"]
        S3Bucket[("Object Storage\n(AWS S3 / R2 / MinIO)")]
    end

    subgraph ArmoredBox["📦 Caixa Blindada ChatNip (Rede Interna: chatnip-network)"]
        subgraph Perimeter["Perímetro e Borda (Único Ponto com Porta Exposta)"]
            Nginx["chatnip-nginx\n(:${NGINX_PORT} -> :80)"]
        end

        subgraph CoreStack["Serviços de Produção (Sem Portas Públicas)"]
            Core["chatnip-core\n(Go / whatsmeow Engine :80)"]
            SQLite[("SQLite Session Store\n(/data/whatsapp.db)")]
            Backup["chatnip-backup\n(Daemon GFS + OpenGPG)"]
        end

        subgraph DevStack["Serviços de Desenvolvimento (profile: dev)"]
            RabbitMQ[("chatnip-rabbitmq\n(AMQP :5672 | Mgmt :15672)")]
            SquidProxy["chatnip-squid-proxy-mock\n(HTTP Proxy :3128)"]
        end
    end

    Admin -->|"X-Internal-Key"| Nginx
    ERP -->|"X-API-Key"| Nginx

    Nginx -->|"Proxy Reverso (/api/v1, /docs, /health)"| Core
    Nginx -.->|"Management (/rabbitmq/)"| RabbitMQ

    Core --> SQLite
    Core -->|"Tópicos AMQP"| RabbitMQ
    Core -.->|"Egress Dev Mock"| SquidProxy
    Core -->|"Egress Proxy Dedicado"| WhatsAppNetwork

    Backup -.->|"Snapshot SQLite (Leitura)"| SQLite
    Backup -->|"Upload Criptografado OpenGPG"| S3Bucket
```

---

## Segurança Blindada & Zero-Trust

1. **Perímetro de Porta Única:** Apenas o contêiner `chatnip-nginx` possui mapeamento de portas (`ports:`). Todos os outros contêineres (`chatnip-core`, `chatnip-backup`, `chatnip-rabbitmq`, `chatnip-squid-proxy-mock`) operam em rede fechada (`chatnip-network`).
2. **Sem Vazamento para o Host:** Não há uso de `host.docker.internal:host-gateway` nem pontes arbitrárias. Em desenvolvimento, a comunicação com o broker é feita diretamente pelo DNS interno do Docker (`chatnip-rabbitmq:5672`).
3. **Bloqueio de Egressão:** O backend aborta qualquer conexão com os servidores do WhatsApp caso a aplicação cliente não informe uma `proxy_url` válida no payload de login.
4. **Segredo de Disco Zero:** O contêiner de backup retém unicamente a chave pública OpenGPG. Restaurações de desastre exigem injeção externa da chave privada pelo operador em tempo real via terminal ou pipeline seguro.
5. **Zero Fallback:** Sem variáveis ocultas com valores padrão mascarados. A configuração é 100% explícita, facilitando auditoria e eliminando comportamentos inesperados entre ambientes.

---

## Início Rápido

### 1. Preparação do Ambiente

Clone o repositório e crie o seu arquivo `.env`:

```bash
git clone https://github.com/daviroodrigues/ChatNip.git
cd ChatNip

# Copiar o template de configuração
cp .env.example .env && chmod 600 .env
```

Gere as chaves criptográficas obrigatórias (32 caracteres hexadecimais):

```bash
# Executar duas vezes:
# 1. Chave mestre administrativa (WHATSAPP_GO_INTERNAL_KEY)
# 2. Chave de criptografia do banco (WHATSAPP_GO_ENCRYPTION_KEY)
openssl rand -hex 16
```

### 2. Geração do Cofre OpenGPG (Backup)

Gere o par de chaves assimétricas para os snapshots criptografados:

```bash
make generate-keys
```

> **Atenção:** Guarde a chave privada exibida no terminal em um cofre seguro (Bitwarden, 1Password, HashiCorp Vault). O servidor armazena apenas a chave pública (`./keys/backup-public.asc`).

### 3. Execução dos Contêineres

#### Modo Produção (Caixa Blindada Pura)
Sobe estritamente os serviços essenciais (`chatnip-nginx`, `chatnip-core`, `chatnip-backup`):

```bash
docker compose up -d --build
```

#### Modo Desenvolvimento
Sobe também as ferramentas locais de teste e mensageria (`chatnip-rabbitmq` e `chatnip-squid-proxy-mock`):

```bash
docker compose --profile dev up -d --build
```

---

## Portas e Rotas no Nginx

O Nginx centraliza todo o tráfego da aplicação na porta configurada em `NGINX_PORT` (padrão: `80` ou `8085`):

| Rota | Descrição |
| --- | --- |
| `/api/v1/` | Endpoints REST da API do WhatsApp e Autenticação |
| `/health` | Sonda de integridade (Healthcheck) do sistema |
| `/docs` ou `/swagger/index.html` | Documentação OpenAPI / Swagger interativa |
| `/rabbitmq/` | Console Administrativo do RabbitMQ *(somente no perfil `dev`)* |

---

## Modelo de Autenticação

### 1. Plano Administrativo (`X-Internal-Key`)
Utilizado para provisionar e revogar chaves de acesso:

```bash
curl -s -X POST http://localhost:8085/api/v1/auth/keys \
  -H "X-Internal-Key: SUA_CHAVE_MESTRA_AQUI" \
  -H "Content-Type: application/json" \
  -d '{"name": "Integração ERP Produção"}'
```

*A chave gerada (`cn_live_...`) é retornada apenas uma vez. O banco de dados armazena somente a sua derivação irreversível com Argon2id.*

### 2. Plano de Consumo (`X-API-Key`)
Utilizado por microsserviços, ERPs e CRMs para operações de WhatsApp:

```bash
curl -s -X POST http://localhost:8085/api/v1/login \
  -H "X-API-Key: cn_live_SuaChaveGerada..." \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "cliente-alfa-01",
    "tenant_id": "tenant-alfa",
    "proxy_url": "http://user:pass@proxy-residencial.com:3128"
  }'
```

---

## Backup e Recuperação (Disaster Recovery)

O sistema emprega a estratégia **Grandfather-Father-Son (GFS)** com compressão gzip, encriptação assimétrica OpenGPG e sincronização para buckets compatíveis com S3/R2.

### Comandos de Backup

```bash
# Disparar backup manual imediato
make backup

# Abrir o painel interativo do Backup Manager
make backup-manager
```

### Procedimento de Restauração de Crise

```bash
# Modo Interativo (solicitará a chave privada no console)
./deployments/scripts/restore.sh daily/whatsapp-backup-atual.sqlite.gz.gpg

# Modo Pipeline Seguro (via SSH ou Pipe)
cat chave-privada.asc | ./deployments/scripts/restore.sh daily/whatsapp-backup-atual.sqlite.gz.gpg -
```

---

## Referência dos Principais Endpoints

| Endpoint | Método | Descrição |
| --- | :---: | --- |
| `/api/v1/login` | `POST` | Inicia sessão com proxy dedicado e gera QR Code Base64 |
| `/api/v1/login/pair-code` | `POST` | Pareamento direto via código de 8 dígitos |
| `/api/v1/sessions` | `GET` | Lista o status de todas as sessões ativas |
| `/api/v1/proxy` | `PUT` | Rotaciona o proxy de uma sessão em tempo de execução |
| `/api/v1/send` | `POST` | Envio de mensagens (texto, imagem, áudio, documentos, etc.) |
| `/api/v1/messages/edit` | `POST` | Edita mensagens de texto já entregues |
| `/api/v1/messages/reaction` | `POST` | Adiciona ou remove reações com emojis |
| `/api/v1/messages/revoke` | `POST` | Revoga/apaga mensagens para todos |
| `/api/v1/messages/read` | `POST` | Envia recibo de leitura humanizado |
| `/api/v1/status/send` | `POST` | Publica Stories / Status no WhatsApp |
| `/api/v1/groups` | `POST`/`GET` | Cria ou lista grupos |
| `/api/v1/contacts/check` | `POST` | Valida em lote se números possuem WhatsApp ativo |

---

## Tópicos de Eventos no RabbitMQ

| Routing Key | Evento | Descrição |
| --- | --- | --- |
| `msg.receive` | `message` | Mensagens recebidas (texto, mídia, stickers, áudio) |
| `whatsapp.message.status` | `message.status` | Recibos de entrega e leitura (`DELIVERED`, `READ`, `PLAYED`) |
| `whatsapp.chat.presence` | `chat.presence` | Notificações em tempo real (`composing`, `recording`) |
| `whatsapp.call.received` | `call.received` | Alerta de chamadas de voz/vídeo recebidas |
| `whatsapp.message.undecryptable` | `message.undecryptable` | Notificação de falha de decriptação do Signal Protocol |
| `whatsapp.group.updated` | `group.updated` | Atualizações de metadados e membros de grupos |
