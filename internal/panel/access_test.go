package panel

import (
 "testing"
 "net/http/httptest"
 "time"
)
func TestIPAllowlist(t *testing.T){for _,tc:=range []struct{list,ip string;want bool}{{"","192.0.2.1",true},{"192.0.2.0/24","192.0.2.1",true},{"192.0.2.0/24","198.51.100.1",false},{"::1","::1",true},{"invalid","192.0.2.1",false}}{if got:=allowedIP(tc.list,tc.ip);got!=tc.want{t.Errorf("%s / %s = %v",tc.list,tc.ip,got)}}}
func TestQuietWindowCrossesMidnight(t *testing.T){a:=&App{config:map[string]string{"alerts.quiet.start":"22:00","alerts.quiet.end":"07:00"}};if !a.quiet(time.Date(2026,1,1,23,0,0,0,time.UTC))||!a.quiet(time.Date(2026,1,1,6,0,0,0,time.UTC))||a.quiet(time.Date(2026,1,1,12,0,0,0,time.UTC)){t.Fatal("incorrect quiet window")}}
func TestOriginPreservesHTTPSPort(t *testing.T){a:=&App{};r:=httptest.NewRequest("POST","https://panel.example.com:8443/api/login",nil);r.Header.Set("Origin","https://panel.example.com:8443");if !a.validOrigin(r){t.Fatal("matching TLS port rejected")};r.Header.Set("Origin","https://panel.example.com:9443");if a.validOrigin(r){t.Fatal("cross-port origin accepted")};r.Header.Set("Origin","https://attacker.example.com:8443");if a.validOrigin(r){t.Fatal("foreign origin accepted")}}
