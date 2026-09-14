# PostgreSQL backup and restore

The VPS Compose PostgreSQL volume is not itself an off-site backup. Run the
backup script to create an encrypted-storage-ready custom-format dump and a
sidecar SHA-256 checksum:

```sh
cd /opt/gameservice
sudo install -d -m 700 /var/backups/gameservice
sudo GAMESERVICE_BACKUP_DIR=/var/backups/gameservice \
  ./deploy/backup/backup-postgres.sh
```

Copy the dump and checksum to the approved encrypted off-site location. Do not
put dumps in Git or paste them into tickets. Verify a dump in an isolated
database with:

```sh
./deploy/backup/verify-restore.sh /var/backups/gameservice/gameservice-<timestamp>.dump
```

The verifier creates and removes a uniquely named temporary database. It does
not overwrite the live `gameservice` database. Schedule this procedure after a
retention/RPO/RTO decision and record the output, elapsed time, and checksum in
the operations log. The current Compose PostgreSQL service is a development/
single-host deployment and does not provide PITR or HA by itself.
