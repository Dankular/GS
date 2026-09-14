# PostgreSQL backup and restore

The VPS Compose PostgreSQL volume is not itself an off-site backup. For
production, provide an age recipient and make encryption mandatory; keep the
age identity outside the VPS or in the approved secret manager:

```sh
cd /opt/gameservice
sudo install -d -m 700 /var/backups/gameservice
sudo GAMESERVICE_BACKUP_DIR=/var/backups/gameservice \
  GAMESERVICE_BACKUP_AGE_RECIPIENT='age1...' \
  GAMESERVICE_BACKUP_REQUIRE_ENCRYPTION=1 \
  ./deploy/backup/backup-postgres.sh
```

Set `GAMESERVICE_BACKUP_S3_URI` to an off-site bucket prefix to upload the
encrypted dump and checksum with the AWS CLI. Configure bucket object-lock,
encrypted retention, and credentials through the approved operations system.
Do not put dumps in Git or paste them into tickets. Verify a dump in an
isolated database with:

```sh
GAMESERVICE_BACKUP_AGE_IDENTITY=/run/secrets/gameservice-backup-agekey \
  ./deploy/backup/verify-restore.sh /var/backups/gameservice/gameservice-<timestamp>.dump.age
```

The verifier creates and removes a uniquely named temporary database. It does
not overwrite the live `gameservice` database. In addition to checking the
restored schema, it compares wallet projections with immutable ledger sums and
fails if reconciliation drift is nonzero. Schedule this procedure after a
retention/RPO/RTO decision and record the output, elapsed time, and checksum in
the operations log. The scripts default to `GAMESERVICE_BACKUP_DATABASE_USER=gameservice_admin`
so the dump includes Nakama-owned public tables; this deployment-only credential
must not be given to application containers. The current Compose PostgreSQL
service is a development/single-host deployment and does not provide PITR or HA
by itself.

## Recurring VPS schedule

Install the supplied hardened units on the Docker VPS after creating the
recipient/identity through the approved secret-management process:

```sh
sudo install -d -m 700 /etc/gameservice /var/backups/gameservice
sudo install -m 600 /path/to/backup.env /etc/gameservice/backup.env
sudo install -m 644 deploy/backup/gameservice-postgres-backup.service /etc/systemd/system/
sudo install -m 644 deploy/backup/gameservice-postgres-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now gameservice-postgres-backup.timer
sudo systemctl start gameservice-postgres-backup.service
sudo systemctl status gameservice-postgres-backup.timer
```

`/etc/gameservice/backup.env` must include at least
`GAMESERVICE_BACKUP_DIR=/var/backups/gameservice`,
`GAMESERVICE_BACKUP_AGE_RECIPIENT`, and
`GAMESERVICE_BACKUP_REQUIRE_ENCRYPTION=1`. Add
`GAMESERVICE_BACKUP_S3_URI` and the AWS credential/role configuration only
when the approved off-site bucket is available. The timer is intentionally not
installed automatically: enabling it without a real recipient would be an
unsafe false-success configuration.

## Migration verification

For development/test migration compatibility, run the complete ordered
up/down/up cycle before changing a migration:

```sh
make migrate-cycle
```

This intentionally drops the control-plane schemas through the checked-in
development rollback and must never be run against production data.
