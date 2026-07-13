package devices

import (
	"context"
	"errors"
	"sync"
)

type Transport interface {
	Write(ctx context.Context, data []byte) error
	Close() error
}

type Hub struct {
	mu          sync.RWMutex
	connections map[string]Transport
}

func NewHub() *Hub {
	return &Hub{connections: map[string]Transport{}}
}

func (h *Hub) Register(endpointID string, transport Transport) error {
	if endpointID == "" {
		return errors.New("endpoint_id is required")
	}
	if transport == nil {
		return errors.New("transport is required")
	}
	h.mu.Lock()
	previous := h.connections[endpointID]
	h.connections[endpointID] = transport
	h.mu.Unlock()
	if previous != nil && previous != transport {
		_ = previous.Close()
	}
	return nil
}

func (h *Hub) Unregister(endpointID string, transport Transport) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.connections[endpointID] != transport {
		return false
	}
	delete(h.connections, endpointID)
	return true
}

func (h *Hub) IsConnected(endpointID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connections[endpointID] != nil
}

func (h *Hub) Send(ctx context.Context, endpointID string, data []byte) error {
	h.mu.RLock()
	transport := h.connections[endpointID]
	h.mu.RUnlock()
	if transport == nil {
		return ErrEndpointOffline
	}
	if err := transport.Write(ctx, data); err != nil {
		return err
	}
	return nil
}

func (h *Hub) Disconnect(endpointID string) {
	h.mu.Lock()
	transport := h.connections[endpointID]
	delete(h.connections, endpointID)
	h.mu.Unlock()
	if transport != nil {
		_ = transport.Close()
	}
}
