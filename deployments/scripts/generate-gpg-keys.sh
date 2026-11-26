#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
KEYS_DIR="$REPO_ROOT/keys"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

mkdir -p "$KEYS_DIR"

echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║${NC}${BOLD}            🔐 GERADOR DE CHAVES OPENGPG - CHATNIP            ${NC}${CYAN}║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

if [ -f "$KEYS_DIR/backup-public.asc" ]; then
    echo -e "${YELLOW}⚠️  Já existe uma chave pública em: $KEYS_DIR/backup-public.asc${NC}"
    read -p "Deseja sobrescrever e gerar uma nova chave? [y/N]: " confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        echo -e "${GREEN}Operação cancelada.${NC}"
        exit 0
    fi
fi

KEY_NAME="${1:-ChatNip Backup}"
KEY_EMAIL="${2:-backup@chatnip.local}"
KEY_COMMENT="ChatNip Automated GFS Backup Key"

echo -e "${CYAN}Configurações da chave:${NC}"
echo -e "  • Nome:    ${BOLD}$KEY_NAME${NC}"
echo -e "  • Email:   ${BOLD}$KEY_EMAIL${NC}"
echo -e "  • Tipo:    RSA 4096 bits (Assimétrica OpenGPG)"
echo ""

TEMP_GPG_HOME=$(mktemp -d /tmp/chatnip-gen-gpg-XXXXXX)
chmod 700 "$TEMP_GPG_HOME"

cleanup() {
    rm -rf "$TEMP_GPG_HOME"
}
trap cleanup EXIT

echo -e "${YELLOW}⚙️  Gerando par de chaves OpenGPG (RSA 4096)...${NC}"

GNUPGHOME="$TEMP_GPG_HOME" gpg --batch --gen-key <<EOF
Key-Type: RSA
Key-Length: 4096
Subkey-Type: RSA
Subkey-Length: 4096
Name-Real: $KEY_NAME
Name-Comment: $KEY_COMMENT
Name-Email: $KEY_EMAIL
Expire-Date: 0
%no-protection
%commit
EOF

GNUPGHOME="$TEMP_GPG_HOME" gpg --armor --export "$KEY_EMAIL" > "$KEYS_DIR/backup-public.asc"
chmod 644 "$KEYS_DIR/backup-public.asc"

PRIVATE_KEY_BLOCK=$(GNUPGHOME="$TEMP_GPG_HOME" gpg --armor --export-secret-keys "$KEY_EMAIL")

FINGERPRINT=$(GNUPGHOME="$TEMP_GPG_HOME" gpg --list-keys --with-colons "$KEY_EMAIL" | awk -F: '/^fpr/{print $10; exit}')
KEY_ID=$(GNUPGHOME="$TEMP_GPG_HOME" gpg --list-keys --with-colons "$KEY_EMAIL" | awk -F: '/^pub/{print $5; exit}')

echo ""
echo -e "${GREEN}✅ Chave pública gerada e salva no projeto:${NC}"
echo -e "  📄 ${BOLD}$KEYS_DIR/backup-public.asc${NC}"
echo -e "  🆔 Key ID:      ${CYAN}$KEY_ID${NC}"
echo -e "  🔑 Fingerprint: ${CYAN}$FINGERPRINT${NC}"
echo ""
echo -e "${RED}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${RED}║${NC}${BOLD} 🔐 CHAVE PRIVADA — COPIE AGORA E SALVE NO SEU COFRE SEGURO   ${NC}${RED}║${NC}"
echo -e "${RED}║${NC}   (A chave privada NÃO será salva no disco do servidor!)     ${NC}${RED}║${NC}"
echo -e "${RED}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}$PRIVATE_KEY_BLOCK${NC}"
echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}Instruções de Segurança:${NC}"
echo -e "  1. Copie o bloco acima e salve em um cofre seguro (ex: 1Password, Bitwarden, KeePass)."
echo -e "  2. O servidor agora possui ${BOLD}apenas a chave pública${NC} para criptografar backups."
echo -e "  3. Quando precisar restaurar um backup, a chave privada será solicitada via terminal ou SSH."
echo -e "${GREEN}════════════════════════════════════════════════════════════════${NC}"
echo ""
