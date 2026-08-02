package commandipc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	Version       = 1
	MaxFrameBytes = 16 * 1024
)

var ErrUnavailable = errors.New("MGA Client command channel is unavailable")

type Request struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	URI       string `json:"uri"`
}

type Response struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Outcome struct {
	RestartPrimary bool
}

type Handler func(context.Context, string) (Outcome, error)

type Server struct {
	endpoint Endpoint
	handler  Handler
	after    func()

	mu         sync.Mutex
	responses  map[string]Response
	restarting bool
	restart    sync.Once
}

func NewServer(endpoint Endpoint, handler Handler, afterRestartAcknowledged func()) (*Server, error) {
	if err := endpoint.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, errors.New("command handler is required")
	}
	return &Server{
		endpoint:  endpoint,
		handler:   handler,
		after:     afterRestartAcknowledged,
		responses: make(map[string]Response),
	}, nil
}

func (s *Server) Serve(ctx context.Context) error {
	if s == nil {
		return errors.New("command server is required")
	}
	listener, err := s.endpoint.Listen()
	if err != nil {
		return fmt.Errorf("listen for MGA Client commands: %w", err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var clients sync.WaitGroup
	defer clients.Wait()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept MGA Client command: %w", acceptErr)
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			s.handleConnection(ctx, connection)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	var request Request
	if err := readFrame(connection, &request); err != nil {
		_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = writeFrame(connection, Response{
			Version:   Version,
			RequestID: request.RequestID,
			OK:        false,
			ErrorCode: "invalid_request",
			Error:     err.Error(),
		})
		return
	}
	_ = connection.SetReadDeadline(time.Time{})

	response, restart := s.execute(ctx, request)
	_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = writeFrame(connection, response)
	if restart && s.after != nil {
		s.restart.Do(s.after)
	}
}

func (s *Server) execute(ctx context.Context, request Request) (Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cached, found := s.responses[request.RequestID]; found {
		return cached, false
	}
	response := Response{Version: Version, RequestID: request.RequestID}
	if err := validateRequest(request); err != nil {
		response.ErrorCode = "invalid_request"
		response.Error = err.Error()
		s.remember(request.RequestID, response)
		return response, false
	}
	if s.restarting {
		response.ErrorCode = "client_restarting"
		response.Error = "MGA Client is applying another request and restarting; try again in a moment"
		s.remember(request.RequestID, response)
		return response, false
	}

	outcome, err := s.handler(ctx, request.URI)
	if err != nil {
		response.ErrorCode = "command_failed"
		response.Error = err.Error()
		s.remember(request.RequestID, response)
		return response, false
	}
	response.OK = true
	s.remember(request.RequestID, response)
	if outcome.RestartPrimary {
		s.restarting = true
		return response, true
	}
	return response, false
}

func (s *Server) remember(requestID string, response Response) {
	if len(s.responses) >= 128 {
		clear(s.responses)
	}
	s.responses[requestID] = response
}

func Forward(ctx context.Context, endpoint Endpoint, request Request) error {
	return ForwardWithDialTimeout(ctx, ctx, endpoint, request)
}

func ForwardWithDialTimeout(ctx context.Context, dialContext context.Context, endpoint Endpoint, request Request) error {
	if err := endpoint.Validate(); err != nil {
		return err
	}
	if err := validateRequest(request); err != nil {
		return err
	}
	connection, err := endpoint.Dial(dialContext)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	if err := writeFrame(connection, request); err != nil {
		return fmt.Errorf("send command to the running MGA Client: %w", err)
	}
	var response Response
	if err := readFrame(connection, &response); err != nil {
		return fmt.Errorf("wait for the running MGA Client acknowledgement: %w", err)
	}
	if response.Version != Version {
		return fmt.Errorf("running MGA Client returned unsupported command protocol version %d", response.Version)
	}
	if response.RequestID != request.RequestID {
		return errors.New("running MGA Client returned a mismatched command acknowledgement")
	}
	if !response.OK {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = "the running MGA Client rejected the command"
		}
		return errors.New(message)
	}
	return nil
}

func validateRequest(request Request) error {
	if request.Version != Version {
		return fmt.Errorf("unsupported MGA Client command protocol version %d", request.Version)
	}
	request.RequestID = strings.TrimSpace(request.RequestID)
	if request.RequestID == "" || len(request.RequestID) > 128 {
		return errors.New("command request ID is required and must be at most 128 characters")
	}
	request.URI = strings.TrimSpace(request.URI)
	if request.URI == "" {
		return errors.New("MGA protocol URI is required")
	}
	if len(request.URI) > MaxFrameBytes-512 {
		return errors.New("MGA protocol URI is too large")
	}
	return nil
}

func writeFrame(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > MaxFrameBytes {
		return fmt.Errorf("command frame size %d is outside the allowed range", len(data))
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := writer.Write(size[:]); err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func readFrame(reader io.Reader, value any) error {
	var size [4]byte
	if _, err := io.ReadFull(reader, size[:]); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > MaxFrameBytes {
		return fmt.Errorf("command frame size %d is outside the allowed range", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode command frame: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("command frame contains trailing JSON")
	}
	return nil
}
