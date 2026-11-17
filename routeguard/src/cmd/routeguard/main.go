package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/ZeeeUs/RouteGuard/internal/app"
	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/sources"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func main() {
	// A GC budget, not a hard limit on RSS, child processes or kernel tables.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(12 << 20)
	}

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	runner := platform.Exec{}
	system := platform.System{Commands: runner, NetPath: "/sys/class/net"}
	engine := &app.App{
		Store:     store.Files{Path: "/etc/routeguard/state.json"},
		Firewall:  platform.Firewall{Commands: runner},
		System:    system,
		Scheduler: platform.Cron{Commands: runner, Path: "/etc/crontabs/root", Program: "/etc/init.d/cron"},
		Prober:    platform.TCPProbe{},
		Compiler:  policy.Compiler{Sources: &sources.Client{PrivateListsPath: "/etc/routeguard/private-list-hosts"}},
		RunPath:   "/var/run/routeguard", Executable: executable,
	}

	if err := run(context.Background(), engine, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runRPC(ctx context.Context, engine *app.App, method string) error {
	encoder := json.NewEncoder(os.Stdout)
	var request struct {
		ID       string `json:"id"`
		Revision string `json:"revision"`
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 16<<10)).Decode(&request); err != nil {
		return encoder.Encode(map[string]string{"error": "invalid RouteGuard request"})
	}

	switch method {
	case "job":
		return run(ctx, engine, []string{"job", request.ID})
	case "apply":
		if request.Revision == "" {
			return encoder.Encode(app.Queued{Error: "settings revision required; reload LuCI and retry"})
		}
		return run(ctx, engine, []string{"queue", "apply", request.Revision})
	default:
		return fmt.Errorf("invalid RouteGuard method")
	}
}

func writeRevision() error {
	encoder := json.NewEncoder(os.Stdout)
	const maxBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxBytes {
		return encoder.Encode(map[string]string{"error": "could not read staged RouteGuard settings"})
	}

	_, revision, err := config.ParseRPC(data)
	if err != nil {
		return encoder.Encode(map[string]string{"error": err.Error()})
	}
	return encoder.Encode(map[string]string{"revision": revision})
}

func run(ctx context.Context, engine *app.App, args []string) error {
	if len(args) == 2 && args[0] == "rpc" {
		return runRPC(ctx, engine, args[1])
	}
	if len(args) == 1 && args[0] == "revision" {
		return writeRevision()
	}
	if len(args) == 2 && args[0] == "job" {
		job, err := engine.Job(args[1])
		if err != nil {
			return json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		}

		return json.NewEncoder(os.Stdout).Encode(job)
	}

	if (len(args) == 2 || len(args) == 3) && args[0] == "queue" {
		revision := ""
		if len(args) == 3 {
			revision = args[2]
		}
		reply, err := engine.Queue(args[1], revision)
		if err != nil {
			reply.Error = err.Error()
		}

		return json.NewEncoder(os.Stdout).Encode(reply)
	}

	if len(args) == 4 && args[0] == "worker" {
		return engine.Worker(ctx, args[1], args[2], args[3])
	}

	if len(args) == 1 {
		switch args[0] {
		case "status":
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")

			status, err := engine.Status(ctx)
			if err != nil {
				return encoder.Encode(map[string]string{"error": err.Error()})
			}

			return encoder.Encode(status)
		case "apply", "refresh", "restore", "rollback", "stop", "recover", "diagnose":
			message, err := engine.Execute(ctx, args[0])
			if err == nil {
				fmt.Println(message)
			}

			return err
		}
	}

	return fmt.Errorf("usage: routeguard {apply|refresh|restore|rollback|status|stop|recover|diagnose}")
}
