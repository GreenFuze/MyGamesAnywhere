package commandipc

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerForwardsAndCachesDuplicateRequest(t *testing.T) {
	t.Parallel()

	endpoint := testEndpoint(t)
	var calls atomic.Int32
	server, err := NewServer(endpoint, func(context.Context, Request) (Outcome, error) {
		calls.Add(1)
		return Outcome{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Serve(ctx) }()

	request := Request{Version: Version, RequestID: "same-request", URI: "mga://start?server=http%3A%2F%2Flocalhost%3A8900"}
	for attempt := 0; attempt < 2; attempt++ {
		if err := forwardWhenReady(endpoint, request); err != nil {
			t.Fatalf("Forward() attempt %d error = %v", attempt+1, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
	cancel()
	if err := <-serverResult; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServerSerializesConcurrentRequests(t *testing.T) {
	t.Parallel()

	endpoint := testEndpoint(t)
	var active atomic.Int32
	var maximum atomic.Int32
	server, err := NewServer(endpoint, func(context.Context, Request) (Outcome, error) {
		now := active.Add(1)
		for {
			previous := maximum.Load()
			if now <= previous || maximum.CompareAndSwap(previous, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return Outcome{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()
	waitForEndpoint(t, endpoint)

	var group sync.WaitGroup
	errorsChannel := make(chan error, 8)
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			request := Request{Version: Version, RequestID: "concurrent-" + string(rune('a'+index)), URI: "mga://start"}
			commandContext, commandCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer commandCancel()
			errorsChannel <- Forward(commandContext, endpoint, request)
		}(index)
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent Forward() error = %v", err)
		}
	}
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent handlers = %d, want 1", got)
	}
}

func TestServerRejectsUnsupportedAndOversizedFrames(t *testing.T) {
	t.Parallel()

	endpoint := testEndpoint(t)
	server, err := NewServer(endpoint, func(context.Context, Request) (Outcome, error) {
		t.Fatal("handler called for invalid request")
		return Outcome{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()
	waitForEndpoint(t, endpoint)

	connection := dialForTest(t, endpoint)
	if err := writeFrame(connection, Request{Version: Version + 1, RequestID: "bad-version", URI: "mga://start"}); err != nil {
		t.Fatalf("write unsupported request: %v", err)
	}
	var response Response
	if err := readFrame(connection, &response); err != nil {
		t.Fatalf("read unsupported response: %v", err)
	}
	_ = connection.Close()
	if response.OK || response.ErrorCode != "invalid_request" {
		t.Fatalf("unsupported response = %#v", response)
	}

	connection = dialForTest(t, endpoint)
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], MaxFrameBytes+1)
	if _, err := connection.Write(size[:]); err != nil {
		t.Fatalf("write oversized frame: %v", err)
	}
	response = Response{}
	if err := readFrame(connection, &response); err != nil {
		t.Fatalf("read oversized response: %v", err)
	}
	_ = connection.Close()
	if response.OK || response.ErrorCode != "invalid_request" {
		t.Fatalf("oversized response = %#v", response)
	}

	connection = dialForTest(t, endpoint)
	binary.BigEndian.PutUint32(size[:], 1)
	if _, err := connection.Write(append(size[:], '{')); err != nil {
		t.Fatalf("write malformed frame: %v", err)
	}
	response = Response{}
	if err := readFrame(connection, &response); err != nil {
		t.Fatalf("read malformed response: %v", err)
	}
	_ = connection.Close()
	if response.OK || response.ErrorCode != "invalid_request" {
		t.Fatalf("malformed response = %#v", response)
	}
}

func TestForwardReportsUnavailableAndAcknowledgementTimeout(t *testing.T) {
	t.Parallel()

	unavailable := testEndpoint(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Forward(ctx, unavailable, Request{Version: Version, RequestID: "unavailable", URI: "mga://start"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Forward() unavailable error = %v, want ErrUnavailable", err)
	}

	endpoint := testEndpoint(t)
	server, serverErr := NewServer(endpoint, func(context.Context, Request) (Outcome, error) {
		time.Sleep(300 * time.Millisecond)
		return Outcome{}, nil
	}, nil)
	if serverErr != nil {
		t.Fatalf("NewServer() error = %v", serverErr)
	}
	serverContext, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() { _ = server.Serve(serverContext) }()
	waitForEndpoint(t, endpoint)

	commandContext, stopCommand := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopCommand()
	err = Forward(commandContext, endpoint, Request{Version: Version, RequestID: "timeout", URI: "mga://start"})
	if err == nil {
		t.Fatal("Forward() timeout error = nil")
	}
}

func TestStopCallbackRunsOnceAfterAcknowledgementAttempt(t *testing.T) {
	t.Parallel()

	endpoint := testEndpoint(t)
	stopped := make(chan struct{}, 2)
	server, err := NewServer(endpoint, func(context.Context, Request) (Outcome, error) {
		return Outcome{StopPrimary: true}, nil
	}, func() { stopped <- struct{}{} })
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	request := Request{Version: Version, RequestID: "restart", URI: "mga://pair"}
	if err := forwardWhenReady(endpoint, request); err != nil {
		t.Fatalf("Forward() error = %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("restart callback was not called")
	}
	if err := Forward(context.Background(), endpoint, request); err != nil {
		t.Fatalf("duplicate Forward() error = %v", err)
	}
	select {
	case <-stopped:
		t.Fatal("restart callback called more than once")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStopForUpgradeActionIsTypedAndRejectsURI(t *testing.T) {
	t.Parallel()

	valid := Request{Version: Version, RequestID: "upgrade", Action: ActionStopForUpgrade}
	if err := validateRequest(valid); err != nil {
		t.Fatalf("validateRequest() error = %v", err)
	}
	for _, invalid := range []Request{
		{Version: Version, RequestID: "unknown", Action: "unknown"},
		{Version: Version, RequestID: "upgrade-uri", Action: ActionStopForUpgrade, URI: "mga://start"},
	} {
		if err := validateRequest(invalid); err == nil {
			t.Fatalf("validateRequest(%+v) error = nil", invalid)
		}
	}
}

func testEndpoint(t *testing.T) Endpoint {
	t.Helper()
	return Endpoint{Name: "MGAClient-Test-" + t.Name(), DataDir: t.TempDir()}
}

func waitForEndpoint(t *testing.T, endpoint Endpoint) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		connection, err := endpoint.Dial(ctx)
		cancel()
		if err == nil {
			_ = connection.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("endpoint did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func forwardWhenReady(endpoint Endpoint, request Request) error {
	deadline := time.Now().Add(3 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := Forward(ctx, endpoint, request)
		cancel()
		if err == nil || !errors.Is(err, ErrUnavailable) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func dialForTest(t *testing.T, endpoint Endpoint) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, err := endpoint.Dial(ctx)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	return connection
}
