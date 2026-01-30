# Test Utilities

This folder contains a manual + automated test catalog and k6 scripts.
No CI/CD changes are required; these scripts are run locally or can be wired into CI later.

## Prereqs
- Go 1.23+
- k6 installed (`brew install k6` on macOS)

## Run the server locally
```sh
go build -o rtty .
./rtty run zsh -p 8080 -v
```

## Run k6 scripts
Set environment variables to target a running server.

```sh
export RTTY_WS_URL=ws://localhost:8080/ws
export RTTY_ORIGIN=http://localhost:8080

k6 run test/k6/ws_lifecycle.js
k6 run test/k6/ws_concurrency.js
k6 run test/k6/ws_stress_soak.js
```

## Run Go tests
```sh
go test ./... -json | tparse -format markdown
go test -tags=localstack ./... -json | tparse -format markdown
```

Go test suite now includes a k6 lifecycle test. It will be skipped if `k6`
is not installed or `cat` is unavailable on the system PATH.

## Scenario tuning
All scripts support env overrides. Examples:

```sh
# Run only the 500 VU scenario
RTTY_SCENARIO=500 k6 run test/k6/ws_concurrency.js

# Stress profile customization
RTTY_STRESS_MAX_VUS=300 RTTY_STRESS_DURATION=2m k6 run test/k6/ws_stress_soak.js

# Soak profile customization
RTTY_SOAK_VUS=50 RTTY_SOAK_DURATION=2h k6 run test/k6/ws_stress_soak.js
```

## Manual test catalog
See `test/catalog/TEST_CASES.md` for detailed manual and automated test cases,
including browser compatibility checks for Chrome/Firefox/Safari.
