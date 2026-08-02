package commandipc

import (
	"context"
	"errors"
	"net"
	"strings"
)

type Endpoint struct {
	Name    string
	DataDir string
}

func (e Endpoint) Validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("command endpoint name is required")
	}
	if strings.TrimSpace(e.DataDir) == "" {
		return errors.New("command endpoint data directory is required")
	}
	return nil
}

func (e Endpoint) Listen() (net.Listener, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return listen(e)
}

func (e Endpoint) Dial(ctx context.Context) (net.Conn, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return dial(ctx, e)
}
