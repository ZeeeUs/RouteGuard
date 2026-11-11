package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/nft"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
)

func TestErrorsAndInterruptedJobsAreVisible(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	if err := a.writeJob(newOperation("apply")); err != nil {
		t.Fatal(err)
	}

	a.firewall.EXPECT().Check(gomock.Any(), nil).Return(nft.Health{Matches: true}, nil)
	a.system.EXPECT().Config(gomock.Any()).Return(&config.Config{}, nil)
	a.system.EXPECT().Routes(gomock.Any()).Return(map[string][]string{}, nil)
	a.system.EXPECT().Offload(gomock.Any()).Return(false, nil)
	a.scheduler.EXPECT().Status(gomock.Any(), "").Return(platform.Schedule{})
	status, err := a.Status(context.Background())
	if err != nil || status.Busy || status.Job.State != "interrupted" {
		t.Fatalf("stale job: %+v, %v", status, err)
	}

	a.system.EXPECT().Config(gomock.Any()).Return(nil, errors.New("early configuration error"))
	if _, err := a.Execute(context.Background(), "apply"); err == nil {
		t.Fatal("expected early error")
	}

	job, _, err := a.jobStatus()
	if err != nil || job.State != "error" || !strings.Contains(job.Message, "early") {
		t.Fatalf("lost early error: %+v, %v", job, err)
	}
}

func TestBusyLockRejectsQueue(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	lock, err := a.lock(false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	if _, err := a.Queue("apply"); !errors.Is(err, ErrBusy) {
		t.Fatalf("queue error: %v", err)
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "worker" {
		// Queue starts the same executable with its production argument layout.
		// Route that child to a normal test so GoMock uses testing.T and cleanup.
		if err := os.Setenv("ROUTEGUARD_TEST_WORKER", os.Args[2]+":"+os.Args[3]); err != nil {
			os.Exit(1)
		}
		os.Args = []string{os.Args[0], "-test.run=^TestWorkerProcess$"}
	}

	os.Exit(m.Run())
}

func TestWorkerProcess(t *testing.T) {
	action, id, child := strings.Cut(os.Getenv("ROUTEGUARD_TEST_WORKER"), ":")
	if !child {
		t.Skip("executed only by the queue subprocess test")
	}

	a := fixtureApp(t, os.Getenv("ROUTEGUARD_TEST_DIRECTORY"))
	a.system.EXPECT().Config(gomock.Any()).Return(&config.Config{}, nil)
	a.scheduler.EXPECT().Check(gomock.Any(), "").Return(nil)
	a.firewall.EXPECT().Check(gomock.Any(), nil).Return(nft.Health{Matches: true}, nil)
	a.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil)
	if err := a.Worker(context.Background(), action, id); err != nil {
		t.Fatal(err)
	}
}

func TestQueueAcknowledgesAndCompletesActualChild(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ROUTEGUARD_TEST_DIRECTORY", directory)
	a := fixtureApp(t, directory)
	var err error
	a.Executable, err = os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	reply, err := a.Queue("apply")
	if err != nil || !reply.Queued || reply.ID == "" {
		t.Fatalf("queue: %+v, %v", reply, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, busy, err := a.jobStatus()
		if err != nil {
			t.Fatal(err)
		}

		if !busy {
			if job.ID != reply.ID || job.State != "ok" {
				t.Fatalf("worker result: %+v", job)
			}

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("worker did not finish")
}
