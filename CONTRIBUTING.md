# Contributing

## Development

```sh
scripts/dev.sh                                              # run Mayoi Dev with live reload, with its own settings
go test ./...                                               # headless UI tests
go test -short ./...                                        # skip the tests that start a shell
go test -short -run XXX -bench Frame ./cmd/mayoi            # benchmark an editor frame
SHOTS=$PWD/docs go test -run TestShotReadme ./cmd/mayoi     # redraw the README screenshots
go run ./tools/icongen                                      # redraw resources/icon.png
```

## Releasing

Write the release notes under `## Unreleased` in `CHANGELOG.md` and commit them, then:

```sh
scripts/bump.sh patch    # or minor, major, or a version such as 0.2.0
```

It sets the version in `mygo.json`, renames `## Unreleased` to `## <version>` in `CHANGELOG.md`, commits and tags `v<version>`, and after asking pushes `main` and the tag. The
[Release workflow](.github/workflows/release.yml) then builds macOS (arm64 and Intel, signed and notarized), Windows and
Linux, uploads them with their signed updates, and publishes the release. Installed apps check for it once a day, and
from **Mayoi → Check for Updates…**; the update window shows the changelog section.

The workflow needs these repository secrets:

| Secret | What |
|---|---|
| `MYGO_UPDATER_PRIVATE_KEY` | `mygo-update.key` from `go tool mygo keygen`, whose public key is in `mygo.json` |
| `MACOS_CERTIFICATE` | the Developer ID Application certificate and its key, as a base64 `.p12` |
| `MACOS_CERTIFICATE_PASSWORD` | the password of the `.p12` |
| `APPLE_ID`, `APPLE_TEAM_ID` | the Apple ID of the developer account, and its team ID |
| `APPLE_APP_PASSWORD` | an app-specific password of the Apple ID, from account.apple.com |

Local builds stay signed ad hoc; only the workflow signs with the Developer ID. Windows builds are not signed.
