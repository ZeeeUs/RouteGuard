package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ZeeeUs/RouteGuard/internal/store"
)

var ErrBusy = errors.New("RouteGuard is busy")

var operationID = regexp.MustCompile(`^[a-z0-9]+-[0-9]+$`)

type Operation struct {
	Diagnostics *DiagnosticReport `json:"diagnostics,omitempty"`
	ID          string            `json:"id,omitempty"`
	PID         int               `json:"pid,omitempty"`
	Action      string            `json:"action,omitempty"`
	State       string            `json:"state,omitempty"`
	Started     string            `json:"started,omitempty"`
	Time        string            `json:"time,omitempty"`
	Message     string            `json:"message,omitempty"`
}

func (a *App) lock(wait bool) (*os.File, error) {
	if err := os.MkdirAll(a.RunPath, 0700); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filepath.Join(a.RunPath, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	flags := syscall.LOCK_EX
	if !wait {
		flags |= syscall.LOCK_NB
	}

	if err := syscall.Flock(int(file.Fd()), flags); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}

		return nil, err
	}

	return file, nil
}

func (a *App) Execute(ctx context.Context, action string) (string, error) {
	lock, err := a.lock(action == "restore" || action == "stop" || action == "recover")
	if err != nil {
		return "", err
	}
	defer lock.Close()

	job := newOperation(action)
	if err := a.writeJob(job); err != nil {
		return "", err
	}

	return a.executeJob(ctx, job)
}

func newOperation(action string) Operation {
	now := time.Now().UTC()
	return Operation{
		ID:  strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.Itoa(os.Getpid()),
		PID: os.Getpid(), Action: action, State: "running", Started: now.Format(time.RFC3339),
	}
}

func (a *App) writeJob(job Operation) error {
	if !operationID.MatchString(job.ID) {
		return fmt.Errorf("invalid operation ID")
	}

	directory := filepath.Join(a.RunPath, "jobs")
	if err := store.WriteJSON(filepath.Join(directory, job.ID+".json"), job); err != nil {
		return err
	}

	if err := store.WriteJSON(filepath.Join(a.RunPath, "job.json"), job); err != nil {
		return err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(b.Name(), a.Name()) })
	for _, entry := range entries[min(16, len(entries)):] {
		_ = os.Remove(filepath.Join(directory, entry.Name()))
	}

	return nil
}

func (a *App) Job(id string) (Operation, error) {
	if !operationID.MatchString(id) {
		return Operation{}, fmt.Errorf("invalid operation ID")
	}

	var job Operation
	exists, err := store.ReadJSON(filepath.Join(a.RunPath, "jobs", id+".json"), &job)
	if err != nil || !exists {
		return job, fmt.Errorf("operation result is unavailable")
	}

	current, busy, err := a.jobStatus()
	if err != nil {
		return job, err
	}

	if job.State == "running" && (!busy || current.ID != id) {
		job.State, job.Message = "interrupted", "Operation was interrupted; retry to recover"
	}

	return job, nil
}

func (a *App) executeJob(ctx context.Context, job Operation) (string, error) {
	var message string
	var operationErr error
	if job.Action == "diagnose" {
		job.Diagnostics, operationErr = a.diagnose(ctx)
		message = "Profile checks completed; review individual results"
	} else {
		message, operationErr = a.perform(ctx, job.Action)
	}
	job.State = "ok"
	if operationErr != nil {
		job.State, message = "error", operationErr.Error()
	}

	job.Message = strings.Join(strings.Fields(message), " ")
	if len(job.Message) > 800 {
		job.Message = job.Message[:800]
	}

	job.Time = time.Now().UTC().Format(time.RFC3339)
	jobErr := a.writeJob(job)
	a.System.Log(context.WithoutCancel(ctx), job.Action+": "+job.Message)

	return message, errors.Join(operationErr, jobErr)
}

func (a *App) jobStatus() (Operation, bool, error) {
	// The inherited flock is authoritative; PID reuse cannot keep a stale job busy.
	lock, lockErr := a.lock(false)
	if lock != nil {
		defer lock.Close()
	}

	if lockErr != nil && !errors.Is(lockErr, ErrBusy) {
		return Operation{}, false, lockErr
	}

	var job Operation
	_, err := store.ReadJSON(filepath.Join(a.RunPath, "job.json"), &job)
	if err != nil {
		return job, false, err
	}

	busy := errors.Is(lockErr, ErrBusy)
	if job.State == "running" && !busy {
		job.State = "interrupted"
		job.Message = fmt.Sprintf("%s was interrupted; retry to recover", job.Action)
	}

	return job, busy, nil
}
