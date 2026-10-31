# CI images on Aliyun Container Registry

The Chengdu server must not compile images. Two local builds hung the machine.
GitHub Actions builds the image `docker-compose.yaml` used to build itself
(`teamgram`, from the root `Dockerfile`) and pushes it to ACR. Deploy is a pull.

The workflow runs on every push to `master` and from the Actions tab
(`workflow_dispatch`). A new push to the same ref cancels the in-progress
build, so an older run cannot finish later and move `:latest` backwards.

## Secrets

Set these as repository Actions secrets. The workflow logs in with
`docker/login-action` and pushes
`${ACR_REGISTRY}/${ACR_NAMESPACE}/teamgram`, tagged with the 7-character commit
SHA and `latest`.

| Secret | Purpose | When unset |
|---|---|---|
| `ACR_REGISTRY` | Registry host the workflow pushes to | `crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com` |
| `ACR_NAMESPACE` | ACR namespace | `tellomi` |
| `ACR_USERNAME` | Registry username | required |
| `ACR_PASSWORD` | Registry password | required |

The workflow pushes to the public endpoint
`crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com`. The application
server is not in the VPC, so Compose pulls that same host. The `-vpc` endpoint
is not reachable there.

`coturn` and `mediamtx` stay on their upstream images. This workflow does not
build them.

Buildx stores layer cache in GitHub Actions (`type=gha`). Provenance
attestations are off so ACR accepts the manifest.

## Deploy

Staging deploys wait for the simulator UI acceptance workflow
(`.github/workflows/ui-acceptance.yml`). It runs from the Actions tab
(`workflow_dispatch`) and when a pull request is labeled `deploy-staging`,
against `staging-api.tellomi.cn`. Do not pull a new image onto the staging
host until that workflow's `ready-for-deploy` job is green. The checklist,
screenshots, and secrets are in [e2e/ui/README.md](../e2e/ui/README.md).
This image-publish workflow is not that gate.

On the server, from a checkout that contains `docker-compose.yaml`. Log in once
if the namespace is private:

```bash
docker login crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com
docker compose pull && docker compose up -d
```

The new image applies `teamgramd/deploy/sql/migrate-*.sql` before the processes
start. Pull this checkout as well so `docker-compose.yaml` passes
`MYSQL_ROOT_PASSWORD` into that step. The app DSN in `etc2` is unchanged.

`TELLOMI_IMAGE_REGISTRY` defaults to
`crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com/tellomi` and `TELLOMI_IMAGE_TAG` defaults
to `latest`, so a stock `.env` (see `.env.example`) needs no image settings.
Do not pass `--build`. If `docker compose pull` fails, fix the login or the
registry. Do not build on this machine.

Dependency images (MySQL, Kafka, and the rest) are unchanged:

```bash
docker compose -f docker-compose-env.yaml up -d
docker compose pull && docker compose up -d
```

## Rollback

Each push is tagged with the first 7 characters of the commit SHA (the short
SHA on the commit and on the Actions run) and `latest` moves to that build.
To run an older build, set `TELLOMI_IMAGE_TAG` to that prefix and pull again.
For commit `9deb01fc…` the tag is `9deb01f`:

```bash
# .env
TELLOMI_IMAGE_TAG=9deb01f
```

```bash
docker compose pull && docker compose up -d
```

To return to the newest build, set `TELLOMI_IMAGE_TAG=latest` (or remove it)
and run the same pull and up.

## Local development

`build:` is still on the `teamgram` service. On a workstation:

```bash
docker compose up -d --build
```

That tags the local image with the same name Compose would pull. It does not
push.
