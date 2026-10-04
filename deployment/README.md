# Debian 13 installation

Use one systemd instance per ODK Central project with a shared binary. Each
instance has its own configuration, session cache, attachments and logs. All
instances run as the `central-sync` system user with a dedicated PostgreSQL role.
They share a Unix account: this separates operations, not security between projects.

Run the commands below as root on the VPS from the repository root. They install
the files; they do not automatically connect to a VPS. Scheduled runs only need
the installed binary and configuration, not Go or a repository checkout.

## CLI installation

The installer replaces manual steps 1 and 2 below. From the repository, prepare
the selected release binary (or build this branch), then install dependencies
as root:

```sh
apt-get update
apt-get install -y ca-certificates logrotate
bash deployment/install.sh --project projet-1 --binary ./central-sync --dry-run
bash deployment/install.sh --project projet-1 --binary ./central-sync
```

To supply your initial configuration instead of the examples:

```sh
bash deployment/install.sh --project projet-1 --binary ./central-sync \
  --env-file /path/projet.env --config-file /path/projet.yaml
```

The installer creates the system account, atomically replaces the binary,
installs systemd/logrotate files and prepares directories and symlinks. Existing
configuration files are never replaced, even when input files are supplied.
Logs, attachments and session caches are preserved. `--dry-run` makes no changes
and does not check Debian host prerequisites. Actual installation requires
Debian 13, a running systemd and root privileges.

Add another project using the installed binary:

```sh
bash deployment/install.sh --project projet-2
```

The installer does not start synchronization or enable timers. Existing timer
state is preserved. When passing `--binary`, stop all project timers and wait
for services to finish before replacing the shared binary. PostgreSQL setup and
migrations remain separate. Edit the generated configuration, then continue at
step 3 to test and schedule. Run `bash deployment/install.sh --help` for options.

To check the installer during development:

```sh
bash -n deployment/install.sh
python3 -m unittest discover -s deployment/tests -v
```

Tests simulate host commands and redirect installation paths into a temporary
directory; they do not perform an actual Debian installation.

## 1. Manually install the binary and system files

Download a release matching `dpkg --print-architecture`, verify its SHA-256 against
the supplied `SHA256SUMS`, and extract `central-sync` into the current directory.
To try this branch before release, build it from source with
`go build -o central-sync`.

```sh
apt-get update
apt-get install -y ca-certificates logrotate
adduser --system --group --no-create-home central-sync
install -o root -g root -m 0755 central-sync /usr/local/bin/central-sync
install -o root -g root -m 0644 deployment/systemd/central-sync@.service /etc/systemd/system/
install -o root -g root -m 0644 deployment/systemd/central-sync@.timer /etc/systemd/system/
install -o root -g root -m 0644 deployment/logrotate/central-sync /etc/logrotate.d/central-sync
systemctl daemon-reload
/usr/local/bin/central-sync version
```

## 2. Set up a project

`projet-1` is a local instance name independent of the Central project ID. Use
lowercase letters, digits and hyphens. Keep exactly one Central project in each
YAML configuration to separate logs. Repeat this section with a different name
for additional projects.

```sh
install -d -o root -g central-sync -m 0750 /etc/central-sync
install -d -o root -g central-sync -m 0750 /etc/central-sync/projet-1
install -d -o central-sync -g central-sync -m 0750 /var/lib/central-sync
install -d -o central-sync -g central-sync -m 0750 /var/lib/central-sync/projet-1
install -d -o central-sync -g central-sync -m 0750 /var/log/central-sync
install -d -o central-sync -g central-sync -m 0750 /var/log/central-sync/projet-1
install -o root -g central-sync -m 0640 deployment/examples/project.env.example /etc/central-sync/projet-1/.env
install -o root -g central-sync -m 0640 deployment/examples/central_config.yaml.example /etc/central-sync/projet-1/central_config.yaml
ln -s /etc/central-sync/projet-1/.env /var/lib/central-sync/projet-1/.env
ln -s /etc/central-sync/projet-1/central_config.yaml /var/lib/central-sync/projet-1/central_config.yaml
ln -s /var/log/central-sync/projet-1/central-sync.log /var/lib/central-sync/projet-1/central-sync.log
```

These commands create a new instance. Do not overwrite an existing configuration
with the examples during an upgrade. Edit both files in `/etc/central-sync/projet-1/`
with credentials, project ID, database and mappings. The application reads `.env`
itself; it is not a shell script. Configuration files remain owned by root and
readable by `central-sync`.

Initialize PostgreSQL using the main README, or apply the migrations required by
the installed version for an existing database, before the first run.
With `local_directory: attachments`, files go under
`/var/lib/central-sync/projet-1/attachments/`. The service permits writes to its
state and log directories; keep writable files there. The session cache is
created in the state directory with mode `0600`.

## 3. Test and enable hourly runs

```sh
systemd-analyze verify /etc/systemd/system/central-sync@.service /etc/systemd/system/central-sync@.timer
logrotate --debug /etc/logrotate.d/central-sync
systemctl start central-sync@projet-1.service
journalctl -u central-sync@projet-1.service -n 100 --no-pager
tail -n 100 /var/log/central-sync/projet-1/central-sync.log
```

Check the logs and `central_metadata.sync_runs` before enabling the timer.
Currently some errors are logged without a nonzero process exit code. A successful
systemd status does not guarantee a successful synchronization and should not be
the sole source of operational alerts.

```sh
systemctl enable --now central-sync@projet-1.timer
systemctl list-timers 'central-sync@*'
systemctl status logrotate.timer
```

Runs are scheduled at each full hour in the VPS timezone, with one-minute
accuracy. `Persistent=true` catches up once if an expiry was missed while the
timer or VPS was stopped; it does not replay every missed hour. An already
running service is left running without a second instance. The PostgreSQL lock
still prevents other runs targeting the same database: use separate databases
for instances that need to run concurrently. There is no global execution timeout.

### Customize a project's schedule

Create an instance override to replace its hourly schedule:

```sh
systemctl edit central-sync@projet-1.timer
```

For example, run this project at minute 15 of every hour:

```ini
[Timer]
OnCalendar=
OnCalendar=*-*-* *:15:00
```

Systemd accumulates `OnCalendar` directives. The empty assignment clears inherited
schedules, including the template's `hourly` value. The next assignment sets the
new schedule. Without the reset, both schedules would be active. This override
applies only to this project and survives reinstalling the shared template.

Other expressions:

| Expression | Schedule |
| --- | --- |
| `*-*-* 08,12,18:00:00` | Every day at 08:00, 12:00 and 18:00 |
| `Mon..Fri *-*-* 08:00:00` | Monday through Friday at 08:00 |

Validate the expression, reload units and restart the timer:

```sh
systemd-analyze calendar '*-*-* *:15:00'
systemctl daemon-reload
systemctl restart central-sync@projet-1.timer
systemctl list-timers 'central-sync@*'
```

The VPS timezone, `Persistent=true` and one-minute accuracy still apply. See
the `OnCalendar` section of [systemd.timer](https://manpages.debian.org/trixie/systemd/systemd.timer.5.en.html)
for accumulation and reset behavior.

## 4. Inspect logs and stop scheduling

```sh
journalctl -u central-sync@projet-1.service --since today
tail -f /var/log/central-sync/projet-1/central-sync.log
systemctl disable --now central-sync@projet-1.timer
```

Stopping the timer lets an active run finish. Logrotate rotates files daily and
retains 14 archives, delaying compression by one rotation. A running process
continues writing to the renamed archive; the next run opens the new file.
No `copytruncate` or service restart is required. For runs lasting more than a
day, also consult journald: delayed compression does not protect a file still
open during later rotations. Journald retention is managed separately by the VPS.

## 5. Upgrade and back up

Disable affected timers and wait for their services to finish
(`systemctl is-active central-sync@projet-1.service`). Back up databases,
configuration and attachments, then apply required migrations. Replace the binary
through a rename instead of overwriting an executable in use:

```sh
install -o root -g root -m 0755 central-sync /usr/local/bin/central-sync.new
mv /usr/local/bin/central-sync.new /usr/local/bin/central-sync
/usr/local/bin/central-sync version
systemctl start central-sync@projet-1.service
```

Verify the synchronization and re-enable timers. All projects share the same
binary version. Reinstall systemd/logrotate templates if they have changed and
run `systemctl daemon-reload` before re-enabling timers. Back up
`/etc/central-sync/`, attachments under `/var/lib/central-sync/` and PostgreSQL
databases. Protect backups containing credentials and session caches.

Debian 13 references: [systemd.timer](https://manpages.debian.org/trixie/systemd/systemd.timer.5.en.html),
[systemd.exec](https://manpages.debian.org/trixie/systemd/systemd.exec.5.en.html),
[logrotate](https://manpages.debian.org/trixie/logrotate/logrotate.8.en.html).
