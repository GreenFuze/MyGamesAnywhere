package plugins

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type Request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type IpcClient interface {
	Call(ctx context.Context, method string, params any, result any) error
	Close() error
}

type jsonIpcClient struct {
	process Process
	mu      sync.Mutex
	pending map[string]chan *Response
	logger  core.Logger
	pluginID string
}

func NewIpcClient(process Process, logger core.Logger, pluginID string) IpcClient {
	c := &jsonIpcClient{
		process: process,
		pending: make(map[string]chan *Response),
		logger:  logger,
		pluginID: pluginID,
	}
	go c.listenStdout()
	go c.listenStderr()
	return c
}

func (c *jsonIpcClient) listenStdout() {
	stdout := c.process.Stdout()
	for {
		var length uint32
		err := binary.Read(stdout, binary.BigEndian, &length)
		if err != nil {
			if err != io.EOF {
				c.logger.Error("failed to read from plugin stdout", err, "plugin_id", c.pluginID)
			}
			return
		}

		payload := make([]byte, length)
		_, err = io.ReadFull(stdout, payload)
		if err != nil {
			c.logger.Error("failed to read payload from plugin stdout", err, "plugin_id", c.pluginID)
			return
		}

		var resp Response
		if err := json.Unmarshal(payload, &resp); err != nil {
			c.logger.Error("failed to unmarshal plugin response", err, "plugin_id", c.pluginID)
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			ch <- &resp
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
	}
}

func (c *jsonIpcClient) listenStderr() {
	scanner := bufio.NewScanner(c.process.Stderr())
	for scanner.Scan() {
		c.logger.Info(scanner.Text(), "plugin_id", c.pluginID, "source", "stderr")
	}
	if err := scanner.Err(); err != nil {
		c.logger.Error("failed to read from plugin stderr", err, "plugin_id", c.pluginID)
	}
}

func (c *jsonIpcClient) Call(ctx context.Context, method string, params any, result any) error {
	id := uuid.New().String()

	req := Request{
		ID:     id,
		Method: method,
		Params: params,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	c.mu.Lock()
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	// Write length-prefixed payload
	c.mu.Lock()
	err = binary.Write(c.process.Stdin(), binary.BigEndian, uint32(len(payload)))
	if err == nil {
		_, err = c.process.Stdin().Write(payload)
	}
	c.mu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("plugin error [%s]: %s", resp.Error.Code, resp.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	}
}

func (c *jsonIpcClient) Close() error {
	return c.process.Kill()
}
