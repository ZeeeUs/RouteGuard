// Package platform contains the OpenWrt command and filesystem adapters.
package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Run(context.Context, string, string, ...string) ([]byte, error)
}

type Exec struct{}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.Len() {
		return 0, fmt.Errorf("command output exceeds %d bytes", b.limit)
	}

	return b.Buffer.Write(data)
}

func (Exec) Run(parent context.Context, input, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	stdout := &boundedBuffer{limit: 4 << 20}
	stderr := &boundedBuffer{limit: 32 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s: %w", name, ctx.Err())
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w: %.500s", name, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func missingObject(err error) bool {
	return err != nil && strings.Contains(err.Error(), "No such file or directory") && !errors.Is(err, exec.ErrNotFound)
}
