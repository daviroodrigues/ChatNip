#!/bin/bash
set -e

BACKUP_DIR="${BACKUP_DIR:-/backups}"
DATA_DIR="${DATA_DIR:-/data}"
DB_FILE="/data/whatsapp.db"
S3_BUCKET="$AWS_S3_BUCKET"

export AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY"
export AWS_REGION="$AWS_REGION"
AWS_ENDPOINT_URL="$AWS_ENDPOINT_URL"

RETENTION_DAILY="$BACKUP_RETENTION_DAILY"
RETENTION_WEEKLY="$BACKUP_RETENTION_WEEKLY"
RETENTION_MONTHLY="$BACKUP_RETENTION_MONTHLY"
RETENTION_YEARLY="$BACKUP_RETENTION_YEARLY"

WEEKLY_BACKUP_DOW="$BACKUP_WEEKLY_DOW"
MONTHLY_BACKUP_DOM="$BACKUP_MONTHLY_DOM"

GPG_PUBLIC_KEY_PATH="$BACKUP_GPG_PUBLIC_KEY_PATH"
GPG_RECIPIENT=""

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

SELECTED_TIER="auto"
while [[ $# -gt 0 ]]; do
    case "$1" in
        --tier)
            SELECTED_TIER="$2"
            shift 2
            ;;
        --tier=*)
            SELECTED_TIER="${1#*=}"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

if [ -n "$BACKUP_TIER" ]; then
    SELECTED_TIER="$BACKUP_TIER"
fi

if [ ! -f "$DB_FILE" ]; then
    log "${RED}❌ Error: Database not found at $DB_FILE.${NC}"
    exit 1
fi

export GNUPGHOME="${GNUPGHOME:-/tmp/gnupg}"
mkdir -p "$GNUPGHOME"
chmod 700 "$GNUPGHOME"

log "${CYAN}🔑 Configuring OpenGPG Public Key...${NC}"

if [ -f "$GPG_PUBLIC_KEY_PATH" ]; then
    log "Importing public key from $GPG_PUBLIC_KEY_PATH"
    gpg --batch --quiet --import "$GPG_PUBLIC_KEY_PATH" 2>/dev/null || true
elif [ -n "$GPG_PUBLIC_KEY" ]; then
    log "Importing public key from GPG_PUBLIC_KEY environment variable"
    echo "$GPG_PUBLIC_KEY" | gpg --batch --quiet --import 2>/dev/null || true
fi

if [ -z "$GPG_RECIPIENT" ]; then
    GPG_RECIPIENT=$(gpg --list-keys --with-colons 2>/dev/null | awk -F: '/^pub/{print $5; exit}')
fi

if [ -z "$GPG_RECIPIENT" ]; then
    log "${RED}❌ Error: No OpenGPG public key available for encryption.${NC}"
    log "${RED}Please mount your public key at $GPG_PUBLIC_KEY_PATH or set GPG_PUBLIC_KEY / GPG_RECIPIENT.${NC}"
    log "${YELLOW}Tip: Run 'make generate-keys' or 'deployments/scripts/generate-gpg-keys.sh' to create one.${NC}"
    exit 1
fi

log "${GREEN}✅ Encryption key ready. Recipient ID: ${BOLD}${GPG_RECIPIENT}${NC}"

TIMESTAMP=$(date +%Y-%m-%d_%H-%M-%S)
CURRENT_DATE=$(date +%Y-%m-%d)
CURRENT_DOW=$(date +%u)    # 1=Mon .. 7=Sun
CURRENT_DOM=$(date +%d)    # 01..31
CURRENT_WEEK=$(date +%V)   # 01..53
CURRENT_MONTH=$(date +%m)  # 01..12
CURRENT_YEAR=$(date +%Y)   # 2026

SNAPSHOT_FILE="/tmp/snapshot_${TIMESTAMP}.db"
TEMP_GZ="/tmp/backup_${TIMESTAMP}.gz"
TEMP_GPG="/tmp/backup_${TIMESTAMP}.sqlite.gz.gpg"

cleanup() {
    rm -f "$SNAPSHOT_FILE" "$TEMP_GZ" "$TEMP_GPG"
}
trap cleanup EXIT

log "${YELLOW}🔄 Starting WhatsApp Hot Backup (SQLite)...${NC}"

log "${YELLOW}📸 Creating SQLite online snapshot (.backup)...${NC}"
sqlite3 "$DB_FILE" ".backup '$SNAPSHOT_FILE'"

if [ ! -f "$SNAPSHOT_FILE" ] || [ ! -s "$SNAPSHOT_FILE" ]; then
    log "${RED}❌ Failed to generate SQLite snapshot.${NC}"
    exit 1
fi
ORIGINAL_SIZE=$(du -h "$SNAPSHOT_FILE" | cut -f1)
log "${GREEN}✅ Snapshot created (${ORIGINAL_SIZE}).${NC}"

log "${YELLOW}📦 Compressing with gzip...${NC}"
gzip -c "$SNAPSHOT_FILE" > "$TEMP_GZ"
GZ_SIZE=$(du -h "$TEMP_GZ" | cut -f1)
log "${GREEN}✅ Compressed (${GZ_SIZE}).${NC}"

log "${YELLOW}🔐 Encrypting with OpenGPG public key (${GPG_RECIPIENT})...${NC}"
gpg --batch --yes --trust-model always --encrypt --recipient "$GPG_RECIPIENT" --output "$TEMP_GPG" "$TEMP_GZ"
ENC_SIZE=$(du -h "$TEMP_GPG" | cut -f1)
log "${GREEN}✅ Encrypted (${ENC_SIZE}).${NC}"

TARGET_TIERS=()

case "$SELECTED_TIER" in
    daily)
        TARGET_TIERS+=("daily")
        ;;
    weekly)
        TARGET_TIERS+=("weekly")
        ;;
    monthly)
        TARGET_TIERS+=("monthly")
        ;;
    yearly)
        TARGET_TIERS+=("yearly")
        ;;
    all)
        TARGET_TIERS+=("daily" "weekly" "monthly" "yearly")
        ;;
    auto|*)
        TARGET_TIERS+=("daily")
        
        if [ "$CURRENT_DOW" -eq "$WEEKLY_BACKUP_DOW" ]; then
            TARGET_TIERS+=("weekly")
        fi
        
        if [ "$CURRENT_DOM" == "$MONTHLY_BACKUP_DOM" ]; then
            TARGET_TIERS+=("monthly")
        fi
        
        if [ "$CURRENT_MONTH" == "01" ] && [ "$CURRENT_DOM" == "01" ]; then
            TARGET_TIERS+=("yearly")
        fi
        ;;
esac

log "${CYAN}🎯 Selected GFS Tier(s): ${BOLD}${TARGET_TIERS[*]}${NC}"

AWS_ARGS=""
if [ -n "$AWS_ENDPOINT_URL" ]; then
    AWS_ARGS="--endpoint-url $AWS_ENDPOINT_URL"
fi

if [ -n "$S3_BUCKET" ]; then
    aws $AWS_ARGS s3 ls "s3://${S3_BUCKET}" >/dev/null 2>&1 || aws $AWS_ARGS s3 mb "s3://${S3_BUCKET}" 2>/dev/null || true
fi

for tier in "${TARGET_TIERS[@]}"; do
    case "$tier" in
        daily)
            FILENAME="whatsapp-backup-${TIMESTAMP}.sqlite.gz.gpg"
            ;;
        weekly)
            FILENAME="whatsapp-backup-${CURRENT_DATE}_week-${CURRENT_YEAR}-W${CURRENT_WEEK}.sqlite.gz.gpg"
            ;;
        monthly)
            FILENAME="whatsapp-backup-${CURRENT_DATE}_month-${CURRENT_YEAR}-${CURRENT_MONTH}.sqlite.gz.gpg"
            ;;
        yearly)
            FILENAME="whatsapp-backup-${CURRENT_DATE}_year-${CURRENT_YEAR}.sqlite.gz.gpg"
            ;;
    esac

    TIER_DIR="$BACKUP_DIR/$tier"
    mkdir -p "$TIER_DIR"
    LOCAL_DEST="$TIER_DIR/$FILENAME"

    log "${YELLOW}💾 Saving locally to [$tier]: $LOCAL_DEST${NC}"
    cp "$TEMP_GPG" "$LOCAL_DEST"

    if [ -n "$S3_BUCKET" ]; then
        S3_TARGET_URI="s3://${S3_BUCKET}/${tier}/${FILENAME}"
        log "${YELLOW}☁️  Uploading to S3: ${S3_TARGET_URI}...${NC}"
        if aws $AWS_ARGS s3 cp "$LOCAL_DEST" "$S3_TARGET_URI" --no-progress; then
            log "${GREEN}✅ [$tier] Uploaded successfully.${NC}"
        else
            log "${RED}❌ [$tier] Upload failed.${NC}"
        fi
    fi
done

prune_tier_local() {
    local tier=$1
    local days=$2
    local dir="$BACKUP_DIR/$tier"
    if [ -d "$dir" ] && [ -n "$days" ] && [ "$days" -gt 0 ]; then
        log "${YELLOW}🧹 Pruning local $tier backups older than ${days} days...${NC}"
        find "$dir" -type f -name "*.sqlite.gz.gpg" -mtime +"$days" -delete
    fi
}

prune_tier_s3() {
    local tier=$1
    local days=$2
    [ -z "$S3_BUCKET" ] && return 0
    [ -z "$days" ] || [ "$days" -le 0 ] && return 0

    local cutoff_ts
    cutoff_ts=$(date -d "${days} days ago" +%s 2>/dev/null || true)
    [ -z "$cutoff_ts" ] && return 0

    log "${YELLOW}🧹 Checking S3 retention for $tier (older than ${days} days)...${NC}"
    aws $AWS_ARGS s3 ls "s3://${S3_BUCKET}/${tier}/" 2>/dev/null | while read -r date_str time_str size_bytes obj_name; do
        [ -z "$obj_name" ] && continue
        local obj_ts
        obj_ts=$(date -d "${date_str} ${time_str}" +%s 2>/dev/null || true)
        if [ -n "$obj_ts" ] && [ "$obj_ts" -lt "$cutoff_ts" ]; then
            log "${YELLOW}🗑️  Deleting expired S3 object: ${tier}/${obj_name}${NC}"
            aws $AWS_ARGS s3 rm "s3://${S3_BUCKET}/${tier}/${obj_name}" 2>/dev/null || true
        fi
    done
}

log "${CYAN}⏳ Executing GFS Retention Policy...${NC}"
prune_tier_local "daily" "$RETENTION_DAILY"
prune_tier_s3 "daily" "$RETENTION_DAILY"

WEEKLY_DAYS=$(( RETENTION_WEEKLY * 7 ))
prune_tier_local "weekly" "$WEEKLY_DAYS"
prune_tier_s3 "weekly" "$WEEKLY_DAYS"

MONTHLY_DAYS=$(( RETENTION_MONTHLY * 31 ))
prune_tier_local "monthly" "$MONTHLY_DAYS"
prune_tier_s3 "monthly" "$MONTHLY_DAYS"

YEARLY_DAYS=$(( RETENTION_YEARLY * 365 ))
prune_tier_local "yearly" "$YEARLY_DAYS"
prune_tier_s3 "yearly" "$YEARLY_DAYS"

log "${GREEN}🎉 WhatsApp GFS OpenGPG backup completed successfully!${NC}"
