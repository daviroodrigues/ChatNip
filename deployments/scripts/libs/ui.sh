#!/bin/bash

if [ -z "$GREEN" ]; then
    BOLD=$'\033[1m'
    BLUE=$'\033[0;34m'
    GREEN=$'\033[0;32m'
    RED=$'\033[0;31m'
    YELLOW=$'\033[1;33m'
    CYAN=$'\033[0;36m'
    DIM=$'\033[2m'
    NC=$'\033[0m'
fi

ui::_strip_colors() {
    echo "$1" | sed "s/$(echo -e '\033')\[[0-9;]*m//g"
}

ui::header() {
    clear
    echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║${NC}${BOLD}            📦 GUARÁ ERP - GESTÃO DE BACKUPS                  ${NC}${BLUE}║${NC}"
    echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

ui::status_bar() {
    local local_count=$1
    local cloud_status=$2
    echo -e "${BOLD}📊 Status do Sistema:${NC}"
    echo -ne "   🏠 Local: ${local_count} arquivos"
    echo -e "  |  ☁️  Nuvem: ${cloud_status}"
    echo -e "${BLUE}==============================================================================================${NC}"
}

ui::print_table_header() {
    printf "${BOLD}%-6s %-50s %-25s %-10s${NC}\n" "ID" "Arquivo" "Origem" "Tamanho"
    echo -e "${DIM}==============================================================================================${NC}"
}

ui::print_table_row() {
    local id=$1
    local name=$2
    local origin_colored=$3
    local size=$4
    local origin_clean=$(ui::_strip_colors "$origin_colored")
    local col_width=25
    local text_len=${#origin_clean}
    local padding=$(( col_width - text_len ))
    [ $padding -lt 0 ] && padding=0

    printf "%-6s %-50s %s%*s %-10s\n" \
        "$id" \
        "${name:0:48}" \
        "$origin_colored" \
        $padding "" \
        "$size"
}

ui::confirm() {
    local msg=$1
    read -p "$(echo -e "${YELLOW}$msg [y/N]: ${NC}")" confirm
    [[ "$confirm" =~ ^[Yy]$ ]]
}

ui::wait() {
    echo ""
    echo -e "${DIM}Pressione [ENTER] para continuar...${NC}"
    read -r
}

ui::success() { echo -e "${GREEN}✅ $1${NC}"; }
ui::error()   { echo -e "${RED}❌ $1${NC}"; }
ui::info()    { echo -e "${CYAN}ℹ️  $1${NC}"; }
ui::warn()    { echo -e "${YELLOW}⚠️  $1${NC}"; }
