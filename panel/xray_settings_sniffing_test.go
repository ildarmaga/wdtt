package panel

import "testing"

func TestEnsureRedirectInSniffingForAccessLog(t *testing.T) {
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag": "redirect-in",
				"sniffing": map[string]interface{}{
					"enabled":   true,
					"routeOnly": true,
					"destOverride": []interface{}{
						"http", "tls", "quic",
					},
				},
			},
		},
	}
	ensureRedirectInSniffingForAccessLog(cfg)
	inbounds := cfg["inbounds"].([]interface{})
	m := inbounds[0].(map[string]interface{})
	sniffing := m["sniffing"].(map[string]interface{})
	if sniffing["enabled"] != true {
		t.Fatalf("enabled=%v", sniffing["enabled"])
	}
	if sniffing["routeOnly"] != false {
		t.Fatalf("routeOnly=%v want false", sniffing["routeOnly"])
	}
	dest, _ := sniffing["destOverride"].([]interface{})
	if len(dest) != 4 {
		t.Fatalf("destOverride=%v", dest)
	}
}
