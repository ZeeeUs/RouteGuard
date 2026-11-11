package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

const cronSuffix = " * * * * /usr/sbin/routeguard refresh # routeguard-managed"

type Schedule struct {
	Minute    string `json:"minute"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
	Error     string `json:"error,omitempty"`
}

type Cron struct {
	Commands Runner
	Path     string
	Program  string
}

func editCron(original, minute string) string {
	var edited strings.Builder
	for _, line := range strings.SplitAfter(original, "\n") {
		value, managed := strings.CutSuffix(strings.TrimSuffix(line, "\n"), cronSuffix)
		_, err := config.Minute(value)
		if !managed || value == "" || err != nil {
			edited.WriteString(line)
		}
	}

	if minute != "" {
		if edited.Len() > 0 && !strings.HasSuffix(edited.String(), "\n") {
			edited.WriteByte('\n')
		}

		edited.WriteString(minute + cronSuffix + "\n")
	}

	return edited.String()
}

func (c Cron) Check(ctx context.Context, minute string) error {
	if _, err := config.Minute(minute); err != nil || minute == "" {
		return err
	}

	_, enabledErr := c.Commands.Run(ctx, "", c.Program, "enabled")
	if enabledErr != nil {
		return fmt.Errorf("scheduled updates require the system cron service to be enabled: %w", enabledErr)
	}

	return nil
}

func (c Cron) Update(ctx context.Context, minute string) error {
	if err := c.Check(ctx, minute); err != nil {
		return err
	}

	original, err := os.ReadFile(c.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	edited := editCron(string(original), minute)
	changed := edited != string(original)
	if !changed && minute == "" {
		return nil
	}

	if changed {
		if err := store.AtomicWrite(c.Path, func(file *os.File) error {
			_, err := file.WriteString(edited)
			return err
		}); err != nil {
			return err
		}
	}

	// OpenWrt refuses to start cron while /etc/crontabs is empty. Install the
	// job first, then start or reload the enabled service and verify liveness.
	_, runningErr := c.Commands.Run(ctx, "", c.Program, "running")
	action := ""
	if runningErr != nil && minute != "" {
		action = "start"
	} else if runningErr == nil && changed {
		action = "restart"
	}

	if action != "" {
		if _, err := c.Commands.Run(ctx, "", c.Program, action); err != nil {
			return fmt.Errorf("cron %s failed: %w", action, err)
		}
	}

	if minute != "" {
		if _, err := c.Commands.Run(ctx, "", c.Program, "running"); err != nil {
			return fmt.Errorf("cron is not running: %w", err)
		}
	}

	return c.Check(ctx, minute)
}

func (c Cron) Status(ctx context.Context, minute string) Schedule {
	result := Schedule{Minute: minute}
	if minute == "" {
		return result
	}

	data, err := os.ReadFile(c.Path)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			result.Installed = result.Installed || line == minute+cronSuffix
		}
	}

	_, runningErr := c.Commands.Run(ctx, "", c.Program, "running")
	_, enabledErr := c.Commands.Run(ctx, "", c.Program, "enabled")
	result.Running, result.Enabled = runningErr == nil, enabledErr == nil
	if !result.Installed || !result.Running || !result.Enabled {
		result.Error = "scheduled updates are not operational; check the managed job and cron service"
	}

	return result
}
