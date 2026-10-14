// Package govee is a small client for the Govee Platform API v2 (https://developer.govee.com).
package govee

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://openapi.api.govee.com/router/api/v1"

// Capability types and instances used by this app.
const (
	TypeOnOff        = "devices.capabilities.on_off"
	TypeRange        = "devices.capabilities.range"
	TypeColor        = "devices.capabilities.color_setting"
	TypeDynamicScene = "devices.capabilities.dynamic_scene"
	TypeOnline       = "devices.capabilities.online"

	InstancePower      = "powerSwitch"
	InstanceBrightness = "brightness"
	InstanceColorRGB   = "colorRgb"
	InstanceSnapshot   = "snapshot"
	InstanceDIYScene   = "diyScene"
	InstanceLightScene = "lightScene"
	InstanceOnline     = "online"
)

var (
	ErrUnauthorized = errors.New("govee: API key rejected")
	ErrRateLimited  = errors.New("govee: rate limit reached")
)

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// WithBaseURL is used by tests.
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = strings.TrimSuffix(url, "/")
	return c
}

type Device struct {
	SKU          string       `json:"sku"`
	ID           string       `json:"device"`
	Name         string       `json:"deviceName"`
	Capabilities []Capability `json:"capabilities"`
}

type Capability struct {
	Type       string `json:"type"`
	Instance   string `json:"instance"`
	Parameters struct {
		Options []Option `json:"options"`
	} `json:"parameters"`
}

// Option is a named enum value (scene, snapshot …). The value is passed back to the API unchanged.
type Option struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// Options returns the enum options of a capability instance, if the device reports it.
func (d Device) Options(instance string) []Option {
	for _, c := range d.Capabilities {
		if c.Instance == instance {
			return c.Parameters.Options
		}
	}
	return nil
}

type State struct {
	Online     bool
	Power      bool
	Brightness int
	ColorRGB   int
}

// Devices lists all devices of the account, including their capabilities.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var resp struct {
		Data []Device `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/user/devices", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// DIYScenes lists the DIY scenes saved in the Govee app for a device.
func (c *Client) DIYScenes(ctx context.Context, d Device) ([]Option, error) {
	var resp struct {
		Payload struct {
			Capabilities []Capability `json:"capabilities"`
		} `json:"payload"`
	}
	if err := c.do(ctx, http.MethodPost, "/device/diy-scenes", c.payload(d, nil), &resp); err != nil {
		return nil, err
	}
	for _, capability := range resp.Payload.Capabilities {
		if capability.Instance == InstanceDIYScene {
			return capability.Parameters.Options, nil
		}
	}
	return nil, nil
}

func (c *Client) State(ctx context.Context, d Device) (State, error) {
	var resp struct {
		Payload struct {
			Capabilities []struct {
				Instance string `json:"instance"`
				State    struct {
					Value json.RawMessage `json:"value"`
				} `json:"state"`
			} `json:"capabilities"`
		} `json:"payload"`
	}
	if err := c.do(ctx, http.MethodPost, "/device/state", c.payload(d, nil), &resp); err != nil {
		return State{}, err
	}
	var s State
	for _, capability := range resp.Payload.Capabilities {
		raw := capability.State.Value
		switch capability.Instance {
		case InstanceOnline:
			_ = json.Unmarshal(raw, &s.Online)
		case InstancePower:
			var v int
			_ = json.Unmarshal(raw, &v)
			s.Power = v == 1
		case InstanceBrightness:
			_ = json.Unmarshal(raw, &s.Brightness)
		case InstanceColorRGB:
			_ = json.Unmarshal(raw, &s.ColorRGB)
		}
	}
	return s, nil
}

func (c *Client) SetPower(ctx context.Context, d Device, on bool) error {
	v := 0
	if on {
		v = 1
	}
	return c.Control(ctx, d, TypeOnOff, InstancePower, v)
}

func (c *Client) SetBrightness(ctx context.Context, d Device, percent int) error {
	return c.Control(ctx, d, TypeRange, InstanceBrightness, min(max(percent, 1), 100))
}

func (c *Client) SetColor(ctx context.Context, d Device, rgb int) error {
	return c.Control(ctx, d, TypeColor, InstanceColorRGB, rgb&0xFFFFFF)
}

func (c *Client) Control(ctx context.Context, d Device, capType, instance string, value any) error {
	body := c.payload(d, map[string]any{"type": capType, "instance": instance, "value": value})
	return c.do(ctx, http.MethodPost, "/device/control", body, nil)
}

func (c *Client) payload(d Device, capability map[string]any) map[string]any {
	p := map[string]any{"sku": d.SKU, "device": d.ID}
	if capability != nil {
		p["capability"] = capability
	}
	return map[string]any{"requestId": requestID(), "payload": p}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Govee-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("govee: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("govee: reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("govee: HTTP %d: %s", resp.StatusCode, apiMessage(data))
	}

	// The API also reports errors inside a 200 response.
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("govee: invalid response: %w", err)
	}
	if envelope.Code != 0 && envelope.Code != http.StatusOK {
		return fmt.Errorf("govee: error %d: %s", envelope.Code, apiMessage(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("govee: invalid response: %w", err)
		}
	}
	return nil
}

// apiMessage extracts the human readable message ("msg" or "message"), never the raw body.
func apiMessage(data []byte) string {
	var m struct {
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &m)
	text := m.Msg
	if text == "" {
		text = m.Message
	}
	if text == "" {
		return "unknown error"
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

func requestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
