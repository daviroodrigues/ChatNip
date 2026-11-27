# Proxy Mock (Squid Proxy Autenticado)

Serviço de proxy HTTP/HTTPS com autenticação básica (*Basic Auth*), projetado para simular provedores de proxy residenciais/datacenter no ambiente de desenvolvimento e testes do **ChatNip**.

---

## 1. Visão Geral e Funcionamento

O `chatnip-squid-proxy-mock` executa uma instância do **Squid** isolada na rede interna Docker (`chatnip-network`, profile `dev`):
- **Autenticação obrigatória:** Baseada no módulo `basic_ncsa_auth` com hashing Apache MD5 (`$apr1$`).
- **Cache desabilitado:** Para simular requisições de saída diretas e sem interferência de cache.
- **Porta interna:** `3128` (acessível pelos contêineres via alias `proxy-mock:3128`).

---

## 2. Variáveis de Ambiente (`.env`)

As credenciais são configuradas no arquivo `.env` na raiz do projeto e mapeadas no `compose.yml`:

| Variável (`.env`) | Variável no contêiner | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `SQUID_PROXY_USER` | `PROXY_USER` | `chatnip` | Usuário para autenticação básica no proxy |
| `SQUID_PROXY_PASS` | `PROXY_PASSWORD` | *(dinâmico)* | Senha do proxy (gerada automaticamente se vazia/omitida) |

### Exemplo no `.env`:
```env
SQUID_PROXY_USER=chatnip
SQUID_PROXY_PASS=chatnip
```

---

## 3. Geração Automática de Credenciais na Inicialização

O contêiner utiliza o script [`entrypoint.sh`](entrypoint.sh):
1. **Verificação de Senha:** Caso `PROXY_PASSWORD` não seja definida ou esteja vazia, gera automaticamente uma senha segura de 24 caracteres hexadecimais via `openssl rand -hex 12`.
2. **Geração do Hash APR1:** A senha é criptografada com `openssl passwd -apr1` e gravada em `/etc/squid/data/passwords` (persistida em `proxy-mock/data/passwords`).
3. **Log Visível:** Durante a inicialização, o contêiner exibe o usuário ativo e a URL de conexão nos logs (`docker compose --profile dev logs chatnip-squid-proxy-mock`).

---

## 4. URL de Conexão (Rede Docker `chatnip-network`)

Utilize o alias `proxy-mock` e a porta `3128` ao informar `proxy_url` no login de sessão do ChatNip:

```text
http://<SQUID_PROXY_USER>:<SQUID_PROXY_PASS>@proxy-mock:3128
```

*Exemplo com os valores padrão de desenvolvimento:*
```text
http://chatnip:chatnip@proxy-mock:3128
```

---

## 5. Como Iniciar e Gerenciar

```bash
# Subir o proxy mock (profile dev)
docker compose --profile dev up -d chatnip-squid-proxy-mock

# Visualizar logs e credenciais ativas
docker compose --profile dev logs -f chatnip-squid-proxy-mock

# Recriar após alterar credenciais no .env
docker compose --profile dev up -d --force-recreate chatnip-squid-proxy-mock

# Parar o serviço
docker compose --profile dev stop chatnip-squid-proxy-mock
```

---

## 6. Estrutura de Arquivos

```text
proxy-mock/
├── data/
│   ├── .gitkeep        # Mantém a pasta no Git
│   └── passwords       # Arquivo htpasswd gerenciado dinamicamente (ignorado no Git)
├── entrypoint.sh       # Inicializador que gera o arquivo htpasswd a partir das env vars
├── squid.conf          # Configuração do Squid (basic_ncsa_auth, cache off)
├── .gitignore          # Ignora o arquivo de senhas
└── README.md           # Esta documentação
```
