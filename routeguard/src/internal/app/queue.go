package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Queued struct {
	Queued bool   `json:"queued"`
	ID     string `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

func queuedAction(action string) bool {
	return action == "apply" || action == "refresh" || action == "rollback" || action == "diagnose"
}

func (a *App) Queue(action string) (Queued, error) {
	if !queuedAction(action) {
		return Queued{}, fmt.Errorf("invalid queued action")
	}

	lock, err := a.lock(false)
	if err != nil {
		return Queued{}, err
	}
	defer lock.Close()

	reader, writer, err := os.Pipe()
	if err != nil {
		return Queued{}, err
	}
	defer reader.Close()
	defer writer.Close()

	job := newOperation(action)
	cmd := exec.Command(a.Executable, "worker", action, job.ID)
	cmd.ExtraFiles = []*os.File{lock, writer}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Queued{}, err
	}

	writer.Close()
	// The child holds the same flock across exec and acknowledges only after it
	// has durably published its job. Merely forking is not an accepted request.
	_ = reader.SetReadDeadline(time.Now().Add(5 * time.Second))
	ack := make([]byte, 1)
	_, err = io.ReadFull(reader, ack)
	if err != nil || ack[0] != '1' {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Queued{}, fmt.Errorf("worker did not acknowledge startup: %w", err)
	}

	_ = cmd.Process.Release()
	return Queued{Queued: true, ID: job.ID}, nil
}

func (a *App) Worker(ctx context.Context, action, id string) error {
	if !queuedAction(action) || id == "" {
		return fmt.Errorf("invalid worker request")
	}

	lock := os.NewFile(3, "routeguard-lock")
	ready := os.NewFile(4, "routeguard-ready")
	if lock == nil || ready == nil {
		return fmt.Errorf("missing worker descriptors")
	}
	defer lock.Close()
	defer ready.Close()

	job := newOperation(action)
	job.ID = id
	if err := a.writeJob(job); err != nil {
		return err
	}

	if _, err := ready.Write([]byte("1")); err != nil {
		return err
	}
	ready.Close()

	_, err := a.executeJob(ctx, job)
	return err
}
