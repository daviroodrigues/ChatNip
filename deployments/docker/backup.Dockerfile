FROM alpine:latest

RUN apk add --no-cache \
    sqlite \
    aws-cli \
    gnupg \
    bash \
    coreutils \
    tzdata \
    curl

RUN mkdir -p /var/log /backups /data /keys

COPY deployments/scripts/backup-sqlite.sh /usr/local/bin/backup.sh
COPY deployments/docker/entrypoint-backup.sh /entrypoint.sh

RUN chmod +x /usr/local/bin/backup.sh /entrypoint.sh

CMD ["/entrypoint.sh"]
