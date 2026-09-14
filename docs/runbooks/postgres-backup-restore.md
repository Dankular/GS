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
not overwrite the live `gameservice` database. Schedule this procedure after a
retention/RPO/RTO decision and record the output, elapsed time, and checksum in
the operations log. The scripts default to `GAMESERVICE_BACKUP_DATABASE_USER=gameservice_admin`
so the dump includes Nakama-owned public tables; this deployment-only credential
must not be given to application containers. The current Compose PostgreSQL
service is a development/single-host deployment and does not provide PITR or HA
by itself.
