#!/usr/bin/env bash
set -euo pipefail
umask 0027
export PATH="${PATH}:/usr/sbin:/sbin"

usage() {
    cat <<'EOF'
Usage: bash deployment/install.sh --project NAME [options]
       bash deployment/install.sh --interactive [--project NAME] [options]

Install central-sync and prepare one project on Debian 13 (run as root).

  --interactive       Ask for instance, Central, PostgreSQL and sync configuration
  --project NAME      Local instance name: lowercase letters, digits, hyphens
  --binary FILE       Local central-sync binary; otherwise reuse the installed one
  --env-file FILE     Initial .env; otherwise use the example
  --config-file FILE  Initial central_config.yaml; otherwise use the example
  --dry-run           Check inputs and show the plan without changing the system
  -h, --help          Show this help

Existing project configurations, logs and data are preserved. The installer does
not initialize PostgreSQL, start synchronization or enable project timers.
EOF
}

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
project=''
binary=''
env_source="$script_dir/examples/project.env.example"
config_source="$script_dir/examples/central_config.yaml.example"
dry_run=false
interactive=false
custom_files=false
temporary_binary=''
trap 'if [[ -n "$temporary_binary" ]]; then rm -f -- "$temporary_binary"; fi' EXIT

while (($#)); do
    case "$1" in
        --project|--binary|--env-file|--config-file)
            (($# >= 2)) && [[ -n "$2" && "$2" != --* ]] || fail "Missing value for $1"
            case "$1" in
                --project) project=$2 ;;
                --binary) binary=$2 ;;
                --env-file) env_source=$2; custom_files=true ;;
                --config-file) config_source=$2; custom_files=true ;;
            esac
            shift 2 ;;
        --interactive) interactive=true; shift ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) fail "Unknown argument: $1 (use --help)" ;;
    esac
done

# Interactive values stay in shell memory until the protected configuration is written.
# Disable shell tracing before reading credentials, including when invoked with bash -x.
if "$interactive"; then
    set +x
    "$custom_files" && fail '--interactive cannot be combined with --env-file or --config-file'
fi

ask() {
    local label=$1 default=${2-} secret=${3-false} answer
    while true; do
        printf '%s' "$label" >&2
        [[ -z "$default" ]] || printf ' [%s]' "$default" >&2
        printf ': ' >&2
        if "$secret"; then
            IFS= read -rs answer || fail 'Input cancelled; configuration was not written'
            printf '\n' >&2
        else
            IFS= read -r answer || fail 'Input cancelled; configuration was not written'
        fi
        answer=${answer:-$default}
        [[ -n "$answer" ]] && break
        printf 'A value is required.\n' >&2
    done
    REPLY=$answer
}

if "$interactive" && [[ -z "$project" ]]; then
    ask 'Instance name (lowercase letters, digits, hyphens)'
    project=$REPLY
fi

[[ "$project" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || fail 'Provide --project using 1–63 lowercase letters, digits or hyphens'
config_dir="/etc/central-sync/$project"
state_dir="/var/lib/central-sync/$project"
log_dir="/var/log/central-sync/$project"
installed_binary=/usr/local/bin/central-sync

for source in "$env_source" "$config_source" \
    "$script_dir/systemd/central-sync@.service" \
    "$script_dir/systemd/central-sync@.timer" "$script_dir/logrotate/central-sync"; do
    [[ -f "$source" && -r "$source" ]] || fail "Cannot read $source"
done
if [[ -n "$binary" ]]; then
    [[ -f "$binary" && -r "$binary" ]] || fail "Cannot read binary: $binary"
else
    [[ -x "$installed_binary" ]] || fail 'Provide --binary for the first installation'
fi

check_link() {
    local target=$1 link=$2
    if [[ -L "$link" ]]; then
        [[ "$(readlink -- "$link")" == "$target" ]] || fail "Conflicting symlink: $link"
    elif [[ -e "$link" ]]; then
        fail "Existing path is not the expected symlink: $link"
    fi
}
preflight_paths() {
    local path
    for path in /etc/central-sync /var/lib/central-sync /var/log/central-sync \
        "$config_dir" "$state_dir" "$log_dir"; do
        [[ ! -L "$path" ]] || fail "Directory must not be a symlink: $path"
        [[ ! -e "$path" || -d "$path" ]] || fail "Not a directory: $path"
    done
    for path in "$config_dir/.env" "$config_dir/central_config.yaml" "$log_dir/central-sync.log"; do
        [[ ! -L "$path" ]] || fail "File must not be a symlink: $path"
        [[ ! -e "$path" || -f "$path" ]] || fail "Not a regular file: $path"
    done
    check_link "$config_dir/.env" "$state_dir/.env"
    check_link "$config_dir/central_config.yaml" "$state_dir/central_config.yaml"
    check_link "$log_dir/central-sync.log" "$state_dir/central-sync.log"
}
preflight_paths

if "$dry_run"; then
    info "Dry run: no files, accounts or services will be changed."
    if "$interactive"; then info "Interactive configuration and credential prompts are skipped during dry run."; fi
    info "Binary: ${binary:-$installed_binary} -> $installed_binary"
    info "System account: central-sync"
    info "Configuration: $config_dir (existing files preserved)"
    info "State and attachments: $state_dir"
    info "Logs: $log_dir (daily rotation, 14 archives)"
    info "Units: central-sync@$project.service and central-sync@$project.timer"
    info "Timer will remain unchanged; a new project is not started or enabled."
    exit 0
fi

[[ $EUID -eq 0 ]] || fail 'Run the installer as root (or use --dry-run)'
# Read the distribution identity without sourcing shell code.
[[ -r /etc/os-release ]] || fail 'Cannot identify the operating system'
os_id=$(sed -n 's/^ID=//p' /etc/os-release | tr -d '"')
os_version=$(sed -n 's/^VERSION_ID=//p' /etc/os-release | tr -d '"')
[[ "$os_id" == debian && "$os_version" == 13 ]] || fail 'This installer supports Debian 13'
for command in adduser getent install mktemp mv readlink flock systemctl systemd-analyze logrotate; do
    command -v "$command" >/dev/null || fail "Missing command: $command (install ca-certificates and logrotate first)"
done
[[ -d /run/systemd/system ]] || fail 'systemd must be running on this host'
exec 9>/run/lock/central-sync-install.lock
flock -n 9 || fail 'Another central-sync installer is running'
preflight_paths

if [[ -n "$binary" ]]; then
    active_units=$(systemctl list-units --all --state=active,activating,deactivating \
        --no-legend --plain 'central-sync@*.service' 'central-sync@*.timer')
    [[ -z "$active_units" ]] || fail 'Stop project timers and let running services finish before replacing the shared binary'
fi

ask_env() {
    local label=$1 default=${2-} secret=${3-false}
    while true; do
        ask "$label" "$default" "$secret"
        # The application's .env reader trims whitespace and boundary quotes.
        if [[ "$REPLY" != [[:space:]\"\']* && "$REPLY" != *[[:space:]\"\'] && "$REPLY" != *$'\r'* ]]; then
            break
        fi
        printf 'Leading/trailing whitespace or quotes, and carriage returns, are unsupported by the .env reader.\n' >&2
    done
}
ask_identifier() {
    while true; do
        ask "$1" "${2-}"
        [[ "$REPLY" =~ ^[a-z_][a-z0-9_]{0,62}$ ]] && break
        printf 'Use 1-63 lowercase letters, digits or underscores; start with a letter or underscore.\n' >&2
    done
}
ask_yes_no() {
    while true; do
        ask "$1 (yes/no)" "${2-no}"
        case "$REPLY" in
            yes|y) REPLY=true; return ;;
            no|n) REPLY=false; return ;;
        esac
    done
}
yaml_string() {
    local value=${1//\'/\'\'}
    printf "'%s'" "$value"
}

if "$interactive"; then
    [[ ! -e "$config_dir/.env" && ! -e "$config_dir/central_config.yaml" ]] ||
        fail 'Interactive setup is for new instances only; existing configuration is preserved. Edit the existing files directly.'
    info 'Configure one ODK Central project. PostgreSQL must be prepared separately.'
    while true; do
        ask_env 'ODK Central URL (http:// or https://)'
        central_url=$REPLY
        [[ "$central_url" =~ ^https?://[^[:space:]]+$ ]] && break
    done
    ask_env 'ODK Central account email'; central_email=$REPLY
    ask_env 'ODK Central password (hidden)' '' true; central_password=$REPLY
    ask_env 'PostgreSQL host' localhost; pg_host=$REPLY
    while true; do
        ask_env 'PostgreSQL port' 5432; pg_port=$REPLY
        [[ "$pg_port" =~ ^[1-9][0-9]{0,4}$ ]] && ((pg_port <= 65535)) && break
    done
    ask_env 'PostgreSQL role' centralsync; pg_user=$REPLY
    ask_env 'PostgreSQL password (hidden)' '' true; pg_password=$REPLY
    while true; do
        ask_env 'PostgreSQL SSL mode (disable/allow/prefer/require/verify-ca/verify-full)' disable
        pg_sslmode=$REPLY
        case "$pg_sslmode" in disable|allow|prefer|require|verify-ca|verify-full) break ;; esac
    done
    ask_identifier 'PostgreSQL database name' "central_${project//-/_}"; database_name=$REPLY
    while true; do
        ask 'ODK Central project ID'; project_id=$REPLY
        [[ "$project_id" =~ ^[1-9][0-9]{0,8}$ ]] && break
    done
    ask 'Project display name' "$project"; project_name=$REPLY
    ask_yes_no 'Synchronize App Users'; sync_app_users=$REPLY
    declare -a dataset_names=() dataset_tables=() form_ids=() form_tables=() form_modes=()
    declare -A mapped_tables=()
    while true; do
        ask_yes_no 'Add a dataset'; [[ "$REPLY" == true ]] || break
        ask 'Dataset name (exact name in Central)'; dataset_names+=("$REPLY")
        while true; do
            ask_identifier 'Dataset destination table'
            [[ -z "${mapped_tables[$REPLY]+present}" ]] && break
            printf 'This table is already mapped.\n' >&2
        done
        dataset_tables+=("$REPLY"); mapped_tables[$REPLY]=1
    done
    info 'Forms include every approval state; automatic approval and attachment downloads are disabled.'
    while true; do
        ask_yes_no 'Add a form'; [[ "$REPLY" == true ]] || break
        ask 'Form XML ID (exact ID in Central)'; form_ids+=("$REPLY")
        while true; do
            ask_identifier 'Form destination table'
            [[ -z "${mapped_tables[$REPLY]+present}" ]] && break
            printf 'This table is already mapped.\n' >&2
        done
        form_tables+=("$REPLY"); mapped_tables[$REPLY]=1
        while true; do
            ask 'Form sync mode (append_only/upsert)' append_only
            [[ "$REPLY" == append_only || "$REPLY" == upsert ]] && break
        done
        form_modes+=("$REPLY")
    done
    if [[ "$sync_app_users" == false && ${#dataset_names[@]} -eq 0 && ${#form_ids[@]} -eq 0 ]]; then
        info "No synchronization selected. Add datasets or forms in $config_dir/central_config.yaml before starting the service."
    fi
fi

write_interactive_config() {
    local index
    install -o root -g central-sync -m 0640 /dev/null "$config_dir/.env"
    install -o root -g central-sync -m 0640 /dev/null "$config_dir/central_config.yaml"
    {
        printf 'ODK_CENTRAL_URL=%s\nODK_CENTRAL_USER_EMAIL=%s\nODK_CENTRAL_USER_PASSWORD=%s\n' \
            "$central_url" "$central_email" "$central_password"
        printf 'PG_HOST=%s\nPG_PORT=%s\nPG_USER=%s\nPG_PASSWORD=%s\nPG_SSLMODE=%s\n' \
            "$pg_host" "$pg_port" "$pg_user" "$pg_password" "$pg_sslmode"
    } > "$config_dir/.env"
    {
        printf 'attachment_storage:\n  backend: local\n  local_directory: attachments\nprojects:\n  - project_id: %s\n' "$project_id"
        printf '    project_name: '; yaml_string "$project_name"
        printf '\n    database_name: '; yaml_string "$database_name"
        printf '\n    sync_app_users: %s\n' "$sync_app_users"
        if ((${#dataset_names[@]} == 0)); then printf '    datasets: []\n'; else
            printf '    datasets:\n'
            for index in "${!dataset_names[@]}"; do
                printf '      - name: '; yaml_string "${dataset_names[$index]}"
                printf '\n        table_name: %s\n        sync: true\n' "${dataset_tables[$index]}"
            done
        fi
        if ((${#form_ids[@]} == 0)); then printf '    forms: []\n'; else
            printf '    forms:\n'
            for index in "${!form_ids[@]}"; do
                printf '      - xml_form_id: '; yaml_string "${form_ids[$index]}"
                printf '\n        table_name: %s\n        sync: true\n        sync_mode: %s\n' "${form_tables[$index]}" "${form_modes[$index]}"
                printf '        approved_only: false\n        approve_after_sync: false\n        sync_attachments: false\n'
            done
        fi
    } > "$config_dir/central_config.yaml"
    unset central_password pg_password REPLY
    info "Created configuration in $config_dir"
}

if ! getent passwd central-sync >/dev/null; then
    adduser --system --group --no-create-home central-sync
fi
getent group central-sync >/dev/null || fail 'The central-sync account needs a central-sync group'

if [[ -n "$binary" ]]; then
    temporary_binary=$(mktemp /usr/local/bin/.central-sync.XXXXXX)
    install -o root -g root -m 0755 -- "$binary" "$temporary_binary"
    "$temporary_binary" version
    mv -fT -- "$temporary_binary" "$installed_binary"
    temporary_binary=''
fi
install -o root -g root -m 0644 -- "$script_dir/systemd/central-sync@.service" /etc/systemd/system/central-sync@.service
install -o root -g root -m 0644 -- "$script_dir/systemd/central-sync@.timer" /etc/systemd/system/central-sync@.timer
install -o root -g root -m 0644 -- "$script_dir/logrotate/central-sync" /etc/logrotate.d/central-sync
install -d -o root -g central-sync -m 0750 /etc/central-sync "$config_dir"
install -d -o central-sync -g central-sync -m 0750 /var/lib/central-sync "$state_dir" /var/log/central-sync "$log_dir"

create_config() {
    local source=$1 destination=$2
    if [[ -e "$destination" ]]; then
        info "Preserved: $destination (input $source was not copied)"
    else
        install -o root -g central-sync -m 0640 -- "$source" "$destination"
        info "Created: $destination"
    fi
}
if "$interactive"; then
    write_interactive_config
else
    create_config "$env_source" "$config_dir/.env"
    create_config "$config_source" "$config_dir/central_config.yaml"
fi
for name in .env central_config.yaml; do
    [[ -L "$state_dir/$name" ]] || ln -s -- "$config_dir/$name" "$state_dir/$name"
done
[[ -L "$state_dir/central-sync.log" ]] || ln -s -- "$log_dir/central-sync.log" "$state_dir/central-sync.log"
systemd-analyze verify /etc/systemd/system/central-sync@.service /etc/systemd/system/central-sync@.timer
logrotate --debug /etc/logrotate.d/central-sync
systemctl daemon-reload

info "Installation complete for $project. Existing timer state was preserved."
info "1. Edit $config_dir/.env and $config_dir/central_config.yaml (one Central project)."
info "2. Initialize PostgreSQL or apply the migrations required by the installed version."
info "3. Test: systemctl start central-sync@$project.service"
info "   Logs: journalctl -u central-sync@$project.service -n 100 --no-pager"
info "   Check logs and central_metadata.sync_runs; the exit code alone is not reliable yet."
info "4. Schedule: systemctl enable --now central-sync@$project.timer"
