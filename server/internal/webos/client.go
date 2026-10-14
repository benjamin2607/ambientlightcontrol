// Package webos reads the power state of an LG webOS TV over the local SSAP WebSocket API (port 3001, TLS).
package webos

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	port         = "3001"
	powerURI     = "ssap://com.webos.service.tvpower/power/getPowerState"
	pingInterval = 20 * time.Second
	idleTimeout  = 50 * time.Second
)

// Power states reported by Watch. Only StateOn counts as "TV in use".
const (
	StateOn        = "on"         // picture on
	StateIdle      = "idle"       // screen saver (e.g. no input signal)
	StateScreenOff = "screen_off" // picture off, sound may continue
	StateStandby   = "standby"    // switched off but still on the network (Quick Start+)
	StateOff       = "off"        // not reachable
	StateUnpaired  = "unpaired"   // no or rejected client key
)

var (
	ErrNotPaired          = errors.New("webos: TV rejected the pairing key, pair again")
	ErrCertificateChanged = errors.New("webos: TV certificate changed, pair again")
	ErrPairingDenied      = errors.New("webos: pairing was declined on the TV")
)

// Credentials are created by Pair and must be stored for later sessions.
type Credentials struct {
	ClientKey string `json:"clientKey"`
	// CertSHA256 pins the TV's self-signed TLS certificate (trust on first use).
	CertSHA256 string `json:"certSHA256"`
}

type message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Error   string          `json:"error"`
	Payload json.RawMessage `json:"payload"`
}

// Pair registers this app on the TV. The TV shows a prompt that must be accepted with the remote.
func Pair(ctx context.Context, host string) (Credentials, error) {
	conn, fingerprint, err := dial(ctx, host, "")
	if err != nil {
		return Credentials{}, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	key, err := register(conn, "")
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{ClientKey: key, CertSHA256: fingerprint}, nil
}

// Watch reports the TV power state until ctx is cancelled, reconnecting as needed.
// onState is called on every change and after connection problems.
func Watch(ctx context.Context, host string, creds Credentials, onState func(state string, err error)) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		err := session(ctx, host, creds, func(state string) {
			backoff = 5 * time.Second
			onState(state, nil)
		})
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrNotPaired), errors.Is(err, ErrCertificateChanged):
			onState(StateUnpaired, err)
			backoff = time.Minute
		default:
			// Without Quick Start+ the TV drops off the network when switched off.
			onState(StateOff, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func session(ctx context.Context, host string, creds Credentials, onState func(string)) error {
	conn, _, err := dial(ctx, host, creds.CertSHA256)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	conn.idle = idleTimeout
	_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
	if _, err := register(conn, creds.ClientKey); err != nil {
		return err
	}
	subscribe, _ := json.Marshal(map[string]any{"type": "subscribe", "id": "power", "uri": powerURI})
	if err := conn.WriteText(subscribe); err != nil {
		return err
	}

	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				if conn.Ping() != nil {
					return
				}
			}
		}
	}()

	for {
		data, err := conn.ReadText()
		if err != nil {
			return err
		}
		var msg message
		if json.Unmarshal(data, &msg) != nil || msg.ID != "power" {
			continue
		}
		if msg.Type == "error" {
			if strings.HasPrefix(msg.Error, "401") {
				return ErrNotPaired // registered with a manifest that lacks the permission
			}
			return fmt.Errorf("webos: power state: %s", msg.Error)
		}
		var payload struct {
			State      string `json:"state"`
			Processing string `json:"processing"`
		}
		if json.Unmarshal(msg.Payload, &payload) == nil && payload.State != "" {
			onState(ClassifyPowerState(payload.State, payload.Processing))
		}
	}
}

// ClassifyPowerState maps the raw webOS values to the states of this package.
func ClassifyPowerState(state, processing string) string {
	// "processing" announces a transition, e.g. "Request Power Off" or "Prepare Suspend".
	p := strings.ToLower(processing)
	if strings.Contains(p, "power off") || strings.Contains(p, "suspend") || strings.Contains(p, "screen off") {
		return StateStandby
	}
	switch state {
	case "Active":
		return StateOn
	case "Screen Saver":
		return StateIdle
	case "Screen Off":
		return StateScreenOff
	case "Power Off":
		return StateOff
	default: // "Active Standby", "Suspend" …
		return StateStandby
	}
}

func register(conn *wsConn, clientKey string) (string, error) {
	payload := map[string]any{
		"forcePairing": false,
		"pairingType":  "PROMPT",
		"manifest":     manifest,
	}
	if clientKey != "" {
		payload["client-key"] = clientKey
	}
	data, _ := json.Marshal(map[string]any{"type": "register", "id": "register", "payload": payload})
	if err := conn.WriteText(data); err != nil {
		return "", err
	}
	for {
		data, err := conn.ReadText()
		if err != nil {
			return "", err
		}
		var msg message
		if json.Unmarshal(data, &msg) != nil || msg.ID != "register" {
			continue
		}
		switch msg.Type {
		case "registered":
			var p struct {
				ClientKey string `json:"client-key"`
			}
			if json.Unmarshal(msg.Payload, &p) != nil || p.ClientKey == "" {
				return "", errors.New("webos: registration without client key")
			}
			return p.ClientKey, nil
		case "error":
			if clientKey != "" {
				return "", ErrNotPaired
			}
			if strings.Contains(strings.ToLower(msg.Error), "cancel") || strings.Contains(msg.Error, "403") {
				return "", ErrPairingDenied
			}
			return "", fmt.Errorf("webos: registration failed: %s", msg.Error)
		}
		// "response" with pairingType PROMPT: the TV now shows the prompt; keep waiting.
	}
}

// dial opens the TLS WebSocket. The TV uses a self-signed certificate, so instead of a CA check the
// certificate is pinned: an empty pin accepts any certificate (only during pairing) and returns its fingerprint.
func dial(ctx context.Context, host, pin string) (*wsConn, string, error) {
	var fingerprint string
	config := &tls.Config{
		InsecureSkipVerify: true, // replaced by the pin check below
		MinVersion:         tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("webos: no certificate")
			}
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			fingerprint = hex.EncodeToString(sum[:])
			if pin != "" && !strings.EqualFold(fingerprint, pin) {
				return ErrCertificateChanged
			}
			return nil
		},
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: config}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := dialer.DialContext(dialCtx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		if errors.Is(err, ErrCertificateChanged) {
			return nil, "", ErrCertificateChanged
		}
		return nil, "", err
	}
	_ = raw.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := wsHandshake(raw, net.JoinHostPort(host, port))
	if err != nil {
		raw.Close()
		return nil, "", err
	}
	_ = raw.SetDeadline(time.Time{})
	return conn, fingerprint, nil
}
