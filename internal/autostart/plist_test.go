package autostart

import (
	"strings"
	"testing"
)

func TestRenderLaunchAgentPlistIncludesConfigFlag(t *testing.T) {
	plist := renderLaunchAgentPlist(
		"com.test.gateway",
		"/usr/local/bin/anthropic-gateway",
		"/Users/test/config.yaml",
		"/Users/test/Library/Logs/gateway.out.log",
		"/Users/test/Library/Logs/gateway.err.log",
	)

	if !strings.Contains(plist, "<string>-c</string>") {
		t.Fatalf("plist missing -c argument")
	}
	if !strings.Contains(plist, "<string>/Users/test/config.yaml</string>") {
		t.Fatalf("plist missing config path")
	}
}
