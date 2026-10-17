# Deploying the Momentum server

The server runs on Cloud Run and keeps its data in Neon (ADR 0002, D8). This guide records what exists, how it was set up, how to release a new version, and what to check. It holds no secrets: those live in Secret Manager and nowhere else.

All `gcloud` commands use the `momentum` configuration, so they act as the personal account in the Momentum project whatever configuration is active.

## What runs where

| Piece | Where |
|---|---|
| Google Cloud | Project `task-manager-493414`; gcloud configuration `momentum` (account `jvrviegas@gmail.com`, region `europe-west3`) |
| Server | Cloud Run service `momentum-server` in `europe-west3`, at `https://momentum-server-466889193039.europe-west3.run.app`; minimum 0 and maximum 2 instances; service account `momentum-server@task-manager-493414.iam.gserviceaccount.com` |
| Images | Artifact Registry `europe-west3-docker.pkg.dev/task-manager-493414/momentum/momentum-server`, tagged by commit |
| Database | Neon project `momentum`, AWS `eu-central-1` (Frankfurt), PostgreSQL 18, database `neondb`. The server uses the pooled endpoint, migrations the direct one |
| Secrets | Secret Manager, replicated in `europe-west3` only: `momentum-database-url` (pooled), `momentum-database-url-direct`, `momentum-google-client-secret`, `momentum-github-client-secret`. The service account can read all but the direct URL |
| Sign-in | A Google OAuth client (Web application, in Testing with test user `jvrviegas@gmail.com`) and a GitHub OAuth app owned by `jvrviegas`, both with the callback `<server>/auth/callback/<provider>` |
| Logs | The server logs requests by route itself. Cloud Run's own request log is excluded from storage (exclusion `momentum-server-requests` on the `_Default` sink), because it keeps full URLs and so the sign-in codes in them |

The server's environment:

| Variable | Value |
|---|---|
| `PUBLIC_URL` | The server's address above |
| `ALLOWED_EMAILS` | `jvrviegas@gmail.com` |
| `GOOGLE_CLIENT_ID`, `GITHUB_CLIENT_ID` | The clients' ids, which are not secret |
| `TRUSTED_PROXY_HOPS` | `1`: Cloud Run's front end appends the client's address to `X-Forwarded-For`. With a load balancer in front, it would be `2`; check it again as below |
| `DATABASE_URL`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_SECRET` | From the secrets, version `latest` |

## Release a new version

From the repository root, on the commit to release:

```sh
V=$(git rev-parse --short HEAD)
IMG=europe-west3-docker.pkg.dev/task-manager-493414/momentum/momentum-server:$V

# Build for Cloud Run (linux/amd64) and push, with a token that expires in an hour.
gcloud auth print-access-token --configuration=momentum |
  docker login -u oauth2accesstoken --password-stdin https://europe-west3-docker.pkg.dev
docker buildx build --platform linux/amd64 -f backend/Dockerfile --build-arg VERSION=$V -t $IMG --push .
docker logout europe-west3-docker.pkg.dev

# Apply new migrations through the direct endpoint, before the new revision serves.
(cd backend && DATABASE_URL="$(gcloud secrets versions access latest --secret=momentum-database-url-direct --configuration=momentum)" \
  go run ./cmd/momentum-server migrate)

# Deploy. The new revision keeps the previous one's settings and secrets.
gcloud run deploy momentum-server --configuration=momentum --region=europe-west3 --image=$IMG
```

Migrations run while the previous revision still serves, so a migration must work with both: add before you remove, and drop what the old code reads only in a later release. Migrations are reviewed before they are applied (D9) and never edited once applied.

## Check a release

```sh
curl https://momentum-server-466889193039.europe-west3.run.app/health   # {"status":"ok"}
momentum doctor   # with [backend] url set: "answers; version <commit>", and the sign-in
```

To check that clients cannot forge their address past the sign-in limits, send polls with a different forged `X-Forwarded-For` each. The 301st must answer `slow_down`; if none does, `TRUSTED_PROXY_HOPS` trusts a forged entry. This limits polling from your address for up to 10 minutes, so run it after signing in.

```sh
U=https://momentum-server-466889193039.europe-west3.run.app
for i in $(seq 1 301); do
  curl -s -X POST -H "Content-Type: application/json" -H "X-Forwarded-For: 198.18.$((i/250)).$((i%250+1))" \
    --data "not json" $U/v1/auth/token | grep -q slow_down && { echo "limited at poll $i"; break; }
done
```

## Set up from nothing

This is how the service was set up on 2026-10-16 and 2026-10-17, for a new project or region.

1. Create the gcloud configuration and enable the services:

   ```sh
   gcloud config configurations create momentum --no-activate
   gcloud config set account jvrviegas@gmail.com --configuration=momentum
   gcloud config set project task-manager-493414 --configuration=momentum
   gcloud config set run/region europe-west3 --configuration=momentum
   gcloud services enable run.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com --configuration=momentum
   gcloud artifacts repositories create momentum --repository-format=docker --location=europe-west3 --configuration=momentum
   ```

2. Create the Neon project in the same city as Cloud Run. Copy each connection string to the clipboard and store it without printing it; the pooled one has `-pooler` in its host:

   ```sh
   pbpaste | tr -d '\r\n' | gcloud secrets create momentum-database-url --data-file=- \
     --replication-policy=user-managed --locations=europe-west3 --configuration=momentum
   ```

   Do the same for `momentum-database-url-direct` and, once the sign-in clients exist, `momentum-google-client-secret` and `momentum-github-client-secret`. Then clear the clipboard (`pbcopy < /dev/null`).

3. Register the sign-in clients with the callbacks `<server>/auth/callback/google` and `<server>/auth/callback/github`. The server's address is known before the first deploy: Cloud Run gives `https://<service>-<project number>.<region>.run.app`. Google's client can also list `http://localhost:8080/auth/callback/google` for local use; a GitHub OAuth app takes one callback.

4. Create the service account and let it read only what the server needs:

   ```sh
   gcloud iam service-accounts create momentum-server --configuration=momentum
   for s in momentum-database-url momentum-google-client-secret momentum-github-client-secret; do
     gcloud secrets add-iam-policy-binding $s --role=roles/secretmanager.secretAccessor --configuration=momentum \
       --member=serviceAccount:momentum-server@task-manager-493414.iam.gserviceaccount.com
   done
   ```

5. Exclude Cloud Run's request log. The exclusion takes about five minutes to apply, so add it before the service is public:

   ```sh
   gcloud logging sinks update _Default --configuration=momentum --add-exclusion='name=momentum-server-requests,filter=resource.type="cloud_run_revision" AND resource.labels.service_name="momentum-server" AND log_id("run.googleapis.com/requests")'
   ```

6. Build and push the image and apply the migrations as in a release, then deploy the first revision, private:

   ```sh
   gcloud run deploy momentum-server --configuration=momentum --region=europe-west3 --image=$IMG \
     --service-account=momentum-server@task-manager-493414.iam.gserviceaccount.com \
     --no-allow-unauthenticated --min-instances=0 --max-instances=2 \
     --set-env-vars=PUBLIC_URL=<server>,ALLOWED_EMAILS=jvrviegas@gmail.com,GOOGLE_CLIENT_ID=<id>,GITHUB_CLIENT_ID=<id>,TRUSTED_PROXY_HOPS=1 \
     --set-secrets=DATABASE_URL=momentum-database-url:latest,GOOGLE_CLIENT_SECRET=momentum-google-client-secret:latest,GITHUB_CLIENT_SECRET=momentum-github-client-secret:latest
   ```

7. Make it public. The browser and the providers' callbacks reach the server without Google identity tokens, so it must be; the apps sign in with their own tokens. The owner runs this step:

   ```sh
   gcloud run services add-iam-policy-binding momentum-server --region=europe-west3 \
     --member=allUsers --role=roles/run.invoker --configuration=momentum
   ```

   For a minute or two afterwards, requests with an `Authorization` header can get Cloud Run's HTML 401 page while others already pass; wait before checking sign-in.

## Day to day

- **Roll back:** `gcloud run revisions list --service=momentum-server --configuration=momentum`, then `gcloud run services update-traffic momentum-server --to-revisions=<revision>=100 --configuration=momentum`. Migrations do not roll back, so roll back only to a revision that works with the current schema.
- **Restore data:** Neon's point-in-time restore is the backup (D8). How far back it reaches depends on the Neon plan; see the project's settings in the Neon console.
- **Rotate a secret:** add a version (`pbpaste | gcloud secrets versions add <secret> --data-file=- --configuration=momentum`). Instances read `latest` when they start, so the new version takes over once the running ones stop, after about 15 minutes idle. To switch at once, change the revision's template, which starts a new revision: `gcloud run services update momentum-server --update-env-vars=SECRETS_ROTATED=$(date +%Y%m%d) --configuration=momentum`.
- **Allow another address:** commas separate variables in `--update-env-vars`, so use another delimiter: `gcloud run services update momentum-server --update-env-vars='^;^ALLOWED_EMAILS=a@example.com,b@example.com' --configuration=momentum`. Google sign-in also needs the address as a test user while the client is in Testing.
- **Read the server's log:** `gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="momentum-server" AND log_id("run.googleapis.com/stdout")' --configuration=momentum --freshness=1h`.
- **Take it offline for callers:** `gcloud run services remove-iam-policy-binding momentum-server --region=europe-west3 --member=allUsers --role=roles/run.invoker --configuration=momentum`.
- **Expect cold starts:** the first request after about 15 minutes idle takes 1 to 1.6 seconds, as both Cloud Run and Neon start (T01).
