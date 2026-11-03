# Restore drill record

D-34, D-53, M7. Copy this file to `docs/ops/drills/YYYY-MM-DD.md` after the drill. Do not put secrets, the age identity, or R2 tokens in it.

| Field | Value |
|---|---|
| Date (UTC) | |
| Operator | |
| Drill host | |
| Image tag (7 hex) | |
| Backup stamp | |
| Git SHA in the manifest | |
| Started (UTC) | |
| Finished (UTC) | |
| Total seconds | |
| Within 2 hours | yes / no |

Target is 120 minutes from the first step to the message check.

| Step | Start (UTC) | End (UTC) | Seconds | Result | Notes |
|---|---|---|---|---|---|
| install-tools | | | | | age, rclone, docker, python3, go |
| pull-images | | | | | `TELLOMI_IMAGE_TAG` from ACR |
| fetch-backups | | | | | read-only token, mysql/ etcd/ redis/ |
| decrypt | | | | | offline age identity, on this host only |
| restore-mysql | | | | | |
| restore-redis | | | | | no FLUSH; pts and message_box counters scanned |
| restore-etcd | | | | | snapshot restore before etcd starts |
| compose-up | | | | | project `tellomi-drill` |
| handshake | | | | | `TestInitConnectionInvokeWithLayer` |
| login | | | | | `TestHelpGetConfig` (`auth.signIn` via the test phone) |
| message | | | | | `TestPrivateChatReadOutbox` |

Result is `pass` or `fail`. Notes stay free of hostnames that are secret, and free of key material.
