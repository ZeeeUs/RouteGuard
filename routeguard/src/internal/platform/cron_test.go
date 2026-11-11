package platform

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/platform/mock"
)

//go:embed testdata/root.crontab
var cronFixture string

func TestCronIdempotenceAndOtherJobs(t *testing.T) {
	installed := editCron(cronFixture, "10")
	if next := editCron(installed, "10"); next != installed || strings.Count(next, cronSuffix) != 1 {
		t.Fatal("cron not idempotent")
	}
	updated := editCron(installed, "32")
	if strings.Count(updated, cronSuffix) != 1 || !strings.Contains(updated, "32"+cronSuffix) {
		t.Fatal("schedule not replaced")
	}
	if next := editCron(updated, ""); next != cronFixture {
		t.Fatal("other jobs modified")
	}
}

func TestDisabledOrMissingCronIsAnError(t *testing.T) {
	commands := mock.NewMockRunner(gomock.NewController(t))
	unavailable := errors.New("cron unavailable")
	gomock.InOrder(
		commands.EXPECT().Run(gomock.Any(), "", "missing-cron", "enabled").Return(nil, unavailable),
		commands.EXPECT().Run(gomock.Any(), "", "missing-cron", "running").Return(nil, unavailable),
		commands.EXPECT().Run(gomock.Any(), "", "missing-cron", "enabled").Return(nil, unavailable),
	)

	c := Cron{Path: filepath.Join(t.TempDir(), "crontab"), Program: "missing-cron", Commands: commands}
	if err := c.Update(context.Background(), "10"); err == nil {
		t.Fatal("reported scheduling success without cron")
	}

	status := c.Status(context.Background(), "10")
	if status.Error == "" || status.Running {
		t.Fatalf("misleading schedule status: %+v", status)
	}

	if err := c.Update(context.Background(), ""); err != nil {
		t.Fatal("removing absent schedule requires cron")
	}
}

func TestEnabledCronStartsAndInstallsSchedule(t *testing.T) {
	commands := mock.NewMockRunner(gomock.NewController(t))
	path := filepath.Join(t.TempDir(), "crontab")
	c := Cron{Path: path, Program: "cron", Commands: commands}

	commands.EXPECT().Run(gomock.Any(), "", "cron", "enabled").Return(nil, nil)
	if err := c.Check(context.Background(), "10"); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}

	gomock.InOrder(
		commands.EXPECT().Run(gomock.Any(), "", "cron", "enabled").Return(nil, nil),
		commands.EXPECT().Run(gomock.Any(), "", "cron", "running").Return(nil, errors.New("not running")),
		commands.EXPECT().Run(gomock.Any(), "", "cron", "start").DoAndReturn(
			func(context.Context, string, string, ...string) ([]byte, error) {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "10"+cronSuffix+"\n" {
					t.Fatalf("cron started before installing its job: %q, %v", data, err)
				}

				return nil, nil
			}),
		commands.EXPECT().Run(gomock.Any(), "", "cron", "running").Return(nil, nil),
		commands.EXPECT().Run(gomock.Any(), "", "cron", "enabled").Return(nil, nil),
	)
	if err := c.Update(context.Background(), "10"); err != nil {
		t.Fatal(err)
	}

	commands.EXPECT().Run(gomock.Any(), "", "cron", "running").Return(nil, nil)
	commands.EXPECT().Run(gomock.Any(), "", "cron", "enabled").Return(nil, nil)
	status := c.Status(context.Background(), "10")
	if !status.Enabled || !status.Running || !status.Installed || status.Error != "" {
		t.Fatalf("schedule=%+v", status)
	}
}
