# Testing

Two suites gate a pull request.

`go test ./...` runs in the **Go test / go-test** check. It is the in-process suite: handlers, schema migrations, and the in-memory cases under `e2e/inmem`. JPEG tests skip when `app/service/dfs/internal/imaging/testdata` is absent. FFmpeg tests skip when the `ffmpeg` binary or `/tmp/ffmpeg` fixtures are absent. Static photo derivatives link govips, so the job installs `libvips-dev` and runs with `CGO_ENABLED=1`. GIF, video frames and plain files do not use it. `scripts/ca_bundle_test.go` checks that the runtime image installs `ca-certificates` and that `scripts/verify-ca-bundle.sh` rejects an empty trust store. The Dockerfile runs that script after the runtime `apt-get`, so the image build fails when `/etc/ssl/certs/ca-certificates.crt` is missing.

`e2e/client` is its own module. **Client regression / e2e** starts the compose stack and runs it. `TELLOMI_E2E=1` is required; without that variable the server tests skip. In-memory cases stay in `e2e/inmem` so they stay on the root module and `go test ./...` still compiles them.

## Adding an endpoint

A pull request that adds or changes an RPC does three things:

1. Implement the route and the handler. "Implemented" means both exist. See `docs/TELLOMI_RULES.md`.
2. Regenerate the coverage docs from the repo root:

   ```
   go run ./tools/apicoverage
   ```

   Commit `docs/api-coverage.csv` and `docs/api-coverage.md`. The API coverage check fails when they drift.
3. Cover the method from the gotd client in `e2e/client`.

   For a method marked implemented at P0 or P1, regenerate the smoke list:

   ```
   python3 e2e/client/gen_smoke.py
   ```

   `TestZZImplementedSmoke` calls every row once with the smallest params that still encode. It fails when the server replies `*_NOT_IMPLEMENTED` or the response does not decode. A 400 from those dummy params is expected and does not fail the test.

   `cd e2e/client && go test -run TestSmokeRequestsEncode` checks the request encodes and decodes again without a server. The session service drops a payload it cannot decode and the client reports that as a missing ack. If a new param type panics or returns "invalid predicate", add that type to `smokeEmptyClass` in `e2e/client/smoke_empty.go` with a constructor that has no required nested objects (an `*Empty` constructor when the schema has one). A field that shares its flag bit with another field is left unset when setting it alone does not decode.

   A behavior a client can see — a decode bug, a missing field, a wrong error — also gets a named `Test...` next to the existing cases, not only a smoke row. `TestStartupRPCs` stays the list clients send while the chat list says Updating.

The pull request template asks for both the e2e case and the regenerated coverage file.

## Branch protection

Require these checks on `master`. The names are the ones GitHub shows after the first run:

- `API coverage / coverage`
- `Compose config / minio-profile`
- `Go test / go-test`
- `Client regression / e2e`
