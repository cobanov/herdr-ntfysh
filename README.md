# herdr-ntfysh

Push [ntfy](https://ntfy.sh) notifications when a [herdr](https://herdr.dev)
agent finishes its work or blocks waiting for your input.

You start a long task, switch to something else, and your phone (or desktop)
pings the moment the agent needs you — no more babysitting the sidebar.

```
🟢  Claude - done            [herd]
    tests green
    📍 api › main

🔴  Codex - needs input      [herd]
    awaiting approval to run migration
    📍 web › deploy
```

## Why another ntfy plugin?

This is a ground-up rewrite of the idea behind
[zom-2018/herdr-ntfy-notify](https://github.com/zom-2018/herdr-ntfy-notify),
built to the quality bar of the first-class herdr plugins:

| | herdr-ntfysh | zom-2018/herdr-ntfy-notify |
|---|---|---|
| Language | Go, single static binary | Node `.mjs` |
| Runtime deps | **none** (Go stdlib only) | requires `node` + shells out to `curl` |
| Debounce / dedup | ✅ per-pane, windowed, persisted in state dir | ❌ (can re-fire on repeated events) |
| Self-signed TLS | ✅ custom CA **or** insecure toggle | partial |
| Config discovery | explicit, documented lookup order | opaque path generation |
| Header safety | ASCII-clean headers, emoji via ntfy tags | raw emoji in headers |
| `--doctor` config check | ✅ | ❌ |
| Failure policy | never disrupts herdr's event pipeline | unclear |
| Tests | unit-tested + e2e smoke | unit only |

## Requirements

- herdr `>= 0.7.0`
- Go `>= 1.23` on the machine where you install (only needed at install time
  to compile the binary)
- An ntfy server and topic (ntfy.sh or self-hosted)

## Install

```bash
herdr plugin install cobanov/herdr-ntfysh
```

herdr compiles the binary during install (`go build`). For local development:

```bash
git clone https://github.com/cobanov/herdr-ntfysh
cd herdr-ntfysh
go build -o herdr-ntfysh .
herdr plugin link "$PWD"
```

## Configure

Copy the example config into the plugin's config directory and edit it:

```bash
cp .env.example "$(herdr plugin config-dir cobanov.herdr-ntfysh)/.env"
$EDITOR "$(herdr plugin config-dir cobanov.herdr-ntfysh)/.env"
```

Minimum required values:

```ini
HERDR_NTFY_SERVER=https://ntfy.example.com
HERDR_NTFY_TOPIC=herd
```

Any setting can also be provided as a process environment variable of the same
name, which overrides the file. See [`.env.example`](./.env.example) for the
full list.

### Self-hosted ntfy

Auth (choose at most one — a token wins over basic credentials):

```ini
HERDR_NTFY_TOKEN=tk_xxxxxxxxxxxx          # ntfy access token (Bearer)
# or
HERDR_NTFY_USERNAME=herder
HERDR_NTFY_PASSWORD=changeme
```

TLS for a private cert — prefer pinning your CA over disabling verification:

```ini
HERDR_NTFY_CA_FILE=/etc/ssl/certs/my-ntfy-ca.pem   # recommended
HERDR_NTFY_TLS_INSECURE=true                        # last resort
```

## Verify

```bash
# Print the resolved config (secrets redacted):
herdr plugin action invoke cobanov.herdr-ntfysh.doctor

# Send a test push through the whole pipeline:
herdr plugin action invoke cobanov.herdr-ntfysh.test
```

You can bind the test action to a key in `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+n"
type = "shell"
command = "herdr plugin action invoke cobanov.herdr-ntfysh.test"
```

## Behavior

- **Triggers** on herdr's `pane.agent_status_changed` event.
- **Notifies** on `done` and `blocked` by default. Change with
  `HERDR_NTFY_NOTIFY_ON` (any of `done,blocked,working,idle`).
- **Priority** defaults to `high` for `blocked`, `default` for `done`.
  Override per status (`HERDR_NTFY_PRIORITY_BLOCKED`, etc.).
- **Debounces** an identical status for the same pane within
  `HERDR_NTFY_DEDUP_WINDOW` seconds (default 10) so a flapping agent can't spam
  you. State lives in herdr's plugin state dir.
- **Fails safe**: a bad config or an unreachable ntfy server is logged to
  stderr (`herdr plugin log list --plugin cobanov.herdr-ntfysh`) and the
  process exits cleanly, never disrupting herdr.

## Troubleshooting

- **No notifications?** Run `...doctor` to confirm the server/topic/auth are
  what you expect, then `...test`. Check
  `herdr plugin log list --plugin cobanov.herdr-ntfysh`.
- **TLS errors** against a self-hosted server: set `HERDR_NTFY_CA_FILE` to your
  CA bundle (preferred) or `HERDR_NTFY_TLS_INSECURE=true`.
- **Too noisy?** Increase `HERDR_NTFY_DEDUP_WINDOW`, or trim
  `HERDR_NTFY_NOTIFY_ON` to just `blocked`.

## License

MIT — see [LICENSE](./LICENSE).
