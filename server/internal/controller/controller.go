// Package controller ties the Govee light, the TV watcher, the automation rules and the settings together.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/benjamin2607/ambientlightcontrol/server/internal/automation"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/govee"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/store"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/webos"
)

// Light is the subset of the Govee client the controller needs (a fake in tests).
type Light interface {
	Devices(ctx context.Context) ([]govee.Device, error)
	DIYScenes(ctx context.Context, d govee.Device) ([]govee.Option, error)
	State(ctx context.Context, d govee.Device) (govee.State, error)
	SetPower(ctx context.Context, d govee.Device, on bool) error
	SetBrightness(ctx context.Context, d govee.Device, percent int) error
	SetColor(ctx context.Context, d govee.Device, rgb int) error
	Control(ctx context.Context, d govee.Device, capType, instance string, value any) error
}

// TVWatcher starts watching the TV; tests replace it.
type TVWatcher func(ctx context.Context, host string, creds webos.Credentials, onState func(string, error))

const (
	tickInterval       = 5 * time.Second
	lightPollInterval  = 60 * time.Second
	sourcesInterval    = 30 * time.Minute
	sourcesRetry       = 5 * time.Minute // on-demand reload when a video scene is missing
	retryAfterFailure  = 30 * time.Second
	tvOffGracePeriod   = 20 * time.Second // input switches / restarts should not flip the light
	appliedManual      = automation.Target("manual")
	modeUnknown        = "unknown"
	sourceKindSnapshot = "snapshot"
	sourceKindDIY      = "diyScene"
)

var ErrNotReady = errors.New("light not found yet")

type Controller struct {
	log      *slog.Logger
	light    Light
	store    *store.Store
	watchTV  TVWatcher
	deviceID string // optional device id or name from the environment
	now      func() time.Time

	cmdMu sync.Mutex // one Govee command sequence at a time

	mu            sync.Mutex
	device        *govee.Device
	state         govee.State
	stateKnown    bool
	mode          string // last mode we set: off, color, video or unknown
	lightErr      string
	sources       []source
	sourcesLoaded time.Time
	tvState       string
	tvErr         string
	tvLastOn      time.Time
	override      *automation.Context
	applied       automation.Target
	lastFailure   time.Time
	automationErr string
	tvCancel      context.CancelFunc
	runCtx        context.Context
	wake          chan struct{}
}

type source struct {
	kind  string
	name  string
	value json.RawMessage
}

func New(log *slog.Logger, light Light, st *store.Store, watchTV TVWatcher, deviceID string) *Controller {
	return &Controller{
		log:      log,
		light:    light,
		store:    st,
		watchTV:  watchTV,
		deviceID: deviceID,
		now:      time.Now,
		mode:     modeUnknown,
		tvState:  webos.StateUnpaired,
		wake:     make(chan struct{}, 1),
	}
}

// Run blocks until ctx is cancelled.
func (c *Controller) Run(ctx context.Context) {
	c.mu.Lock()
	c.runCtx = ctx
	c.mu.Unlock()
	c.restartTVWatch()

	var lastPoll, lastSources time.Time
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		now := c.now()
		if c.currentDevice() == nil {
			c.loadDevice(ctx)
		}
		if c.currentDevice() != nil {
			if now.Sub(lastSources) >= sourcesInterval {
				c.refreshSources(ctx)
				lastSources = now
			}
			if now.Sub(lastPoll) >= lightPollInterval {
				c.refreshLight(ctx)
				lastPoll = now
			}
			c.evaluate(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.wake:
		}
	}
}

func (c *Controller) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// MARK: Light

func (c *Controller) currentDevice() *govee.Device {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.device
}

func (c *Controller) loadDevice(ctx context.Context) {
	devices, err := c.light.Devices(ctx)
	if err != nil {
		c.setLightErr(err)
		return
	}
	var chosen *govee.Device
	for i, d := range devices {
		if c.deviceID == "" || strings.EqualFold(d.ID, c.deviceID) || strings.EqualFold(d.Name, c.deviceID) {
			chosen = &devices[i]
			break
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if chosen == nil {
		c.lightErr = "No matching Govee device found in your account."
		return
	}
	c.device = chosen
	c.lightErr = ""
	c.log.Info("using Govee device", "name", chosen.Name, "sku", chosen.SKU)
}

func (c *Controller) refreshLight(ctx context.Context) {
	d := c.currentDevice()
	s, err := c.light.State(ctx, *d)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lightErr = userMessage(err)
		return
	}
	c.state, c.stateKnown, c.lightErr = s, true, ""
	if !s.Power {
		c.mode = "off"
	} else if c.mode == "off" {
		c.mode = modeUnknown // switched on elsewhere
	}
}

// refreshSources reloads the snapshots and DIY scenes that can serve as "video" mode.
func (c *Controller) refreshSources(ctx context.Context) {
	devices, err := c.light.Devices(ctx)
	if err != nil {
		c.setLightErr(err)
		return
	}
	d := c.currentDevice()
	var list []source
	for _, dev := range devices {
		if dev.ID != d.ID {
			continue
		}
		for _, o := range dev.Options(govee.InstanceSnapshot) {
			list = append(list, source{sourceKindSnapshot, o.Name, o.Value})
		}
	}
	if diy, err := c.light.DIYScenes(ctx, *d); err == nil {
		for _, o := range diy {
			list = append(list, source{sourceKindDIY, o.Name, o.Value})
		}
	}
	c.mu.Lock()
	c.sources = list
	c.sourcesLoaded = c.now()
	c.mu.Unlock()

	// First run: adopt a snapshot or scene called "Video" automatically.
	if c.store.Settings().VideoSource == nil {
		for _, s := range list {
			if strings.EqualFold(strings.TrimSpace(s.name), "video") {
				ref := &store.SceneRef{Kind: s.kind, Name: s.name}
				if _, err := c.store.UpdateSettings(func(st *store.Settings) { st.VideoSource = ref }); err == nil {
					c.log.Info("video source selected automatically", "kind", s.kind, "name", s.name)
				}
				break
			}
		}
	}
}

func (c *Controller) setLightErr(err error) {
	c.mu.Lock()
	c.lightErr = userMessage(err)
	c.mu.Unlock()
	c.log.Warn("govee request failed", "error", err)
}

// MARK: TV

func (c *Controller) restartTVWatch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tvCancel != nil {
		c.tvCancel()
		c.tvCancel = nil
	}
	host := c.store.Settings().TVHost
	creds := c.store.TV()
	switch {
	case c.runCtx == nil:
		return
	case host == "" || creds.ClientKey == "":
		c.tvState, c.tvErr = webos.StateUnpaired, ""
		return
	}
	ctx, cancel := context.WithCancel(c.runCtx)
	c.tvCancel = cancel
	c.tvState = webos.StateOff
	go c.watchTV(ctx, host, creds, func(state string, err error) {
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.log.Debug("TV connection", "state", state, "error", err)
		}
		c.mu.Lock()
		changed := state != c.tvState || (err != nil && err.Error() != c.tvErr && state == webos.StateUnpaired)
		c.tvState = state
		c.tvErr = ""
		if err != nil && state == webos.StateUnpaired {
			c.tvErr = err.Error()
		}
		if state == webos.StateOn {
			c.tvLastOn = c.now()
		}
		c.mu.Unlock()
		if changed && err != nil && state == webos.StateUnpaired {
			c.log.Warn("TV state", "state", state, "error", err)
			c.poke()
		} else if changed {
			c.log.Info("TV state", "state", state)
			c.poke()
		}
	})
}

// PairTV registers with the TV (the user confirms on screen) and stores the key.
func (c *Controller) PairTV(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		host = c.store.Settings().TVHost
	}
	if err := validateHost(host); err != nil {
		return err
	}
	creds, err := webos.Pair(ctx, host)
	if err != nil {
		return err
	}
	if err := c.store.SetTV(creds); err != nil {
		return err
	}
	if _, err := c.store.UpdateSettings(func(s *store.Settings) { s.TVHost = host }); err != nil {
		return err
	}
	c.log.Info("TV paired", "host", host)
	c.restartTVWatch()
	return nil
}

func validateHost(host string) error {
	if host == "" {
		return errors.New("TV address is missing")
	}
	if len(host) > 253 || strings.ContainsAny(host, " /:@?#") && net.ParseIP(host) == nil {
		return errors.New("TV address must be a host name or IP address")
	}
	return nil
}

// MARK: Automation

func (c *Controller) automationContext(now time.Time, settings store.Settings) (automation.Context, automation.Schedule) {
	schedule := scheduleOf(settings)
	c.mu.Lock()
	defer c.mu.Unlock()
	tvActive := c.tvState == webos.StateOn ||
		(!c.tvLastOn.IsZero() && now.Sub(c.tvLastOn) < tvOffGracePeriod)
	if c.tvState == webos.StateOn {
		c.tvLastOn = now
	}
	return automation.Context{InWindow: schedule.Contains(now), TVActive: tvActive}, schedule
}

func scheduleOf(s store.Settings) automation.Schedule {
	on, err1 := automation.ParseClock(s.OnTime)
	off, err2 := automation.ParseClock(s.OffTime)
	if err1 != nil || err2 != nil {
		return automation.Schedule{}
	}
	return automation.Schedule{On: on, Off: off}
}

func (c *Controller) evaluate(ctx context.Context) {
	now := c.now()
	settings := c.store.Settings()
	actx, _ := c.automationContext(now, settings)

	c.mu.Lock()
	if c.override != nil && *c.override != actx {
		c.log.Info("manual override ended")
		c.override = nil
	}
	target := automation.Decide(actx)
	skip := !settings.AutomationEnabled || c.override != nil || c.applied == target ||
		now.Sub(c.lastFailure) < retryAfterFailure
	c.mu.Unlock()
	if skip {
		return
	}

	c.cmdMu.Lock()
	err := c.apply(ctx, target, settings)
	c.cmdMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastFailure = now
		c.automationErr = userMessage(err)
		c.log.Warn("automation failed", "target", target, "error", err)
		return
	}
	c.applied = target
	c.automationErr = ""
	c.log.Info("automation applied", "target", target)
}

// apply switches the light to a target. The caller holds cmdMu.
func (c *Controller) apply(ctx context.Context, target automation.Target, settings store.Settings) error {
	d := c.currentDevice()
	if d == nil {
		return ErrNotReady
	}
	switch target {
	case automation.TargetOff:
		if err := c.light.SetPower(ctx, *d, false); err != nil {
			return err
		}
		c.updateState(func(s *govee.State) { s.Power = false }, "off")
	case automation.TargetColor:
		if err := c.applyColor(ctx, *d, settings.Color, settings.Brightness); err != nil {
			return err
		}
	case automation.TargetVideo:
		if err := c.applyVideo(ctx, *d, settings.VideoSource, settings.VideoBrightness); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) applyColor(ctx context.Context, d govee.Device, color, brightness int) error {
	if err := c.light.SetPower(ctx, d, true); err != nil {
		return err
	}
	if err := c.light.SetColor(ctx, d, color); err != nil {
		return err
	}
	if err := c.light.SetBrightness(ctx, d, brightness); err != nil {
		return err
	}
	c.updateState(func(s *govee.State) {
		s.Power, s.ColorRGB, s.Brightness = true, color, brightness
	}, "color")
	return nil
}

func (c *Controller) applyVideo(ctx context.Context, d govee.Device, ref *store.SceneRef, brightness int) error {
	src, err := c.resolveSource(ctx, ref)
	if err != nil {
		return err
	}
	if err := c.light.SetPower(ctx, d, true); err != nil {
		return err
	}
	if err := c.light.Control(ctx, d, govee.TypeDynamicScene, src.kind, src.value); err != nil {
		// A snapshot saved again under the same name gets a new id: reload the list once and retry.
		c.refreshSources(ctx)
		fresh, findErr := c.resolveSource(ctx, ref)
		if findErr != nil || string(fresh.value) == string(src.value) {
			return err
		}
		if err := c.light.Control(ctx, d, govee.TypeDynamicScene, fresh.kind, fresh.value); err != nil {
			return err
		}
	}
	// Set after the scene, because recalling a snapshot also restores its brightness.
	if err := c.light.SetBrightness(ctx, d, brightness); err != nil {
		return err
	}
	c.updateState(func(s *govee.State) { s.Power, s.Brightness = true, brightness }, "video")
	return nil
}

var errNoVideoSource = errors.New(`no video scene set: save a snapshot called "Video" in the Govee app (with Video mode active)`)

func (c *Controller) resolveSource(ctx context.Context, ref *store.SceneRef) (source, error) {
	if ref == nil {
		return source{}, errNoVideoSource
	}
	find := func() (source, bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, s := range c.sources {
			if s.kind == ref.Kind && s.name == ref.Name {
				return s, true
			}
		}
		return source{}, false
	}
	if s, ok := find(); ok {
		return s, nil
	}
	// Maybe created in the Govee app after the last refresh. Limited, because a missing scene
	// would otherwise cost two API calls on every retry.
	c.mu.Lock()
	stale := c.now().Sub(c.sourcesLoaded) >= sourcesRetry
	c.mu.Unlock()
	if stale {
		c.refreshSources(ctx)
	}
	if s, ok := find(); ok {
		return s, nil
	}
	return source{}, fmt.Errorf("video scene %q not found in the Govee app", ref.Name)
}

func (c *Controller) updateState(fn func(*govee.State), mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.state)
	c.state.Online = true
	c.stateKnown = true
	c.mode = mode
	c.lightErr = ""
}

// MARK: Manual control

type LightRequest struct {
	Power      *bool   `json:"power,omitempty"`
	Brightness *int    `json:"brightness,omitempty"`
	Color      *int    `json:"color,omitempty"`
	Mode       *string `json:"mode,omitempty"` // "color" or "video"
}

func (r LightRequest) Validate() error {
	if r.Brightness != nil && (*r.Brightness < 1 || *r.Brightness > 100) {
		return errors.New("brightness must be 1–100")
	}
	if r.Color != nil && (*r.Color < 0 || *r.Color > 0xFFFFFF) {
		return errors.New("color must be 0x000000–0xFFFFFF")
	}
	if r.Mode != nil && *r.Mode != "color" && *r.Mode != "video" {
		return errors.New(`mode must be "color" or "video"`)
	}
	if r.Power == nil && r.Brightness == nil && r.Color == nil && r.Mode == nil {
		return errors.New("nothing to change")
	}
	return nil
}

// SetLight applies a manual change. The colour and brightness chosen in colour mode become the
// daytime colour. If the result differs from what the automation wants, the automation pauses
// until the next event (TV on/off, schedule).
func (c *Controller) SetLight(ctx context.Context, r LightRequest) error {
	if err := r.Validate(); err != nil {
		return err
	}
	d := c.currentDevice()
	if d == nil {
		return ErrNotReady
	}
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()
	settings := c.store.Settings()

	switch {
	case r.Power != nil && !*r.Power:
		if err := c.light.SetPower(ctx, *d, false); err != nil {
			return c.fail(err)
		}
		c.updateState(func(s *govee.State) { s.Power = false }, "off")
	case r.Mode != nil && *r.Mode == "video":
		brightness := settings.VideoBrightness
		if r.Brightness != nil {
			brightness = *r.Brightness
		}
		if err := c.applyVideo(ctx, *d, settings.VideoSource, brightness); err != nil {
			return c.fail(err)
		}
		if _, err := c.store.UpdateSettings(func(s *store.Settings) { s.VideoBrightness = brightness }); err != nil {
			return err
		}
	case r.Brightness != nil && r.Color == nil && r.Mode == nil && (c.currentMode() == "color" || c.currentMode() == "video"):
		// Dimming: one request is enough, and the level is remembered for the current mode.
		mode := c.currentMode()
		if err := c.light.SetBrightness(ctx, *d, *r.Brightness); err != nil {
			return c.fail(err)
		}
		c.updateState(func(s *govee.State) { s.Power, s.Brightness = true, *r.Brightness }, mode)
		if _, err := c.store.UpdateSettings(func(s *store.Settings) {
			if mode == "video" {
				s.VideoBrightness = *r.Brightness
			} else {
				s.Brightness = *r.Brightness
			}
		}); err != nil {
			return err
		}
	case r.Color != nil || (r.Mode != nil && *r.Mode == "color"):
		color, brightness := settings.Color, settings.Brightness
		if r.Color != nil {
			color = *r.Color
		}
		if r.Brightness != nil {
			brightness = *r.Brightness
		}
		if err := c.applyColor(ctx, *d, color, brightness); err != nil {
			return c.fail(err)
		}
		if _, err := c.store.UpdateSettings(func(s *store.Settings) { s.Color, s.Brightness = color, brightness }); err != nil {
			return err
		}
	case r.Brightness != nil:
		if err := c.light.SetBrightness(ctx, *d, *r.Brightness); err != nil {
			return c.fail(err)
		}
		mode := c.currentMode()
		c.updateState(func(s *govee.State) { s.Power, s.Brightness = true, *r.Brightness }, mode)
	case r.Power != nil && *r.Power:
		if err := c.light.SetPower(ctx, *d, true); err != nil {
			return c.fail(err)
		}
		mode := c.currentMode()
		if mode == "off" {
			mode = modeUnknown
		}
		c.updateState(func(s *govee.State) { s.Power = true }, mode)
	}

	c.noteManualResult(settings)
	return nil
}

func (c *Controller) currentMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

func (c *Controller) fail(err error) error {
	c.setLightErr(err)
	return errors.New(userMessage(err))
}

// noteManualResult compares the new light state with the automation's target.
func (c *Controller) noteManualResult(settings store.Settings) {
	actx, _ := c.automationContext(c.now(), settings)
	target := automation.Decide(actx)
	c.mu.Lock()
	defer c.mu.Unlock()
	result := automation.Target(c.mode)
	if c.mode == modeUnknown {
		result = automation.TargetColor
	}
	if !settings.AutomationEnabled {
		return
	}
	if result == target {
		c.override = nil
		c.applied = target
		return
	}
	c.override = &actx
	c.applied = appliedManual
	c.log.Info("manual override", "light", result, "automation", target)
}

// ResumeAutomation drops a manual override and applies the automation's target right away.
func (c *Controller) ResumeAutomation() {
	c.mu.Lock()
	c.override = nil
	c.applied = ""
	c.lastFailure = time.Time{}
	c.mu.Unlock()
	c.poke()
}

// MARK: Settings

type SettingsPatch struct {
	AutomationEnabled *bool           `json:"automationEnabled,omitempty"`
	OnTime            *string         `json:"onTime,omitempty"`
	OffTime           *string         `json:"offTime,omitempty"`
	Color             *int            `json:"color,omitempty"`
	Brightness        *int            `json:"brightness,omitempty"`
	VideoBrightness   *int            `json:"videoBrightness,omitempty"`
	VideoSource       *store.SceneRef `json:"videoSource,omitempty"`
	TVHost            *string         `json:"tvHost,omitempty"`
}

func (p SettingsPatch) Validate() error {
	for _, t := range []*string{p.OnTime, p.OffTime} {
		if t != nil {
			if _, err := automation.ParseClock(*t); err != nil {
				return err
			}
		}
	}
	if p.Color != nil && (*p.Color < 0 || *p.Color > 0xFFFFFF) {
		return errors.New("color must be 0x000000–0xFFFFFF")
	}
	for _, b := range []*int{p.Brightness, p.VideoBrightness} {
		if b != nil && (*b < 1 || *b > 100) {
			return errors.New("brightness must be 1–100")
		}
	}
	if p.VideoSource != nil && p.VideoSource.Kind != sourceKindSnapshot && p.VideoSource.Kind != sourceKindDIY {
		return errors.New(`videoSource.kind must be "snapshot" or "diyScene"`)
	}
	if p.VideoSource != nil && (p.VideoSource.Name == "" || len(p.VideoSource.Name) > 200) {
		return errors.New("videoSource.name is invalid")
	}
	if p.TVHost != nil && *p.TVHost != "" {
		if err := validateHost(strings.TrimSpace(*p.TVHost)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) UpdateSettings(p SettingsPatch) (store.Settings, error) {
	if err := p.Validate(); err != nil {
		return store.Settings{}, err
	}
	before := c.store.Settings()
	after, err := c.store.UpdateSettings(func(s *store.Settings) {
		if p.AutomationEnabled != nil {
			s.AutomationEnabled = *p.AutomationEnabled
		}
		if p.OnTime != nil {
			s.OnTime = *p.OnTime
		}
		if p.OffTime != nil {
			s.OffTime = *p.OffTime
		}
		if p.Color != nil {
			s.Color = *p.Color
		}
		if p.Brightness != nil {
			s.Brightness = *p.Brightness
		}
		if p.VideoBrightness != nil {
			s.VideoBrightness = *p.VideoBrightness
		}
		if p.VideoSource != nil {
			s.VideoSource = p.VideoSource
		}
		if p.TVHost != nil {
			s.TVHost = strings.TrimSpace(*p.TVHost)
		}
	})
	if err != nil {
		return after, err
	}

	if before.TVHost != after.TVHost {
		_ = c.store.SetTV(webos.Credentials{}) // a different TV needs its own pairing
		c.restartTVWatch()
	}
	c.mu.Lock()
	switch {
	case after.AutomationEnabled && !before.AutomationEnabled:
		c.override, c.applied = nil, ""
	case c.applied == automation.TargetColor && (before.Color != after.Color || before.Brightness != after.Brightness):
		c.applied = "" // show the new daytime colour right away
	case c.applied == automation.TargetVideo &&
		(!sameScene(before.VideoSource, after.VideoSource) || before.VideoBrightness != after.VideoBrightness):
		c.applied = ""
	}
	c.lastFailure = time.Time{}
	c.mu.Unlock()
	c.poke()
	return after, nil
}

func sameScene(a, b *store.SceneRef) bool {
	return a == b || (a != nil && b != nil && *a == *b)
}

// MARK: Status

type Status struct {
	Light        LightStatus      `json:"light"`
	TV           TVStatus         `json:"tv"`
	Automation   AutomationStatus `json:"automation"`
	Settings     store.Settings   `json:"settings"`
	VideoSources []store.SceneRef `json:"videoSources"`
}

type LightStatus struct {
	Ready      bool   `json:"ready"`
	Name       string `json:"name"`
	SKU        string `json:"sku"`
	Online     bool   `json:"online"`
	Power      bool   `json:"power"`
	Brightness int    `json:"brightness"`
	Color      int    `json:"color"`
	Mode       string `json:"mode"`
	Error      string `json:"error,omitempty"`
}

type TVStatus struct {
	Host   string `json:"host"`
	Paired bool   `json:"paired"`
	State  string `json:"state"`
	Active bool   `json:"active"`
	Error  string `json:"error,omitempty"`
}

type AutomationStatus struct {
	Enabled    bool       `json:"enabled"`
	Overridden bool       `json:"overridden"`
	Target     string     `json:"target"`
	Reason     string     `json:"reason"`
	NextChange *time.Time `json:"nextChange,omitempty"`
	Error      string     `json:"error,omitempty"`
}

func (c *Controller) Status() Status {
	now := c.now()
	settings := c.store.Settings()
	actx, schedule := c.automationContext(now, settings)
	paired := c.store.TV().ClientKey != ""

	c.mu.Lock()
	defer c.mu.Unlock()
	st := Status{Settings: settings, VideoSources: []store.SceneRef{}}
	if c.device != nil {
		st.Light = LightStatus{
			Ready: true, Name: c.device.Name, SKU: c.device.SKU,
			Online: c.state.Online || !c.stateKnown, Power: c.state.Power,
			Brightness: c.state.Brightness, Color: c.state.ColorRGB, Mode: c.mode,
		}
	}
	st.Light.Error = c.lightErr
	st.TV = TVStatus{Host: settings.TVHost, Paired: paired, State: c.tvState, Active: actx.TVActive, Error: c.tvErr}
	st.Automation = AutomationStatus{
		Enabled:    settings.AutomationEnabled,
		Overridden: c.override != nil,
		Target:     string(automation.Decide(actx)),
		Reason:     automation.Reason(actx, schedule),
		Error:      c.automationErr,
	}
	if next := schedule.NextChange(now); !next.IsZero() {
		st.Automation.NextChange = &next
	}
	for _, s := range c.sources {
		st.VideoSources = append(st.VideoSources, store.SceneRef{Kind: s.kind, Name: s.name})
	}
	return st
}

// userMessage turns errors into short texts for the app, without internal details.
func userMessage(err error) string {
	switch {
	case errors.Is(err, govee.ErrUnauthorized):
		return "Govee rejected the API key."
	case errors.Is(err, govee.ErrRateLimited):
		return "Govee rate limit reached, try again later."
	case errors.Is(err, context.DeadlineExceeded):
		return "Govee did not respond in time."
	default:
		return err.Error()
	}
}
