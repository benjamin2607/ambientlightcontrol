package webos

import "testing"

func TestClassifyPowerState(t *testing.T) {
	cases := []struct{ state, processing, want string }{
		{"Active", "", StateOn},
		{"Active", "Request Power Off", StateStandby},
		{"Active", "Prepare Suspend", StateStandby},
		{"Active Standby", "", StateStandby},
		{"Suspend", "", StateStandby},
		{"Screen Off", "", StateScreenOff},
		{"Screen Saver", "", StateIdle},
		{"Power Off", "", StateOff},
	}
	for _, c := range cases {
		if got := ClassifyPowerState(c.state, c.processing); got != c.want {
			t.Errorf("%q/%q = %q, want %q", c.state, c.processing, got, c.want)
		}
	}
}

func TestAcceptKey(t *testing.T) {
	// Example from RFC 6455, section 1.3.
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("got %s", got)
	}
}
