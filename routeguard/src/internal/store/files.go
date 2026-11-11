// Package store owns bounded, versioned and durable state on disk.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxJSONBytes = 2 << 20

func ReadJSON(path string, dest any) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := &io.LimitedReader{R: file, N: maxJSONBytes + 1}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(dest); err != nil {
		return true, err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || reader.N == 0 {
		return true, fmt.Errorf("oversized or trailing state data")
	}

	return true, nil
}

func WriteJSON(path string, value any) error {
	return AtomicWrite(path, func(file *os.File) error {
		return json.NewEncoder(file).Encode(value)
	})
}

func AtomicWrite(path string, write func(*os.File) error) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}

	file, err := os.CreateTemp(directory, ".routeguard-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	err = write(file)
	if err == nil {
		err = file.Sync()
	}

	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}

	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}

	return syncDirectory(directory)
}

func Remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()

	return directory.Sync()
}
