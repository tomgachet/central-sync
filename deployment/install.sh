#!/usr/bin/env bash
set -euo pipefail
umask 0027
export PATH="${PATH}:/usr/sbin:/sbin"

usage() {
    cat <<'EOF'
Usage: bash deployment/install.sh --project NAME [options]

Install central-sync and prepare one project on Debian 13 (run as root).

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
temporary_binary=''
trap 'if [[ -n "$temporary_binary" ]]; then rm -f -- "$temporary_binary"; fi' EXIT

while (($#)); do
    case "$1" in
        --project|--binary|--env-file|--config-file)
            (($# >= 2)) && [[ -n "$2" && "$2" != --* ]] || fail "Missing value for $1"
            case "$1" in
                --project) project=$2 ;;
                --binary) binary=$2 ;;
                --env-file) env_source=$2 ;;
                --config-file) config_source=$2 ;;
            esac
            shift 2 ;;
        --dry-run) dry_run=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) fail "Unknown argument: $1 (use --help)" ;;
    esac
done

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
create_config "$env_source" "$config_dir/.env"
create_config "$config_source" "$config_dir/central_config.yaml"
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
