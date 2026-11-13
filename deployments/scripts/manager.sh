#!/bin/bash

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"

source "$BASE_DIR/libs/common.sh" 2>/dev/null || true
source "$BASE_DIR/libs/ui.sh" 2>/dev/null || true
source "$BASE_DIR/libs/cloud.sh" 2>/dev/null || true

BACKUP_DIR="../../volumes/backups"
KEYS_DIR="../../keys"
CONTAINER_BACKUP="chatnip-backup"

export S3_BACKUP_BUCKET="$AWS_S3_BUCKET"
export AWS_ENDPOINT_URL="$AWS_ENDPOINT_URL"
export AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY"

run_aws() {
    if ! docker compose ps -q "$CONTAINER_BACKUP" > /dev/null 2>&1; then
        echo "container_stopped"
        return 1
    fi

    local cmd="aws"
    if [ -n "$AWS_ENDPOINT_URL" ]; then
        cmd="aws --endpoint-url $AWS_ENDPOINT_URL"
    fi

    docker compose exec -T "$CONTAINER_BACKUP" $cmd "$@"
}

ctrl::dashboard() {
    local local_count
    local_count=$(find "$BACKUP_DIR" -type f \( -name "*.gpg" -o -name "*.enc" -o -name "*.sqlite.gz*" \) 2>/dev/null | wc -l)

    local cloud_status="Off/Container Stopped"
    local cloud_raw
    cloud_raw=$(cloud::check_health)

    if [[ "$cloud_raw" == *"online"* ]]; then
        IFS='|' read -r status count <<< "$cloud_raw"
        cloud_status="${GREEN}Online ($count files)${NC}"
    elif [[ "$cloud_raw" == *"container_stopped"* ]]; then
        cloud_status="${YELLOW}Container Stopped${NC}"
    else
        cloud_status="${RED}S3 Offline / Unreachable${NC}"
    fi

    local gpg_key_file="${BACKUP_GPG_PUBLIC_KEY_PATH}"
    local gpg_status="${RED}Missing ($gpg_key_file)${NC}"
    if [ -f "$gpg_key_file" ]; then
        gpg_status="${GREEN}Configured ($gpg_key_file)${NC}"
    fi

    ui::header
    echo -e "${BOLD}📱 WHATSAPP BACKUP MANAGER (OpenGPG + GFS Strategy)${NC}"
    echo -e "   🔐 Chave GPG: ${gpg_status}"
    ui::status_bar "$local_count" "$cloud_status"
}

ctrl::list_all() {
    local return_mode=$1
    [ "$return_mode" != "quiet" ] && ui::header && ui::info "Consultando Backups Locais e S3..."

    declare -A map_files
    declare -A map_size
    declare -A map_origin

    # Scan local directory recursively
    if [ -d "$BACKUP_DIR" ]; then
        while read -r f; do
            [ -z "$f" ] && continue
            # Relative path from $BACKUP_DIR (e.g. daily/filename or filename)
            rel_path="${f#$BACKUP_DIR/}"
            map_files["$rel_path"]="$rel_path"
            map_size["$rel_path"]=$(du -h "$f" | cut -f1)
            map_origin["$rel_path"]="${YELLOW}🏠 Local${NC}"
        done < <(find "$BACKUP_DIR" -type f \( -name "*.gpg" -o -name "*.enc" -o -name "*.sqlite.gz*" \) | sort -r)
    fi

    # Scan S3 recursively
    local health_raw
    health_raw=$(cloud::check_health)
    if [[ "$health_raw" == online* ]]; then
        while read -r rel_path size_bytes; do
            [ -z "$rel_path" ] && continue

            local size_fmt
            size_fmt=$(numfmt --to=iec --suffix=B "$size_bytes" 2>/dev/null || echo "$size_bytes")

            if [ -n "${map_files[$rel_path]}" ]; then
                map_origin["$rel_path"]="${GREEN}✅ Synced${NC}"
            else
                map_files["$rel_path"]="$rel_path"
                map_size["$rel_path"]="$size_fmt"
                map_origin["$rel_path"]="${BLUE}☁️  S3 Only${NC}"
            fi
        done < <(cloud::list_files)
    fi

    if [ "$return_mode" != "quiet" ]; then ui::header; fi
    ui::print_table_header

    # Sort keys
    local sorted_keys=()
    while IFS= read -r k; do
        [ -n "$k" ] && sorted_keys+=("$k")
    done < <(for k in "${!map_files[@]}"; do echo "$k"; done | sort -r)

    local i=1
    for key in "${sorted_keys[@]}"; do
        ui::print_table_row "$i." "$key" "${map_origin[$key]}" "${map_size[$key]}"
        ((i++))
    done

    if [ ${#sorted_keys[@]} -eq 0 ]; then
        ui::warn "Nenhum backup encontrado."
    fi

    echo ""
    if [ "$return_mode" != "quiet" ]; then ui::wait; fi
}

ctrl::create_backup() {
    ui::header
    echo -e "${BOLD}🚀 CRIAR NOVO BACKUP${NC}"
    echo -e "Escolha a estratégia do backup:"
    echo -e "  [1] Automático GFS (Daily + Weekly se domingo + Monthly se dia 1)"
    echo -e "  [2] Forçar Tier DAILY (Son)"
    echo -e "  [3] Forçar Tier WEEKLY (Father)"
    echo -e "  [4] Forçar Tier MONTHLY (Grandfather)"
    echo -e "  [5] Forçar Tier YEARLY (Great-Grandfather)"
    echo -e "  [6] Forçar TODOS os Tiers simultaneamente"
    echo -e "  [0] Voltar"
    echo ""
    read -p "> " opt

    local tier_flag="auto"
    case $opt in
        1) tier_flag="auto" ;;
        2) tier_flag="daily" ;;
        3) tier_flag="weekly" ;;
        4) tier_flag="monthly" ;;
        5) tier_flag="yearly" ;;
        6) tier_flag="all" ;;
        0) return ;;
        *) ui::error "Opção inválida"; sleep 1; return ;;
    esac

    ui::info "Iniciando container de backup se necessário..."
    docker compose up -d "$CONTAINER_BACKUP" 2>/dev/null

    ui::info "Disparando backup no container (Tier: $tier_flag)..."
    if docker compose exec "$CONTAINER_BACKUP" /usr/local/bin/backup.sh --tier="$tier_flag"; then
        ui::success "Backup OpenGPG (GFS: $tier_flag) gerado e enviado com sucesso!"
    else
        ui::error "Falha na execução do backup. Verifique: docker compose logs $CONTAINER_BACKUP"
    fi
    ui::wait
}

ctrl::restore_flow() {
    ctrl::list_all "quiet"
    echo -e "${YELLOW}----------------------------------------------------------------------------------------------${NC}"
    echo -e "Digite o caminho relativo do arquivo para restaurar (ex: daily/whatsapp-backup-...):"
    read -p "> " fname

    [ -z "$fname" ] && return

    # Check if local exists
    if [ ! -f "$BACKUP_DIR/$fname" ]; then
        ui::info "Arquivo não encontrado localmente. Baixando do S3..."

        if cloud::download_file "$fname" "$BACKUP_DIR"; then
            ui::success "Download do S3 concluído com sucesso."
        else
            ui::error "Erro ao baixar arquivo do S3."
            ui::wait; return
        fi
    fi

    ./restore.sh "$fname"
    ui::wait
}

ctrl::generate_keys() {
    ui::header
    ./generate-gpg-keys.sh
    ui::wait
}

while true; do
    ctrl::dashboard
    echo -e "${BOLD}MENU PRINCIPAL${NC}"
    echo -e "  [1] 🚀 Novo Backup (GFS / OpenGPG)"
    echo -e "  [2] ♻️  Restaurar Backup"
    echo -e "  [3] 📂 Listar Backups (Local & Nuvem)"
    echo -e "  [4] 🔐 Gerar Chaves OpenGPG (RSA 4096)"
    echo -e "  [0] 🚪 Sair"
    echo ""
    read -p "> " choice

    case $choice in
        1) ctrl::create_backup ;;
        2) ctrl::restore_flow ;;
        3) ctrl::list_all ;;
        4) ctrl::generate_keys ;;
        0) exit 0 ;;
        *) ui::error "Opção inválida"; sleep 1 ;;
    esac
done
