package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benjamin2607/ambientlightcontrol/server/internal/govee"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/store"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/webos"
)

type fakeLight struct {
	mu           sync.Mutex
	calls        []string
	devicesCalls int
}

var testDevice = govee.Device{
	SKU: "H6099", ID: "AA:BB", Name: "TV Backlight",
	Capabilities: []govee.Capability{snapshotCapability("Video", "Evening")},
}

func snapshotCapability(names ...string) govee.Capability {
	c := govee.Capability{Type: govee.TypeDynamicScene, Instance: govee.InstanceSnapshot}
	for i, n := range names {
		c.Parameters.Options = append(c.Parameters.Options, govee.Option{Name: n, Value: json.RawMessage(fmt.Sprint(100 + i))})
	}
	return c
}

func (f *fakeLight) record(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// take returns and clears the recorded calls.
func (f *fakeLight) take() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := strings.Join(f.calls, " ")
	f.calls = nil
	return s
}

func (f *fakeLight) Devices(context.Context) ([]govee.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devicesCalls++
	return []govee.Device{testDevice}, nil
}

func (f *fakeLight) deviceCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.devicesCalls
}
func (f *fakeLight) DIYScenes(context.Context, govee.Device) ([]govee.Option, error) {
	return nil, nil
}
func (f *fakeLight) State(context.Context, govee.Device) (govee.State, error) {
	return govee.State{Online: true}, nil
}
func (f *fakeLight) SetPower(_ context.Context, _ govee.Device, on bool) error {
	f.record("power=%v", on)
	return nil
}
func (f *fakeLight) SetBrightness(_ context.Context, _ govee.Device, p int) error {
	f.record("brightness=%d", p)
	return nil
}
func (f *fakeLight) SetColor(_ context.Context, _ govee.Device, rgb int) error {
	f.record("color=%06X", rgb)
	return nil
}
func (f *fakeLight) Control(_ context.Context, _ govee.Device, _, instance string, value any) error {
	f.record("%s=%s", instance, value)
	return nil
}

type harness struct {
	t     *testing.T
	ctx   context.Context
	c     *Controller
	light *fakeLight
	now   time.Time
	tv    func(string, error)
}

func newHarness(t *testing.T, hour int) *harness {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateSettings(func(s *store.Settings) {
		s.TVHost = "192.0.2.10"
		s.Color = 0x112233
		s.Brightness = 50
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTV(webos.Credentials{ClientKey: "key", CertSHA256: "pin"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	h := &harness{t: t, ctx: ctx, light: &fakeLight{}, now: time.Date(2026, 10, 14, hour, 0, 0, 0, time.UTC)}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	started := make(chan func(string, error), 1)
	watch := func(_ context.Context, _ string, _ webos.Credentials, onState func(string, error)) {
		started <- onState
	}
	h.c = New(log, h.light, st, watch, "")
	h.c.now = func() time.Time { return h.now }
	h.c.runCtx = ctx
	h.c.restartTVWatch()
	h.tv = <-started
	h.c.loadDevice(ctx)
	h.c.refreshSources(ctx)
	return h
}

func (h *harness) step(d time.Duration) string {
	h.now = h.now.Add(d)
	h.c.evaluate(h.ctx)
	return h.light.take()
}

func (h *harness) expect(got, want string) {
	h.t.Helper()
	if got != want {
		h.t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestDaytimeShowsFixedColorOnce(t *testing.T) {
	h := newHarness(t, 7)
	h.expect(h.step(0), "power=true color=112233 brightness=50")
	h.expect(h.step(time.Minute), "")
}

func TestVideoModeFollowsTheTV(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)

	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=100 brightness=100")

	// Short interruptions (input switch, restart) don't flip the light.
	h.tv(webos.StateStandby, nil)
	h.expect(h.step(10*time.Second), "")
	h.expect(h.step(15*time.Second), "power=true color=112233 brightness=50")
}

func TestNightRules(t *testing.T) {
	h := newHarness(t, 21)
	h.step(0)
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=100 brightness=100")

	// 22:00 passes while the TV runs: keep following the picture.
	h.expect(h.step(time.Hour), "")

	// TV off at night: light off.
	h.tv(webos.StateOff, nil)
	h.expect(h.step(time.Minute), "power=false")

	// Next morning.
	h.now = time.Date(2026, 10, 15, 6, 59, 0, 0, time.UTC)
	h.expect(h.step(0), "")
	h.expect(h.step(time.Minute), "power=true color=112233 brightness=50")
}

func TestManualOffPausesUntilNextEvent(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)

	off := false
	if err := h.c.SetLight(h.ctx, LightRequest{Power: &off}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.light.take(), "power=false")
	h.expect(h.step(time.Hour), "")
	if !h.c.Status().Automation.Overridden {
		t.Error("expected override")
	}

	// The TV turning on is a new event: automation takes over again.
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=100 brightness=100")
	if h.c.Status().Automation.Overridden {
		t.Error("override should have ended")
	}
}

func TestColorChangeBecomesDaytimeColor(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)

	color, brightness := 0xFF0000, 80
	if err := h.c.SetLight(h.ctx, LightRequest{Color: &color, Brightness: &brightness}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.light.take(), "power=true color=FF0000 brightness=80")
	st := h.c.Status()
	if st.Automation.Overridden || st.Settings.Color != 0xFF0000 || st.Settings.Brightness != 80 {
		t.Errorf("unexpected status %+v", st)
	}
	h.expect(h.step(time.Minute), "")
}

func TestDimmingDaytimeColorIsOneRequest(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	if err := h.c.SetLight(h.ctx, LightRequest{Brightness: ptr(30)}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.light.take(), "brightness=30")
	if st := h.c.Status(); st.Settings.Brightness != 30 || st.Automation.Overridden {
		t.Errorf("unexpected status %+v", st)
	}
}

func TestVideoBrightnessIsRemembered(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=100 brightness=100")

	if err := h.c.SetLight(h.ctx, LightRequest{Brightness: ptr(80)}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.light.take(), "brightness=80")
	st := h.c.Status()
	if st.Settings.VideoBrightness != 80 || st.Settings.Brightness != 50 || st.Automation.Overridden {
		t.Errorf("unexpected settings %+v", st.Settings)
	}

	// Next time the TV turns on, video uses the remembered level.
	h.tv(webos.StateOff, nil)
	h.expect(h.step(time.Minute), "power=true color=112233 brightness=50")
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=100 brightness=80")
}

func TestResumeReappliesTarget(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	mode := "video"
	if err := h.c.SetLight(h.ctx, LightRequest{Mode: &mode}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.light.take(), "power=true snapshot=100 brightness=100")
	h.c.ResumeAutomation()
	h.expect(h.step(0), "power=true color=112233 brightness=50")
}

func TestSettingsChangeAppliesNewColor(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	color := 0x00FF00
	if _, err := h.c.UpdateSettings(SettingsPatch{Color: &color}); err != nil {
		t.Fatal(err)
	}
	h.expect(h.step(0), "power=true color=00FF00 brightness=50")
}

func TestVideoSourceByName(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	if _, err := h.c.UpdateSettings(SettingsPatch{VideoSource: &store.SceneRef{Kind: "snapshot", Name: "Evening"}}); err != nil {
		t.Fatal(err)
	}
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "power=true snapshot=101 brightness=100")
}

func TestMissingVideoSourceIsReported(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	if _, err := h.c.UpdateSettings(SettingsPatch{VideoSource: &store.SceneRef{Kind: "snapshot", Name: "Gone"}}); err != nil {
		t.Fatal(err)
	}
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Second), "")
	if !strings.Contains(h.c.Status().Automation.Error, "Gone") {
		t.Errorf("expected error, got %q", h.c.Status().Automation.Error)
	}
}

func TestMissingSceneDoesNotHammerTheAPI(t *testing.T) {
	h := newHarness(t, 12)
	h.step(0)
	if _, err := h.c.UpdateSettings(SettingsPatch{VideoSource: &store.SceneRef{Kind: "snapshot", Name: "Gone"}}); err != nil {
		t.Fatal(err)
	}
	h.tv(webos.StateOn, nil)
	before := h.light.deviceCalls()
	for range 10 { // ten retries, 30 s apart = 5 minutes
		h.step(30 * time.Second)
	}
	if got := h.light.deviceCalls() - before; got > 2 {
		t.Errorf("scene list reloaded %d times in 5 minutes", got)
	}
}

func TestDisabledAutomationDoesNothing(t *testing.T) {
	h := newHarness(t, 12)
	disabled := false
	if _, err := h.c.UpdateSettings(SettingsPatch{AutomationEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	h.tv(webos.StateOn, nil)
	h.expect(h.step(time.Minute), "")
}

func TestValidation(t *testing.T) {
	bad := []LightRequest{
		{},
		{Brightness: ptr(0)},
		{Brightness: ptr(101)},
		{Color: ptr(0x1000000)},
		{Mode: ptr("party")},
	}
	for _, r := range bad {
		if r.Validate() == nil {
			t.Errorf("%+v should be invalid", r)
		}
	}
	badPatches := []SettingsPatch{
		{OnTime: ptr("7")},
		{TVHost: ptr("http://tv/")},
		{VideoSource: &store.SceneRef{Kind: "x", Name: "y"}},
	}
	for _, p := range badPatches {
		if p.Validate() == nil {
			t.Errorf("%+v should be invalid", p)
		}
	}
}

func ptr[T any](v T) *T { return &v }
