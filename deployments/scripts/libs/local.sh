#!/bin/bash

local::count_backups() {
    find "$BACKUP_DIR" -type f -name "*.sql.gz*" 2>/dev/null | wc -l
}

local::list_files() {
    find "$BACKUP_DIR" -type f -name "*.sql.gz*" 2>/dev/null | sort -r
}

local::trigger_backup_container() {
    docker compose up -d db-backup 2>/dev/null
    docker compose exec db-backup /bin/bash /backup.sh
}

local::update_cron() {
    local schedule=$1
    echo "$schedule /bin/bash /backup.sh >> /var/log/cron.log 2>&1" | \
        docker compose exec -T db-backup crontab -
}
