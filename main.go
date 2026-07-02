// Command herdr-ntfysh is a herdr plugin that publishes ntfy notifications
// when a herdr-managed agent changes state (e.g. finishes work or blocks
// waiting for your input).
//
// herdr invokes this binary in two ways:
//
//   - As an event hook on "pane.agent_status_changed". herdr sets
//     HERDR_PLUGIN_EVENT_JSON and the standard plugin runtime variables, and
//     we decide whether the status warrants a push.
//   - As an action ("--test", "--doctor") the user triggers manually to
//     verify their configuration.
//
// The design goal is to be invisible when it works and safe when it doesn't:
// any misconfiguration or ntfy outage is logged to stderr (captured by
// `herdr plugin log list`) and the process exits 0 so the herdr event
// pipeline is never disrupted. Only the user-facing --test/--doctor actions
// exit non-zero on failure.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cobanov/herdr-ntfysh/internal/config"
	"github.com/cobanov/herdr-ntfysh/internal/dedup"
	"github.com/cobanov/herdr-ntfysh/internal/event"
	"github.com/cobanov/herdr-ntfysh/internal/ntfy"
	"github.com/cobanov/herdr-ntfysh/internal/render"
)

const version = "0.1.0"

// logf writes a diagnostic line to stderr. herdr captures plugin stderr and
// exposes it via `herdr plugin log list --plugin cobanov.herdr-ntfysh`.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "herdr-ntfysh: "+format+"\n", args...)
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		testMode   bool
		doctorMode bool
		showVer    bool
	)
	flag.BoolVar(&testMode, "test", false, "send a test notification and exit")
	flag.BoolVar(&doctorMode, "doctor", false, "print resolved configuration (secrets redacted) and exit")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Parse()

	if showVer {
		fmt.Println("herdr-ntfysh", version)
		return 0
	}

	// Whether a failure should be surfaced to the user (exit non-zero) or
	// swallowed to protect herdr's event pipeline.
	userFacing := testMode || doctorMode

	cfg, err := config.Load()
	if err != nil {
		logf("config error: %v", err)
		if userFacing {
			return 1
		}
		return 0
	}

	if doctorMode {
		cfg.PrintRedacted(os.Stdout)
		return 0
	}

	if !cfg.Enabled {
		logf("disabled (HERDR_NTFY_ENABLED=false), skipping")
		return 0
	}

	client := ntfy.New(cfg)

	if testMode {
		if err := client.Publish(render.TestMessage(cfg)); err != nil {
			logf("test notification failed: %v", err)
			return 1
		}
		fmt.Printf("herdr-ntfysh: test notification sent to %s/%s\n", cfg.Server, cfg.Topic)
		return 0
	}

	// Event mode: triggered by herdr on pane.agent_status_changed.
	ev, err := event.FromEnv()
	if err != nil {
		logf("cannot read event: %v", err)
		return 0
	}

	status := ev.Status()
	if status == "" {
		logf("event carried no agent status, skipping")
		return 0
	}
	if !cfg.NotifyOn[status] {
		// A status we are not configured to announce; silent success.
		return 0
	}

	// Debounce duplicate emissions of the same status for the same pane so a
	// flapping agent (or a re-emitted event) cannot spam the user.
	store := dedup.Open(cfg)
	if !store.ShouldNotify(ev.PaneKey(), status) {
		logf("debounced duplicate %q for %s", status, ev.PaneKey())
		return 0
	}

	if err := client.Publish(render.EventMessage(cfg, ev)); err != nil {
		// A transient ntfy outage must not break herdr; log and move on.
		logf("publish failed: %v", err)
		return 0
	}
	store.Record(ev.PaneKey(), status)
	logf("notified %q for %s", status, ev.PaneKey())
	return 0
}
