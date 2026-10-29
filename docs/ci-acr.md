# CI images on Aliyun Container Registry

The Chengdu server must not compile images. Two local builds hung the machine.
GitHub Actions builds the image `docker-compose.yaml` used to build itself
(`teamgram`, from the root `Dockerfile`) and pushes it to ACR. Deploy is a pull.

The workflow runs on every push to `master` and from the Actions tab
(`workflow_dispatch`).

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

GitHub-hosted runners are not on the VPC, so they push to the public regional
endpoint `crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com`. The Chengdu server pulls the same
repository through the VPC endpoint `crpi-tge4xmtf4iip17mq-vpc.cn-chengdu.personal.cr.aliyuncs.com`.
Tags match. Only the host differs.

`coturn` and `mediamtx` stay on their upstream images. This workflow does not
build them.

Buildx stores layer cache in GitHub Actions (`type=gha`). Provenance
attestations are off so ACR accepts the manifest.

## Deploy

On the server, from a checkout that contains `docker-compose.yaml`. Log in once
if the namespace is private. Use the VPC host, not the public one:

```bash
docker login crpi-tge4xmtf4iip17mq-vpc.cn-chengdu.personal.cr.aliyuncs.com
docker compose pull && docker compose up -d
```

`TELLOMI_IMAGE_REGISTRY` defaults to
`crpi-tge4xmtf4iip17mq-vpc.cn-chengdu.personal.cr.aliyuncs.com/tellomi` and `TELLOMI_IMAGE_TAG` defaults
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
