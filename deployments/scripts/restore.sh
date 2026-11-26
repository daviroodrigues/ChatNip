#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

source "$SCRIPT_DIR/libs/common.sh" 2>/dev/null || true

BACKUP_DIR="${REPO_ROOT}/volumes/backups"
CONTAINER_APP="chatnip-core"
DB_PATH_IN_CONTAINER="/data/whatsapp.db"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

if [ -z "$1" ]; then
    echo -e "${RED}❌ Usage: ./restore.sh <filename-or-relative-path> [path-to-private-key.asc]${NC}"
    echo -e "   Example: ./restore.sh daily/whatsapp-backup-2026-11-10_02-00-00.sqlite.gz.gpg"
    echo -e "   Example: ./restore.sh daily/whatsapp-backup-2026-11-10_02-00-00.sqlite.gz.gpg /tmp/my-key.asc"
    echo -e "   Piped key: cat my-key.asc | ./restore.sh daily/whatsapp-backup-2026-11-10_02-00-00.sqlite.gz.gpg -"
    exit 1
fi

TARGET_INPUT="$1"
GPG_KEY_ARG="$2"
BACKUP_FILE=""

if [ -f "$TARGET_INPUT" ]; then
    BACKUP_FILE="$TARGET_INPUT"
elif [ -f "${BACKUP_DIR}/$TARGET_INPUT" ]; then
    BACKUP_FILE="${BACKUP_DIR}/$TARGET_INPUT"
else
    FOUND=$(find "$BACKUP_DIR" -type f -name "$(basename "$TARGET_INPUT")" 2>/dev/null | head -n 1)
    if [ -n "$FOUND" ] && [ -f "$FOUND" ]; then
        BACKUP_FILE="$FOUND"
    fi
fi

if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
    echo -e "${RED}❌ Backup file not found locally: $TARGET_INPUT${NC}"
    echo -e "${YELLOW}Tip: Use './manager.sh' to download it from S3 first.${NC}"
    exit 1
fi

echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║${NC}${BOLD}            ♻️  CHATNIP RESTORE WIZARD                         ${NC}${CYAN}║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo -e "Selected file: ${BOLD}$BACKUP_FILE${NC}"
echo -e "File size:     ${BOLD}$(du -h "$BACKUP_FILE" | cut -f1)${NC}"
echo ""

echo -e "${RED}${BOLD}⚠️  CRITICAL WARNING!${NC}"
echo -e "You are about to REPLACE the active WhatsApp database with this backup."
echo -e "${RED}All current sessions, active messages and state will be overwritten!${NC}"
echo ""

if [ "$GPG_KEY_ARG" != "-" ]; then
    read -p "Type 'RESTORE' to proceed: " reply
    if [ "$reply" != "RESTORE" ]; then
        echo -e "${YELLOW}Operation cancelled by user.${NC}"
        exit 0
    fi
fi

TEMP_DECRYPTED="/tmp/whatsapp_restore_$$.sqlite.gz"
TEMP_DB="/tmp/whatsapp_restore_$$.db"
TEMP_GPG_HOME=""

cleanup() {
    rm -f "$TEMP_DECRYPTED" "$TEMP_DB"
    if [ -n "$TEMP_GPG_HOME" ] && [ -d "$TEMP_GPG_HOME" ]; then
        find "$TEMP_GPG_HOME" -type f -exec rm -f {} + 2>/dev/null || true
        rm -rf "$TEMP_GPG_HOME"
    fi
}
trap cleanup EXIT

if [[ "$BACKUP_FILE" == *.gpg ]]; then
    echo -e "${YELLOW}🔐 Backup is OpenGPG encrypted (.gpg).${NC}"

    TEMP_GPG_HOME=$(mktemp -d /tmp/chatnip-restore-gpg-XXXXXX)
    chmod 700 "$TEMP_GPG_HOME"

    KEY_IMPORTED=false

    if [ -n "$GPG_KEY_ARG" ]; then
        if [ "$GPG_KEY_ARG" == "-" ]; then
            echo -e "${CYAN}🔑 Reading private key from STDIN...${NC}"
            GNUPGHOME="$TEMP_GPG_HOME" gpg --batch --quiet --import - 2>/dev/null || true
            KEY_IMPORTED=true
        elif [ -f "$GPG_KEY_ARG" ]; then
            echo -e "${CYAN}🔑 Importing private key from: $GPG_KEY_ARG...${NC}"
            GNUPGHOME="$TEMP_GPG_HOME" gpg --batch --quiet --import "$GPG_KEY_ARG" 2>/dev/null || true
            KEY_IMPORTED=true
        fi
    fi

    if [ "$KEY_IMPORTED" = false ]; then
        HOST_HAS_KEY=$(gpg --list-secret-keys --with-colons 2>/dev/null | awk -F: '/^sec/{print $5; exit}')
        if [ -n "$HOST_HAS_KEY" ]; then
            echo -e "${GREEN}🔑 Secret key found in local environment keyring.${NC}"
            USE_HOST_GPG=true
        else
            USE_HOST_GPG=false
            echo -e "${YELLOW}A chave privada não reside neste servidor (por política de segurança).${NC}"
            echo -e "Como deseja fornecer sua chave privada OpenGPG para a restauração?"
            echo -e "  [1] Informar o caminho de um arquivo temporário (ex: /tmp/my-key.asc)"
            echo -e "  [2] Colar o bloco ASCII da chave privada agora pelo terminal"
            read -p "> " key_opt

            if [ "$key_opt" == "1" ]; then
                read -p "Caminho do arquivo da chave privada: " priv_path
                if [ ! -f "$priv_path" ]; then
                    echo -e "${RED}❌ Arquivo não encontrado: $priv_path${NC}"
                    exit 1
                fi
                GNUPGHOME="$TEMP_GPG_HOME" gpg --batch --quiet --import "$priv_path" 2>/dev/null || true
            elif [ "$key_opt" == "2" ]; then
                echo -e "${CYAN}Cole o bloco da chave privada abaixo (pressione ENTER, depois digite 'EOF' ou aperte Ctrl+D):${NC}"
                KEY_BUFFER=""
                while IFS= read -r line; do
                    [ "$line" == "EOF" ] && break
                    KEY_BUFFER+="$line"$'\n'
                done
                echo "$KEY_BUFFER" | GNUPGHOME="$TEMP_GPG_HOME" gpg --batch --quiet --import 2>/dev/null || true
            else
                echo -e "${RED}Opção inválida.${NC}"
                exit 1
            fi
        fi
    fi

    echo -e "${YELLOW}🔓 Decrypting OpenGPG backup...${NC}"
    if [ "$USE_HOST_GPG" == "true" ]; then
        if ! gpg --yes --decrypt --output "$TEMP_DECRYPTED" "$BACKUP_FILE"; then
            echo -e "${RED}❌ Decryption failed with host keyring.${NC}"
            exit 1
        fi
    else
        if ! GNUPGHOME="$TEMP_GPG_HOME" gpg --yes --decrypt --output "$TEMP_DECRYPTED" "$BACKUP_FILE"; then
            echo -e "${RED}❌ Decryption failed. Please check the private key and passphrase.${NC}"
            exit 1
        fi
    fi
    RESTORE_SOURCE="$TEMP_DECRYPTED"

elif [[ "$BACKUP_FILE" == *.enc ]]; then
    echo -e "${YELLOW}🔓 Legacy OpenSSL AES backup detected (.enc)...${NC}"
    if [ -z "$WHATSAPP_BACKUP_PASSWORD" ]; then
        read -s -p "Enter WHATSAPP_BACKUP_PASSWORD: " WHATSAPP_BACKUP_PASSWORD
        echo ""
    fi

    if ! openssl enc -aes-256-cbc -d -pbkdf2 -in "$BACKUP_FILE" -out "$TEMP_DECRYPTED" -pass pass:"$WHATSAPP_BACKUP_PASSWORD" 2>/dev/null; then
        echo -e "${RED}❌ OpenSSL decryption failed: Incorrect password or corrupted file.${NC}"
        exit 1
    fi
    RESTORE_SOURCE="$TEMP_DECRYPTED"

elif [[ "$BACKUP_FILE" == *.gz ]]; then
    RESTORE_SOURCE="$BACKUP_FILE"
else
    echo -e "${RED}❌ Unrecognized backup format. Expected .gpg, .enc, or .gz${NC}"
    exit 1
fi

echo -e "${YELLOW}📦 Decompressing backup archive...${NC}"
if ! gunzip -c "$RESTORE_SOURCE" > "$TEMP_DB"; then
    echo -e "${RED}❌ Decompression failed.${NC}"
    exit 1
fi

if command -v sqlite3 >/dev/null 2>&1; then
    echo -e "${CYAN}🔍 Verifying SQLite database integrity...${NC}"
    INTEGRITY_CHECK=$(sqlite3 "$TEMP_DB" "PRAGMA quick_check;" 2>&1 || echo "failed")
    if [ "$INTEGRITY_CHECK" != "ok" ]; then
        echo -e "${RED}❌ SQLite integrity check failed (${INTEGRITY_CHECK}). Backup file is corrupted!${NC}"
        exit 1
    fi
    echo -e "${GREEN}✅ Database integrity verified (PRAGMA quick_check: ok).${NC}"
fi

echo -e "${YELLOW}🛑 Stopping WhatsApp core service (${CONTAINER_APP})...${NC}"
docker stop "$CONTAINER_APP" 2>/dev/null || true

echo -e "${YELLOW}♻️  Copying restored database to ${DB_PATH_IN_CONTAINER}...${NC}"
docker cp "$TEMP_DB" "${CONTAINER_APP}:${DB_PATH_IN_CONTAINER}"

echo -e "${YELLOW}🚀 Restarting WhatsApp core service (${CONTAINER_APP})...${NC}"
docker start "$CONTAINER_APP"

echo -e "${GREEN}🎉 WhatsApp restore completed successfully!${NC}"
echo -e "Ephemeral decryption keys have been wiped from memory."
