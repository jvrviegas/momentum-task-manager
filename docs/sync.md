# Self-hosted Taskwarrior sync

Momentum delegates synchronization to Taskwarrior. This guide is operational documentation for the approved two-device topology; it is not a provisioning command and contains no real credentials.

```text
macOS Taskwarrior ─┐
                   ├─> Cloud Run TaskChampion PostgreSQL server ─> Neon PostgreSQL
Linux Taskwarrior ─┘
```

## Before deploying

1. Create a Neon PostgreSQL project and database.
2. Choose and pin one TaskChampion PostgreSQL server release. Use the image and environment-variable names documented by that exact release; do not use an unpinned `latest` image.
3. Apply the matching `postgres/schema.sql` from that release to Neon.
4. Generate one high-entropy client UUID (`uuidgen`) and a separate random 256-bit encryption secret. Store both in a password manager; never put either in this repository.
5. Pre-create the allowed client row, or bootstrap once as documented by the selected server release, before disabling automatic client creation.

## Cloud Run

Use the official PostgreSQL server image for the pinned release (the current image family is published by TaskChampion), a generated Cloud Run `run.app` HTTPS URL, and one instance for this two-device deployment.

Configure:

- minimum instances: `0`
- maximum instances: `1`
- unauthenticated ingress: enabled, because Taskwarrior does not mint Google IAM tokens for this protocol
- port/listen address: the port and listen setting required by the pinned image
- `CONNECTION`: Neon direct TLS connection string from Google Secret Manager
- `CLIENT_ID`: the generated client UUID
- `CREATE_CLIENTS=false` after the permitted client is provisioned, if supported by the pinned release
- logging: normal service logs; never log the connection string or encryption secret

Keep the direct Neon endpoint rather than a pooler unless the selected TaskChampion release documents otherwise. TaskChampion already manages its database access and serializable transaction behavior.

Example deployment shape (replace placeholders with release-specific values; do not paste secrets into shell history):

```sh
export TASKCHAMPION_IMAGE='ghcr.io/gothenburgbitfactory/taskchampion-sync-server-postgres:<PINNED_RELEASE>'
export SERVICE='momentum-taskchampion'
# Create a Secret Manager secret containing the Neon direct TLS URL.
# Deploy $TASKCHAMPION_IMAGE with CONNECTION sourced from Secret Manager,
# CLIENT_ID sourced from your password manager, and the release's listen/port settings.
gcloud run deploy "$SERVICE" \
  --image "$TASKCHAMPION_IMAGE" \
  --region '<REGION>' \
  --min 0 --max 1 \
  --allow-unauthenticated
```

Review the official server release documentation before running the final command. Verify the generated HTTPS URL and the server's health/readiness behavior without printing credentials.

## Client configuration

Both machines use the same values in an untracked `~/.config/taskwarrior/secrets.rc`:

```text
sync.server.url=<Cloud Run URL>
sync.server.client_id=<generated client UUID>
sync.encryption_secret=<random 256-bit secret>
```

Create that file through a password manager or a no-echo bootstrap flow and set mode `0600`:

```sh
install -m 600 /dev/null ~/.config/taskwarrior/secrets.rc
$EDITOR ~/.config/taskwarrior/secrets.rc
```

A tracked `.taskrc` may contain shared non-secret preferences and:

```text
include ~/.config/taskwarrior/secrets.rc
```

Never copy `~/.task` between machines and never synchronize it with rsync or Syncthing.

## First replica and recurrence

Linux is the recurrence primary:

```sh
task config recurrence on
task sync
```

Configure macOS as an empty second replica, with recurrence disabled:

```sh
task config recurrence off
task sync
```

Do not seed either replica by copying Taskwarrior data files. The shared encryption secret stays on clients and is not stored by the server.

## Verify two-way sync

Create one task on each machine, run `task sync` twice on each side, and verify the other task appears. Allow for Cloud Run and Neon cold starts. Momentum's `Ctrl+R` invokes the same native `task sync` command while it is running.

Momentum delays automatic sync for 15 seconds after a mutation so `u` can invoke Taskwarrior undo. Manual sync closes that grace window. If the server is unavailable, local task operations continue and Momentum retries with 15s, 30s, 1m, 2m, then 5m backoff.
