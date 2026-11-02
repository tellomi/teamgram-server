## What changed

<!-- One short paragraph. Link the issue if there is one. -->

## API

- [ ] This PR does not add or change an RPC.
- [ ] This PR adds or changes an RPC, and it includes an e2e case (`e2e/client`, including a row in the implemented-method smoke list when the method is P0 or P1).
- [ ] `docs/api-coverage.csv` was regenerated with `go run ./tools/apicoverage` and the diff is in this PR.

See `docs/testing.md`.

## Checks

- [ ] `go test ./...` passes.
- [ ] Client regression passes when server behavior a Telegram client can see has changed.
