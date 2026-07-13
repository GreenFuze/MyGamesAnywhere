package v1

import "testing"

func TestEndpointStateValidate(t *testing.T) {
	t.Parallel()

	valid := []EndpointState{
		EndpointReady,
		EndpointBusy,
		EndpointOffline,
		EndpointUpdateRequired,
		EndpointError,
	}
	for _, state := range valid {
		if err := state.Validate(); err != nil {
			t.Fatalf("EndpointState(%q).Validate() error = %v", state, err)
		}
	}
	if err := EndpointState("connected").Validate(); err == nil {
		t.Fatal("unknown EndpointState.Validate() error = nil, want error")
	}
}
