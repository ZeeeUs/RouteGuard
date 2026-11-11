package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ZeeeUs/RouteGuard/internal/config"
)

type System struct {
	Commands Runner
	NetPath  string
}

func (s System) Config(ctx context.Context) (*config.Config, error) {
	out, err := s.Commands.Run(ctx, "", "uci", "-q", "export", "routeguard")
	if err != nil {
		return nil, err
	}

	return config.Parse(out)
}

func (s System) DeviceExists(device string) bool {
	_, err := os.Stat(filepath.Join(s.NetPath, device))
	return err == nil
}

func (s System) Offload(ctx context.Context) (bool, error) {
	for _, key := range []string{"flow_offloading", "flow_offloading_hw"} {
		out, err := s.Commands.Run(ctx, "", "uci", "-q", "get", "firewall.@defaults[0]."+key)
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				continue // An unset UCI option has exit status 1.
			}

			return false, err
		}

		enabled, err := config.ParseBool(strings.TrimSpace(string(out)))
		if err != nil || enabled {
			return enabled, err
		}
	}

	_, err := s.Commands.Run(ctx, "", "nft", "list", "flowtable", "inet", "fw4", "ft")
	if missingObject(err) {
		return false, nil
	}

	return err == nil, err
}

func (s System) Routes(ctx context.Context) (map[string][]string, error) {
	routes := map[string][]string{"-4": {}, "-6": {}}
	for _, family := range []string{"-4", "-6"} {
		out, err := s.Commands.Run(ctx, "", "ip", family, "route", "show", "default")
		if err != nil {
			return routes, err
		}

		fields := strings.Fields(string(out))
		for index, field := range fields {
			if field == "dev" && index+1 < len(fields) {
				routes[family] = append(routes[family], fields[index+1])
			}
		}

		slices.Sort(routes[family])
		routes[family] = slices.Compact(routes[family])
	}

	return routes, nil
}

func (s System) Log(ctx context.Context, message string) {
	_, _ = s.Commands.Run(ctx, "", "logger", "-t", "routeguard", "--", message)
}
