//go:build windows

package commandipc

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func listen(endpoint Endpoint) (net.Listener, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("resolve current Windows user for command channel: %w", err)
	}
	sid := tokenUser.User.Sid.String()
	securityDescriptor := "D:P(A;;GA;;;SY)(A;;GA;;;" + sid + ")S:(ML;;NW;;;ME)"
	return winio.ListenPipe(pipePath(endpoint), &winio.PipeConfig{
		SecurityDescriptor: securityDescriptor,
		InputBufferSize:    MaxFrameBytes + 4,
		OutputBufferSize:   MaxFrameBytes + 4,
	})
}

func dial(ctx context.Context, endpoint Endpoint) (net.Conn, error) {
	return winio.DialPipeContext(ctx, pipePath(endpoint))
}

func pipePath(endpoint Endpoint) string {
	name := strings.NewReplacer("\\", "-", "/", "-", ":", "-").Replace(endpoint.Name)
	return `\\.\pipe\` + name + "-Command-v1"
}
