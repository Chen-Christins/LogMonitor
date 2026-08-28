package feishu

import "testing"

func TestSignature(t *testing.T) {
	const timestamp = "1599360473"
	const secret = "test-secret"
	const expected = "wSds2BzzFIIGf/WrhUO+NI1q/9j+FRJd3JNHKAq0NZY="

	if actual := signature(timestamp, secret); actual != expected {
		t.Fatalf("signature() = %q, want %q", actual, expected)
	}
}
