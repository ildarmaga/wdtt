package paneldb

// RawDirectPortOffset — default RAW WRAP listen = DTLS + offset (qWDTT / direct).
const RawDirectPortOffset = 3

// EffectiveRawDirectPort returns the UDP port for direct RAW (WRAP, no DTLS).
// rawDirectPort 0 means auto: dtlsPort + RawDirectPortOffset.
func EffectiveRawDirectPort(dtlsPort, rawDirectPort int) int {
	if rawDirectPort > 0 {
		return rawDirectPort
	}
	if dtlsPort > 0 {
		return dtlsPort + RawDirectPortOffset
	}
	return 56000 + RawDirectPortOffset
}
