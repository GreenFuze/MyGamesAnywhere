package plugins

import (
	"context"
	"io"
	"os/exec"
)

type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait() error
	Kill() error
}

type ProcessManager interface {
	Spawn(ctx context.Context, command string, args []string, dir string) (Process, error)
}

type osProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p *osProcess) Stdin() io.WriteCloser {
	return p.stdin
}

func (p *osProcess) Stdout() io.ReadCloser {
	return p.stdout
}

func (p *osProcess) Stderr() io.ReadCloser {
	return p.stderr
}

func (p *osProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *osProcess) Kill() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

type osProcessManager struct{}

func NewProcessManager() ProcessManager {
	return &osProcessManager{}
}

func (m *osProcessManager) Spawn(ctx context.Context, command string, args []string, dir string) (Process, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &osProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}
