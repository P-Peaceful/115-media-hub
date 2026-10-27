package drive115

import (
	"errors"
	"testing"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

func TestSafeNetworkErrorExtractsAndRedactsUpstreamFields(t *testing.T) {
	raw := errors.Join(driver.ErrUnexpected, errors.New(`{"code":401,"message":"拒绝访问 Cookie=UID=secret; sign=qr-secret; token=access-secret"}`))
	err := safeNetworkError(raw)
	upstream, ok := AsUpstreamError(err)
	if !ok {
		t.Fatal("safeNetworkError did not retain the upstream error")
	}
	if upstream.Code != "401" {
		t.Fatalf("code = %q, want 401", upstream.Code)
	}
	if upstream.Message != "拒绝访问 Cookie=[已隐藏]; sign=[已隐藏]; token=[已隐藏]" {
		t.Fatalf("message was not safely redacted: %q", upstream.Message)
	}
	if errors.Is(err, ErrLimited) == false {
		t.Fatal("error should retain its safe local classification")
	}
	for _, secret := range []string{"secret", "qr-secret", "access-secret", raw.Error()} {
		if secret != "" && contains(upstream.Error(), secret) {
			t.Fatalf("message leaks %q", secret)
		}
	}
}

func TestSafeNetworkErrorDoesNotEchoUnstructuredSDKBody(t *testing.T) {
	err := safeNetworkError(errors.Join(driver.ErrUnexpected, errors.New("raw response contains UID=abc")))
	upstream, ok := AsUpstreamError(err)
	if !ok {
		t.Fatal("safeNetworkError did not retain the upstream error")
	}
	if upstream.Message != "115 上游请求失败" {
		t.Fatalf("message = %q", upstream.Message)
	}
	if contains(upstream.Error(), "UID=abc") {
		t.Fatal("raw SDK body leaked")
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
