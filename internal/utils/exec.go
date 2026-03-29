package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type CommandResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func RunWithDir(timeout time.Duration, dir string, name string, args ...string) (*CommandResult, error) {
	if name == "sudo" && os.Geteuid() == 0 {
		if len(args) == 0 {
			return &CommandResult{}, fmt.Errorf("run sudo []: missing command")
		}
		name = args[0]
		args = args[1:]
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	res := &CommandResult{Stdout: out.String(), Stderr: errBuf.String()}
	if err != nil {
		return res, fmt.Errorf("run %s %v: %w", name, args, err)
	}
	return res, nil
}

func Run(timeout time.Duration, name string, args ...string) (*CommandResult, error) {
	return RunWithDir(timeout, "", name, args...)
}

func RunShell(timeout time.Duration, command string) (*CommandResult, error) {
	return Run(timeout, "/bin/sh", "-lc", command)
}
