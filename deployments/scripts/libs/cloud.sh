#!/bin/bash

cloud::check_health() {
    if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_BACKUP}$"; then
        echo "container_stopped|0"
        return
    fi

    if run_aws s3 ls "s3://${S3_BACKUP_BUCKET}/" >/dev/null 2>&1; then
        local count=$(run_aws s3 ls "s3://${S3_BACKUP_BUCKET}/" --recursive 2>/dev/null | grep -E '\.(gpg|enc|gz)$' | wc -l)
        echo "online|$count"
    else
        echo "offline|0"
    fi
}

cloud::list_files() {
    run_aws s3 ls "s3://${S3_BACKUP_BUCKET}/" --recursive 2>/dev/null | grep -E '\.(gpg|enc|gz)$' | awk '{print $4, $3}'
}

cloud::upload_file() {
    local filepath=$1
    local dest_path=$2
    if [ -z "$dest_path" ]; then
        dest_path=$(basename "$filepath")
    fi

    run_aws s3 cp "$filepath" "s3://${S3_BACKUP_BUCKET}/$dest_path"
}

cloud::download_file() {
    local s3_rel_path=$1
    local dest_dir=$2
    local full_dest="$dest_dir/$s3_rel_path"

    mkdir -p "$(dirname "$full_dest")"

    run_aws s3 cp "s3://${S3_BACKUP_BUCKET}/$s3_rel_path" "$full_dest"
}
