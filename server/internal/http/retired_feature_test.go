package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type readCountingBody struct{ reads int }

func (b *readCountingBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }
func (*readCountingBody) Close() error               { return nil }

func TestRetiredFeatureHandlerReturnsTypedGoneWithoutReadingMutationBody(t *testing.T) {
	body := &readCountingBody{}
	request := httptest.NewRequest(http.MethodPost, "/api/devices/old/games/game/install-archive", nil)
	request.Body = body
	recorder := httptest.NewRecorder()
	RetiredFeatureHandler("")(recorder, request)
	if recorder.Code != http.StatusGone || !strings.Contains(recorder.Body.String(), `"code":"feature_retired"`) || !strings.Contains(recorder.Body.String(), "catalog/content APIs") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if body.reads != 0 {
		t.Fatalf("retirement handler read mutation body %d times", body.reads)
	}
}

func TestNoDatabaseRouterReturnsRetirementResponseForLegacyClientRoutes(t *testing.T) {
	router := BuildRouter(nil, 0, "")
	for _, item := range []struct{ method, path, code string }{
		{http.MethodGet, "/api/devices/connect", "mga_client_retired"},
		{http.MethodPost, "/api/devices/id/games/game/sources/source/repair", "feature_retired"},
		{http.MethodHead, "/api/games/game/play", "feature_retired"},
	} {
		request := httptest.NewRequest(item.method, item.path, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusGone || (item.method != http.MethodHead && !strings.Contains(recorder.Body.String(), item.code)) {
			t.Fatalf("%s %s = %d %s", item.method, item.path, recorder.Code, recorder.Body.String())
		}
	}
}
