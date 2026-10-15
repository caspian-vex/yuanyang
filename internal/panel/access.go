package panel

import (
 "net"
 "net/http"
 "net/url"
 "strconv"
 "strings"
)
func(a *App) clientIP(r *http.Request)string{host,_,err:=net.SplitHostPort(r.RemoteAddr);if err!=nil{return ""};ip:=net.ParseIP(host);if a.trustProxy&&ip!=nil&&ip.IsLoopback(){if real:=net.ParseIP(r.Header.Get("X-Real-IP"));real!=nil{return real.String()}};return host}
func allowedIP(list,source string)bool{if strings.TrimSpace(list)==""{return true};ip:=net.ParseIP(source);if ip==nil{return false};for _,item:=range strings.Split(list,","){item=strings.TrimSpace(item);if other:=net.ParseIP(item);other!=nil&&other.Equal(ip){return true};if _,network,err:=net.ParseCIDR(item);err==nil&&network.Contains(ip){return true}};return false}
func(a *App) intSetting(key string,fallback int)int{n,err:=strconv.Atoi(a.setting(key,""));if err!=nil{return fallback};return n}
func(a *App) requestScheme(r *http.Request)string{if r.TLS!=nil{return "https"};peer,_,_:=net.SplitHostPort(r.RemoteAddr);ip:=net.ParseIP(peer);if a.trustProxy&&ip!=nil&&ip.IsLoopback()&&r.Header.Get("X-Forwarded-Proto")=="https"{return "https"};return "http"}
func(a *App) validOrigin(r *http.Request)bool{raw:=r.Header.Get("Origin");if raw==""{return true};origin,err:=url.Parse(raw);return err==nil&&origin.Host==r.Host&&origin.Scheme==a.requestScheme(r)}
