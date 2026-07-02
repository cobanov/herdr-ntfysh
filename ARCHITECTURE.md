# Architecture

herdr-ntfysh is a short-lived CLI that herdr spawns per event. It reads the
event and configuration, decides whether to notify, publishes to ntfy, and
exits. There is no daemon and no persistent connection.

## Invocation model

herdr runs the binary in three situations, all through the same `main`:

| Trigger | herdr config | Mode |
|---|---|---|
| Agent status change | `[[events]] on = "pane.agent_status_changed"` | event |
| Manual test | `[[actions]] id = "test"` → `--test` | user-facing |
| Config inspection | `[[actions]] id = "doctor"` → `--doctor` | user-facing |

Because the binary is re-spawned on every status change, startup cost matters.
A compiled Go binary with no runtime and no `node_modules` starts in
low-single-digit milliseconds, which is why Go was chosen over the Node
approach of the prior art.

## Failure policy

The single most important design rule: **the event path never fails loudly.**
A misconfigured `.env`, an unreachable ntfy server, or a malformed event is
logged to stderr (which herdr captures) and the process exits `0`. If this
plugin crashed or exited non-zero on every event, it could disrupt herdr's
event pipeline — a notifier is not worth that.

The `--test` and `--doctor` actions are the exception: they are invoked
deliberately by a human who wants a pass/fail signal, so they exit non-zero on
error.

## Packages

```
main.go                 orchestration + the fail-safe exit-code policy
internal/config         resolve + validate settings (env > .env file)
internal/event          parse HERDR_PLUGIN_EVENT_JSON, with HERDR_* fallbacks
internal/render         event + config  ->  ntfy.Message (title/body/tags)
internal/ntfy           dependency-free HTTP client (auth + custom TLS)
internal/dedup          per-pane windowed debounce, persisted as JSON
```

Dependency direction is acyclic: `render` and `ntfy` depend on `config`;
`main` depends on everything. Nothing depends on `main`.

## Configuration resolution

Two layers, highest priority first:

1. Process environment (`HERDR_NTFY_*`) — herdr passes the invoking
   environment through, useful for overrides and testing.
2. A `.env` file, looked up in order: `HERDR_NTFY_ENV_FILE`, then
   `HERDR_PLUGIN_CONFIG_DIR/.env` (the durable per-plugin location herdr
   assigns), then `./.env`.

Validation only runs when the plugin is enabled, so a disabled plugin never
errors on a missing server/topic. Secrets are redacted in every output path.

## Event payload

The `pane.agent_status_changed` payload is read defensively. herdr versions
differ, so every field is optional; missing location fields fall back to the
`HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` / `HERDR_PANE_ID` runtime variables.
The fields consumed are:

```
data.agent_status         idle | working | blocked | done
data.display_agent        friendly agent name (falls back to data.agent)
data.workspace, data.tab  location breadcrumb
data.pane_id              stable key for debounce bookkeeping
data.custom_status        free-form status text
data.state_labels.task    detail preferred for done/working
data.state_labels.error   detail preferred for blocked
```

## ntfy protocol choices

- Notifications use ntfy's HTTP header protocol (`X-Title`, `X-Priority`,
  `X-Tags`, …) over a plain `POST`, so there are no client libraries to
  maintain.
- **Emoji are ntfy tag shortcodes** (`white_check_mark`, `rotating_light`),
  not raw Unicode. HTTP header values must stay ISO-8859-1/ASCII clean, so
  the title uses an ASCII separator and lets ntfy render the emoji from tags.
  Free-form Unicode only ever appears in the request body.
- Auth precedence: an access token (Bearer) beats basic credentials.
- TLS: a custom CA bundle is preferred; `InsecureSkipVerify` is an explicit,
  documented last resort.

## Debounce

herdr can re-emit an event, and an agent can flap between states, so a single
logical "done" could otherwise become a burst of pushes. `internal/dedup`
records the last-notified `{status, timestamp}` per pane key in a small JSON
file under `HERDR_PLUGIN_STATE_DIR`, and suppresses an identical status within
the configured window. It writes via a temp-file rename so a crash can't leave
truncated state. When no state dir is available (e.g. a standalone `--test`),
debouncing is disabled rather than failing.
