package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/ZeeeUs/RouteGuard/internal/app"
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
	engine := &app.App{
		Store:     store.Files{Path: "/etc/routeguard/state.json"},
		Firewall:  platform.Firewall{Commands: runner},
		System:    platform.System{Commands: runner, NetPath: "/sys/class/net"},
		Scheduler: platform.Cron{Commands: runner, Path: "/etc/crontabs/root", Program: "/etc/init.d/cron"},
		Compiler:  policy.Compiler{Sources: &sources.Client{}},
		RunPath:   "/var/run/routeguard", Executable: executable,
	}

	if err := run(context.Background(), engine, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, engine *app.App, args []string) error {
	if len(args) == 2 && args[0] == "job" {
		job, err := engine.Job(args[1])
		if err != nil {
			return json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		}

		return json.NewEncoder(os.Stdout).Encode(job)
	}

	if len(args) == 2 && args[0] == "queue" {
		reply, err := engine.Queue(args[1])
		if err != nil {
			reply.Error = err.Error()
		}

		return json.NewEncoder(os.Stdout).Encode(reply)
	}

	if len(args) == 3 && args[0] == "worker" {
		return engine.Worker(ctx, args[1], args[2])
	}

	if len(args) == 1 {
		switch args[0] {
		case "status":
			status, err := engine.Status(ctx)
			if err != nil {
				return json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			}

			return json.NewEncoder(os.Stdout).Encode(status)
		case "apply", "refresh", "restore", "rollback", "stop", "recover":
			message, err := engine.Execute(ctx, args[0])
			if err == nil {
				fmt.Println(message)
			}

			return err
		}
	}

	return fmt.Errorf("usage: routeguard {apply|refresh|restore|rollback|status|stop|recover}")
}
