#!/bin/bash
set -e

# Carrega valores padrões caso não definidos no ambiente
PROXY_USER="${PROXY_USER:-guara}"

# Se PROXY_PASSWORD não foi informada no .env ou ambiente, gera automaticamente
if [ -z "$PROXY_PASSWORD" ]; then
    PROXY_PASSWORD=$(openssl rand -hex 12)
    echo "===================================================================="
    echo "[PROXY-MOCK] AVISO: PROXY_PASSWORD não foi definida no .env/ambiente."
    echo "[PROXY-MOCK] Senha temporária gerada automaticamente:"
    echo "  Usuário:  ${PROXY_USER}"
    echo "  Senha:    ${PROXY_PASSWORD}"
    echo "===================================================================="
else
    echo "===================================================================="
    echo "[PROXY-MOCK] Inicializando com credenciais do .env/ambiente:"
    echo "  Usuário:  ${PROXY_USER}"
    echo "===================================================================="
fi

# Garante que o diretório de dados exista
mkdir -p /etc/squid/data

# Gera arquivo htpasswd para o Squid (Apache MD5 / APR1)
HASH=$(openssl passwd -apr1 "${PROXY_PASSWORD}")
echo "${PROXY_USER}:${HASH}" > /etc/squid/data/passwords
chmod 644 /etc/squid/data/passwords

# Mantém compatibilidade caso algo ainda referencie /etc/squid/passwords
ln -sf /etc/squid/data/passwords /etc/squid/passwords 2>/dev/null || true

HOST_PORT="${PROXY_HOST_PORT:-3127}"
CONTAINER_PORT="${PROXY_PORT:-3128}"

echo "[PROXY-MOCK] Autenticação básica configurada em /etc/squid/data/passwords."
echo "[PROXY-MOCK] URLs de conexão:"
echo "  Rede Docker (outros containers): http://${PROXY_USER}:${PROXY_PASSWORD}@proxy-mock:${CONTAINER_PORT}"
echo "  Host (sua máquina / localhost): http://${PROXY_USER}:${PROXY_PASSWORD}@localhost:${HOST_PORT}"
echo "===================================================================="

# Se argumentos foram passados ao container, repassa; caso contrário, usa flags padrão do Squid
if [ $# -eq 0 ]; then
    exec /usr/local/bin/entrypoint.sh -f /etc/squid/squid.conf -NYC
else
    exec /usr/local/bin/entrypoint.sh "$@"
fi
