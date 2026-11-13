#!/bin/bash
# Common configuration and helpers for ChatNip scripts

# Resolve repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." 2>/dev/null && pwd)"
if [ ! -f "$REPO_ROOT/compose.yml" ]; then
    # Fallback if sourced from different nesting
    REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"
fi

# Load .env if present
if [ -f "$REPO_ROOT/.env" ]; then
    set -a
    source "$REPO_ROOT/.env"
    set +a
elif [ -f "$REPO_ROOT/.env.prod" ]; then
    set -a
    source "$REPO_ROOT/.env.prod"
    set +a
elif [ -f "$REPO_ROOT/.env.dev" ]; then
    set -a
    source "$REPO_ROOT/.env.dev"
    set +a
fi

# Terminal colors
if [ -t 1 ] || [ -n "$FORCE_COLOR" ]; then
    BOLD='\033[1m'
    BLUE='\033[0;34m'
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    DIM='\033[2m'
    NC='\033[0m'
else
    BOLD=''
    BLUE=''
    GREEN=''
    RED=''
    YELLOW=''
    CYAN=''
    DIM=''
    NC=''
fi

log_info()    { echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${CYAN}ℹ️  $1${NC}"; }
log_success() { echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${GREEN}✅ $1${NC}"; }
log_warn()    { echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${YELLOW}⚠️  $1${NC}"; }
log_error()   { echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${RED}❌ $1${NC}"; }
