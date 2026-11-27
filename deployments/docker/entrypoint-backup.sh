#!/bin/bash
set -e

CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}${BOLD}       📦 ChatNip Backup Daemon (OpenGPG + GFS Strategy)        ${NC}"
echo -e "${CYAN}================================================================${NC}"

S3_BUCKET_NAME="$AWS_S3_BUCKET"
S3_ENDPOINT="$AWS_ENDPOINT_URL"
PUB_KEY_PATH="$BACKUP_GPG_PUBLIC_KEY_PATH"
RET_DAILY="$BACKUP_RETENTION_DAILY"
RET_WEEKLY="$BACKUP_RETENTION_WEEKLY"
RET_MONTHLY="$BACKUP_RETENTION_MONTHLY"
RET_YEARLY="$BACKUP_RETENTION_YEARLY"
STARTUP_RUN="$BACKUP_RUN_ON_STARTUP"

echo -e "${BOLD}Configuration:${NC}"
echo -e "  • S3 Bucket:          ${S3_BUCKET_NAME}"
echo -e "  • S3 Endpoint:        ${S3_ENDPOINT}"
echo -e "  • Public Key Path:    ${PUB_KEY_PATH}"
echo -e "  • GFS Retention:      Daily: ${RET_DAILY}d | Weekly: ${RET_WEEKLY}w | Monthly: ${RET_MONTHLY}m | Yearly: ${RET_YEARLY}y"
echo -e "  • Run on Startup:     ${STARTUP_RUN}"

export GNUPGHOME="/tmp/gnupg"
mkdir -p "$GNUPGHOME"
chmod 700 "$GNUPGHOME"

if [ -f "$PUB_KEY_PATH" ]; then
    gpg --batch --quiet --import "$PUB_KEY_PATH" 2>/dev/null || true
    echo -e "${GREEN}✅ GPG Public Key loaded from file ($PUB_KEY_PATH).${NC}"
else
    echo -e "${YELLOW}⚠️  No GPG Public Key found yet at $PUB_KEY_PATH. Please generate one with 'make generate-keys'.${NC}"
fi

if [ "$STARTUP_RUN" = "true" ]; then
    echo -e "${CYAN}🚀 Executing startup backup...${NC}"
    /usr/local/bin/backup.sh || echo -e "${YELLOW}⚠️  Startup backup reported warnings or failed. Service will continue according to schedule.${NC}"
fi

echo -e "${GREEN}🕒 Running in Cron mode (Schedule: '${BACKUP_CRON_SCHEDULE}')...${NC}"
touch /var/log/backup.log

env | grep -E '^(AWS_|BACKUP_|GNUPGHOME|PATH|TZ)' | sed 's/^\(.*\)$/export \1/g' > /etc/backup-env.sh
chmod 600 /etc/backup-env.sh

echo "$BACKUP_CRON_SCHEDULE . /etc/backup-env.sh && /usr/local/bin/backup.sh >> /var/log/backup.log 2>&1" | crontab -

trap 'echo "Stopping backup daemon..."; exit 0' SIGTERM SIGINT

crond -b -l 2
echo -e "${GREEN}✅ Backup daemon active. Waiting for scheduled triggers...${NC}"
exec tail -f /var/log/backup.log
