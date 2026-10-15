package panel

import (
 "context"
 "errors"
 "net"
 "net/http"
 "path/filepath"
 "time"
)
func(a *App) overview(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{w.WriteHeader(405);return};a.overviewMu.Lock();defer a.overviewMu.Unlock();if time.Since(a.overviewTime)>10*time.Second{summary:=map[string]int{};for key,q:=range map[string]string{"sites":"SELECT count(*) FROM sites","alerts":"SELECT count(*) FROM alerts WHERE active=1","services":"SELECT count(*) FROM alerts WHERE active=1 AND key LIKE 'service:%'"}{var n int;if err:=a.db.QueryRow(q).Scan(&n);err!=nil{problem(w,500,err);return};summary[key]=n};summary["certificates"]=0;files,_:=filepath.Glob(filepath.Join(a.data,"certificates","*.json"));for _,p:=range files{var c Certificate;if decodeJSON(readFile(p),&c)==nil&&time.Until(c.Expires)<30*24*time.Hour{summary["certificates"]++}};a.overviewCache=summary;a.overviewTime=time.Now()};reply(w,200,a.overviewCache)}
func(a *App) domainCheck(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{w.WriteHeader(405);return};domain:=r.URL.Query().Get("domain");if !domainName.MatchString(domain)||len(domain)>253{problem(w,400,errors.New("invalid domain"));return};ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel();addresses,err:=net.DefaultResolver.LookupIPAddr(ctx,domain);if err!=nil{problem(w,400,err);return};items:=[]string{};for _,ip:=range addresses{if len(items)>=20{break};items=append(items,ip.IP.String())};reply(w,200,map[string]any{"domain":domain,"addresses":items,"notice":"Verify these IPs point to this server or its NAT/public address; ACME validation requires public HTTP port 80."})}
func(a *App) loginBlocks(w http.ResponseWriter,r *http.Request,u User){if !admin(w,u){return};if r.Method=="GET"{a.mu.RLock();items:=[]map[string]any{};for ip,f:=range a.failures{if time.Now().Before(f.Until){items=append(items,map[string]any{"ip":ip,"attempts":f.Count,"until":f.Until})}};a.mu.RUnlock();reply(w,200,items);return};if r.Method=="POST"{var in struct{IP string};if !body(w,r,&in){return};a.mu.Lock();delete(a.failures,in.IP);a.mu.Unlock();a.audit(u.Name,"login.unblock",in.IP,"ok");reply(w,200,map[string]bool{"ok":true});return};w.WriteHeader(405)}
