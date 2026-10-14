package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/benjamin2607/ambientlightcontrol/server/internal/controller"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/store"
)

type fakeController struct {
	lastLight controller.LightRequest
}

func (f *fakeController) Status() controller.Status { return controller.Status{} }
func (f *fakeController) SetLight(_ context.Context, r controller.LightRequest) error {
	f.lastLight = r
	return nil
}
func (f *fakeController) UpdateSettings(p controller.SettingsPatch) (store.Settings, error) {
	return store.Settings{}, p.Validate()
}
func (f *fakeController) ResumeAutomation()                    {}
func (f *fakeController) PairTV(context.Context, string) error { return nil }

const token = "test-token-0123456789abcdef"

func newServer() (*fakeController, http.Handler) {
	f := &fakeController{}
	return f, New(f, token, "test", slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
}

func request(h http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthzNeedsNoToken(t *testing.T) {
	_, h := newServer()
	if rec := request(h, "GET", "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestTokenRequired(t *testing.T) {
	_, h := newServer()
	for _, auth := range []string{"", "Bearer wrong", "Basic " + token, token} {
		if rec := request(h, "GET", "/api/v1/status", auth, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("auth %q: got %d", auth, rec.Code)
		}
	}
	rec := request(h, "GET", "/api/v1/status", "Bearer "+token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":"test"`) {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers missing")
	}
}

func TestRepeatedFailuresAreBlocked(t *testing.T) {
	_, h := newServer()
	for range maxFailures {
		request(h, "GET", "/api/v1/status", "Bearer wrong", "")
	}
	if rec := request(h, "GET", "/api/v1/status", "Bearer "+token, ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("got %d, want 429", rec.Code)
	}
}

func TestLightRequest(t *testing.T) {
	f, h := newServer()
	rec := request(h, "POST", "/api/v1/light", "Bearer "+token, `{"color":255,"brightness":40}`)
	if rec.Code != http.StatusOK || *f.lastLight.Color != 255 || *f.lastLight.Brightness != 40 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"brightness":500}`, `{"unknown":1}`, `not json`, `{}`} {
		if rec := request(h, "POST", "/api/v1/light", "Bearer "+token, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d", body, rec.Code)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	_, h := newServer()
	big := `{"tvHost":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	if rec := request(h, "PATCH", "/api/v1/settings", "Bearer "+token, big); rec.Code != http.StatusBadRequest {
		t.Errorf("got %d", rec.Code)
	}
}

func TestSettingsValidation(t *testing.T) {
	_, h := newServer()
	if rec := request(h, "PATCH", "/api/v1/settings", "Bearer "+token, `{"onTime":"25:00"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("got %d", rec.Code)
	}
	if rec := request(h, "PATCH", "/api/v1/settings", "Bearer "+token, `{"onTime":"06:30"}`); rec.Code != http.StatusOK {
		t.Errorf("got %d", rec.Code)
	}
}
