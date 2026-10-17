package broker

import (
	"strings"
	"testing"

	"velexa-sim-me/internal/config"
)

func TestEndpointsFollowThisMachine(t *testing.T) {
	for _, k := range []string{"ME_PUB_IP", "ME_PUB_HOST", "ME_INT_IP", "ME_ITCH_IP", "ME_FIXS_PORT", "ME_INT_PORT", "ME_ITCH_PORT"} {
		t.Setenv(k, "")
	}
	t.Setenv("FIX_PORT", "6001")
	t.Setenv("ITCH_PORT", "6501")
	ep := meEndpoints()
	lan := config.LocalIP()
	if ep["pub_ip"] != lan || ep["pub_host"] != lan || ep["itch_ip"] != lan || ep["ext_addr"] != lan+":6001" || ep["itch_port"] != "6501" {
		t.Fatalf("defaults should be this PC and the real ports: %v", ep)
	}
	if ep["ext_tls"] != "" || ep["show_int"] != "" {
		t.Fatalf("no TLS gateway and no separate internal endpoint expected: %v", ep)
	}
	for _, v := range ep {
		if strings.Contains(v, "103.169.105.76") || strings.Contains(v, "velexa.net") || strings.Contains(v, "10.1.1.9") {
			t.Fatalf("production address leaked: %v", ep)
		}
	}
	sheet := brokerSheetHTML("Co", "JBL", "p", "JBL", "q")
	if strings.Contains(sheet, "SocketUseSSL") || !strings.Contains(sheet, lan+":6001") {
		t.Fatal("sheet should show plain FIX on this PC without TLS instructions")
	}

	t.Setenv("ME_PUB_IP", "203.0.113.10")
	t.Setenv("ME_FIXS_PORT", "5443")
	ep = meEndpoints()
	if ep["ext_addr"] != "203.0.113.10:5443" || ep["ext_tls"] != "1" || ep["itch_ip"] != "203.0.113.10" {
		t.Fatalf("env values must win: %v", ep)
	}
	if !strings.Contains(brokerSheetHTML("Co", "JBL", "p", "JBL", "q"), "SocketUseSSL") {
		t.Fatal("TLS instructions missing when a FIXS gateway is configured")
	}
}
