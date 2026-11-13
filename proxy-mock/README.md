# Proxy Mock (Squid Proxy Autenticado)

Serviço de proxy HTTP/HTTPS com autenticação básica (*Basic Auth*), projetado para simular provedores de proxy residenciais/datacenter (como Oxylabs, BrightData, etc.) no ambiente de desenvolvimento e testes do ecossistema **Guara ERP**.

---

## 1. Visão Geral e Funcionamento

O `proxy-mock` executa uma instância do **Squid** configurada com:
- **Autenticação obrigatória:** Baseada no módulo `basic_ncsa_auth` com hashing Apache MD5 (`$apr1$`).
- **Cache desabilitado:** Para simular com fidelidade requisições de saída diretas e sem interferência de cache.
- **Portas duplas suportadas:**
  - `3128`: Porta padrão do Squid (mapeada internamente na rede Docker).
  - `3127`: Porta mapeada para o Host (`localhost:3127`) e também escutada internamente para compatibilidade com rotas legadas.

---

## 2. Variáveis de Ambiente (.env)

As credenciais e portas são configuradas centralmente no arquivo `.env` na raiz do projeto:

| Variável | Padrão | Descrição |
| :--- | :--- | :--- |
| `PROXY_USER` | `guara` | Usuário para autenticação básica no proxy |
| `PROXY_PASSWORD` | *(dinâmico)* | Senha do proxy (gerada automaticamente se vazia/omitida) |
| `PROXY_HOST_PORT` | `3127` | Porta exposta para a máquina host (`localhost`) |
| `PROXY_PORT` | `3128` | Porta do Squid dentro do container e da rede Docker |
| `WHATSAPP_GO_PROXY_URL` | *(composto)* | URL formatada para consumo de serviços dependentes |

### Exemplo no `.env`:
```env
PROXY_USER=guara
PROXY_PASSWORD=eee2a8f2ccaaed5a92dc3b82
PROXY_HOST_PORT=3127
PROXY_PORT=3128
WHATSAPP_GO_PROXY_URL=http://guara:eee2a8f2ccaaed5a92dc3b82@proxy-mock:3128
```

---

## 3. Geração Automática de Credenciais na Inicialização

O container utiliza o script [`entrypoint.sh`](file:///home/daviroodrigues/workspace/guara_erp/proxy-mock/entrypoint.sh):
1. **Verificação de Senha:** Caso `PROXY_PASSWORD` não seja definida no `.env` ou esteja vazia, o container gera automaticamente uma senha segura de 24 caracteres hexadecimais via `openssl rand -hex 12`.
2. **Geração do Hash APR1:** A senha (fornecida ou gerada) é criptografada em tempo real com `openssl passwd -apr1` e gravada no formato htpasswd dentro da pasta de dados em `/etc/squid/data/passwords` (persistida no host em `proxy-mock/data/passwords`).
3. **Log Visível:** Durante a inicialização, o container exibe as credenciais ativas e as URLs de conexão prontas para uso nos logs (`docker compose logs proxy-mock`).

---

## 4. URLs de Conexão

O proxy pode ser consumido em dois contextos:

### A. De dentro da Rede Docker (`guara-network`)
Utilize o hostname do serviço `proxy-mock` e a porta `3128` (ou `3127`):
```text
http://<USUARIO>:<SENHA>@proxy-mock:3128
```
*Exemplo com valores atuais:*
```text
http://guara:eee2a8f2ccaaed5a92dc3b82@proxy-mock:3128
```

### B. A partir do Host (sua máquina / localhost)
Utilize `localhost` e a porta mapeada no host (`3127`):
```text
http://<USUARIO>:<SENHA>@localhost:3127
```
*Exemplo com valores atuais:*
```text
http://guara:eee2a8f2ccaaed5a92dc3b82@localhost:3127
```

---

## 5. Como Iniciar e Gerenciar

### Subir apenas o proxy-mock:
```bash
docker compose up -d proxy-mock
```

### Visualizar logs e credenciais ativas:
```bash
docker compose logs -f proxy-mock
```

### Reiniciar o proxy (após alterar senha no `.env`):
```bash
docker compose up -d --force-recreate proxy-mock
```

### Parar o serviço:
```bash
docker compose stop proxy-mock
```

---

## 6. Como Testar e Validar

### 1. Teste de Sucesso (com autenticação)
```bash
curl -x "http://guara:eee2a8f2ccaaed5a92dc3b82@localhost:3127" https://api.ipify.org?format=json
```
*Resposta esperada: JSON com o seu IP público.*

### 2. Teste de Autenticação Rejeitada (sem credenciais)
```bash
curl -I -x "http://localhost:3127" https://api.ipify.org
```
*Resposta esperada:*
```text
HTTP/1.1 407 Proxy Authentication Required
Proxy-Authenticate: Basic realm="Proxy Mock Oxylabs"
```

### 3. Teste com Senha Incorreta
```bash
curl -I -x "http://guara:senha_errada@localhost:3127" https://api.ipify.org
```
*Resposta esperada: `HTTP/1.1 407 Proxy Authentication Required`.*

---

## 7. Estrutura de Arquivos

```text
proxy-mock/
├── data/
│   ├── .gitkeep        # Mantém a pasta no Git
│   └── passwords       # Arquivo htpasswd gerenciado dinamicamente (ignorado no Git)
├── entrypoint.sh       # Inicializador que lê variáveis do .env, gera senha se necessário e cria passwords
├── squid.conf          # Configuração do Squid (portas, basic_ncsa_auth, cache off)
├── .gitignore          # Ignora o arquivo de senhas
└── README.md           # Esta documentação
```
