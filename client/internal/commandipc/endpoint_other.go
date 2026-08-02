//go:build !windows

package commandipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
)

func listen(endpoint Endpoint) (net.Listener, error) {
	path := socketPath(endpoint)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &unixListener{Listener: listener, path: path}, nil
}

func dial(ctx context.Context, endpoint Endpoint) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", socketPath(endpoint))
}

func socketPath(endpoint Endpoint) string {
	sum := sha256.Sum256([]byte(endpoint.Name))
	return filepath.Join(endpoint.DataDir, ".mga-command-"+hex.EncodeToString(sum[:8])+".sock")
}

type unixListener struct {
	net.Listener
	path string
}

func (l *unixListener) Close() error {
	err := l.Listener.Close()
	removeErr := os.Remove(l.path)
	if err != nil {
		return err
	}
	if removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return nil
}
