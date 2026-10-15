package panel

import (
 "testing"
 "net/http/httptest"
 "time"
)
func TestDirectHTTPAccessIgnoresForwardedHeaders(t *testing.T) {
 a := &App{trustProxy: true}
 r := httptest.NewRequest("POST", "http://192.0.2.10:9090/api/login", nil)
 r.RemoteAddr = "198.51.100.20:45678"
 r.Header.Set("Origin", "http://192.0.2.10:9090")
 r.Header.Set("X-Real-IP", "127.0.0.1")
 r.Header.Set("X-Forwarded-Proto", "https")
 if !a.validOrigin(r) || a.requestScheme(r) != "http" || a.clientIP(r) != "198.51.100.20" {
  t.Fatal("direct HTTP access must use the peer address and actual scheme")
 }
 r.Header.Set("Origin", "http://192.0.2.10:9091")
 if a.validOrigin(r) { t.Fatal("cross-port origin accepted") }
}
func TestIPAllowlist(t *testing.T){for _,tc:=range []struct{list,ip string;want bool}{{"","192.0.2.1",true},{"192.0.2.0/24","192.0.2.1",true},{"192.0.2.0/24","198.51.100.1",false},{"::1","::1",true},{"invalid","192.0.2.1",false}}{if got:=allowedIP(tc.list,tc.ip);got!=tc.want{t.Errorf("%s / %s = %v",tc.list,tc.ip,got)}}}
func TestQuietWindowCrossesMidnight(t *testing.T){a:=&App{config:map[string]string{"alerts.quiet.start":"22:00","alerts.quiet.end":"07:00"}};if !a.quiet(time.Date(2026,1,1,23,0,0,0,time.UTC))||!a.quiet(time.Date(2026,1,1,6,0,0,0,time.UTC))||a.quiet(time.Date(2026,1,1,12,0,0,0,time.UTC)){t.Fatal("incorrect quiet window")}}
func TestOriginPreservesHTTPSPort(t *testing.T){a:=&App{};r:=httptest.NewRequest("POST","https://panel.example.com:8443/api/login",nil);r.Header.Set("Origin","https://panel.example.com:8443");if !a.validOrigin(r){t.Fatal("matching TLS port rejected")};r.Header.Set("Origin","https://panel.example.com:9443");if a.validOrigin(r){t.Fatal("cross-port origin accepted")};r.Header.Set("Origin","https://attacker.example.com:8443");if a.validOrigin(r){t.Fatal("foreign origin accepted")}}
