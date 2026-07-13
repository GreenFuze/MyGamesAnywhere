package v1

import "fmt"

// EndpointState is the server-derived state displayed by the web interface.
type EndpointState string

const (
	EndpointReady          EndpointState = "ready"
	EndpointBusy           EndpointState = "busy"
	EndpointOffline        EndpointState = "offline"
	EndpointUpdateRequired EndpointState = "update_required"
	EndpointError          EndpointState = "error"
)

// Validate rejects unknown endpoint states.
func (s EndpointState) Validate() error {
	switch s {
	case EndpointReady, EndpointBusy, EndpointOffline, EndpointUpdateRequired, EndpointError:
		return nil
	default:
		return fmt.Errorf("unknown endpoint state %q", s)
	}
}
