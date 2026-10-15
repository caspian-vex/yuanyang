package panel

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "mime"
 "net/http"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "time"

 "golang.org/x/crypto/bcrypt"
)
type User struct { ID int64 `json:"id"`; Name string `json:"name"`; Role string `json:"role"`; CSRF string `json:"csrf,omitempty"`;Terminal bool `json:"terminal"` }
func randomToken()string{b:=make([]byte,32);if _,err:=rand.Read(b);err!=nil{panic(err)};return hex.EncodeToString(b)}
func reply(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);json.NewEncoder(w).Encode(v)}
func problem(w http.ResponseWriter,status int,err error){reply(w,status,map[string]string{"error":err.Error()})}
func body(w http.ResponseWriter,r *http.Request,v any)bool{mediaType,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"));if err!=nil||mediaType!="application/json"{problem(w,415,errors.New("Content-Type must be application/json"));return false};r.Body=http.MaxBytesReader(w,r.Body,64<<10);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();if err:=d.Decode(v);err!=nil{problem(w,400,err);return false};if err:=d.Decode(new(any));err!=io.EOF{problem(w,400,errors.New("expected one JSON object"));return false};return true}
func(a *App) Handler()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Cache-Control","no-store");ctx,cancel:=context.WithTimeout(r.Context(),time.Second);defer cancel();if err:=a.db.PingContext(ctx);err!=nil{problem(w,503,errors.New("database unavailable"));return};reply(w,200,map[string]bool{"ok":true})})
 mux.HandleFunc("GET /api/status",func(w http.ResponseWriter,r *http.Request){var n int;a.db.QueryRow("SELECT count(*) FROM users").Scan(&n);reply(w,200,map[string]any{"initialized":n>0,"version":"0.1.0","name":a.setting("panel.name","鸳鸯")})})
 mux.HandleFunc("POST /api/login",a.login)
 mux.HandleFunc("/api/",a.auth(a.api))
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{w.WriteHeader(405);return};p:=filepath.Join(a.web,filepath.Clean("/"+r.URL.Path));if st,e:=os.Stat(p);e==nil&&!st.IsDir(){http.ServeFile(w,r,p);return};http.ServeFile(w,r,filepath.Join(a.web,"index.html"))})
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 if r.Method!="GET"&&r.Method!="HEAD"&&!a.validOrigin(r){problem(w,403,errors.New("cross-origin request denied"));return}
 if strings.HasPrefix(r.URL.Path,"/api/")&&!allowedIP(a.setting("security.allowIPs",""),a.clientIP(r)){problem(w,403,errors.New("source IP not allowed"));return}
 if strings.HasPrefix(r.URL.Path,"/api/")&&r.URL.Path!="/api/terminal"&&r.URL.Path!="/api/logs"{select{case a.requests<-struct{}{}:defer func(){<-a.requests}();default:problem(w,429,errors.New("request capacity reached"));return};deadline:=15*time.Second;if r.URL.Path=="/api/files"{deadline=10*time.Minute};controller:=http.NewResponseController(w);controller.SetReadDeadline(time.Now().Add(deadline));controller.SetWriteDeadline(time.Now().Add(10*time.Minute))}
 w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("X-Frame-Options","DENY");w.Header().Set("Referrer-Policy","no-referrer");w.Header().Set("Content-Security-Policy","default-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'")
 if strings.HasPrefix(r.URL.Path,"/api/"){w.Header().Set("Cache-Control","no-store")};mux.ServeHTTP(w,r)
 })
}
func(a *App) login(w http.ResponseWriter,r *http.Request){
 select{case a.logins<-struct{}{}:defer func(){<-a.logins}();default:problem(w,429,errors.New("login capacity reached"));return}
 ip:=a.clientIP(r)
 a.mu.Lock();f:=a.failures[ip];if f!=nil&&time.Now().Before(f.Until){a.mu.Unlock();problem(w,429,errors.New("too many login attempts"));return};a.mu.Unlock()
 var in struct{Name,Password,Code string};if !body(w,r,&in){return}
 if len(in.Name)<1||len(in.Name)>64||len(in.Password)<1||len(in.Password)>72||len(in.Code)>6{problem(w,400,errors.New("invalid credential length"));return}
 var u User;var hash,secret string
 err:=a.db.QueryRow("SELECT id,name,password,role,totp,terminal FROM users WHERE name=? AND disabled=0",in.Name).Scan(&u.ID,&u.Name,&hash,&u.Role,&secret,&u.Terminal)
 checkedHash:=hash;if err!=nil{checkedHash="$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"};passwordOK:=bcrypt.CompareHashAndPassword([]byte(checkedHash),[]byte(in.Password))==nil
 if secret!=""{plain,e:=a.unseal(secret);if e!=nil{secret="invalid"}else{secret=string(plain)}}
 if err!=nil||!passwordOK||(secret!=""&&!validTOTP(secret,in.Code,time.Now())){
 a.mu.Lock();if len(a.failures)>1024{for k,v:=range a.failures{if time.Now().After(v.Until){delete(a.failures,k)}}};if len(a.failures)<1024{if a.failures[ip]==nil{a.failures[ip]=&failure{}};f=a.failures[ip];f.Count++;if f.Count>=5{f.Until=time.Now().Add(15*time.Minute)}};a.mu.Unlock();a.audit(in.Name,"login",ip,"denied");problem(w,401,errors.New("invalid credentials"));return}
 a.mu.Lock();delete(a.failures,ip);a.mu.Unlock()
 token:=randomToken();u.CSRF=randomToken();now:=time.Now().Unix()
 if err=a.exec("INSERT INTO sessions VALUES(?,?,?,?,?)",token,u.ID,u.CSRF,now,now);err!=nil{problem(w,500,err);return}
 http.SetCookie(w,&http.Cookie{Name:"session",Value:token,Path:"/",HttpOnly:true,Secure:a.requestScheme(r)=="https",SameSite:http.SameSiteStrictMode,MaxAge:a.intSetting("session.maxHours",12)*3600})
 a.audit(u.Name,"login",ip,"ok");reply(w,200,u)
}
func(a *App) auth(next func(http.ResponseWriter,*http.Request,User))http.HandlerFunc{return func(w http.ResponseWriter,r *http.Request){
 c,err:=r.Cookie("session");if err!=nil{problem(w,401,errors.New("login required"));return}
 var u User;var created,seen int64
 err=a.db.QueryRow("SELECT u.id,u.name,u.role,s.csrf,s.created,s.seen,u.terminal FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND u.disabled=0",c.Value).Scan(&u.ID,&u.Name,&u.Role,&u.CSRF,&created,&seen,&u.Terminal)
 now:=time.Now().Unix();if err!=nil||now-created>int64(a.intSetting("session.maxHours",12)*3600)||now-seen>int64(a.intSetting("session.idleMinutes",30)*60){problem(w,401,errors.New("session expired"));return}
 if r.Method!="GET"&&subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")),[]byte(u.CSRF))!=1{problem(w,403,errors.New("invalid CSRF token"));return}
 selfAction:=r.Method=="POST"&&(r.URL.Path=="/api/logout"||r.URL.Path=="/api/password"||r.URL.Path=="/api/sessions"||r.URL.Path=="/api/security/totp")
 if r.Method!="GET"&&u.Role=="viewer"&&!selfAction{problem(w,403,errors.New("read only account"));return}
 if now-seen>60{a.exec("UPDATE sessions SET seen=? WHERE token=?",now,c.Value)}
 next(w,r,u)
}}
func admin(w http.ResponseWriter,u User)bool{if u.Role!="admin"{problem(w,403,errors.New("administrator required"));return false};return true}
func(a *App) api(w http.ResponseWriter,r *http.Request,u User){
 switch r.URL.Path{
 case "/api/me":if r.Method=="GET"{reply(w,200,u);return}
 case "/api/logout":if r.Method=="POST"{c,_:=r.Cookie("session");a.exec("DELETE FROM sessions WHERE token=?",c.Value);http.SetCookie(w,&http.Cookie{Name:"session",Path:"/",MaxAge:-1,HttpOnly:true,SameSite:http.SameSiteStrictMode});reply(w,200,map[string]bool{"ok":true});return}
 case "/api/metrics":if r.Method=="GET"{a.mu.RLock();m:=a.metric;a.mu.RUnlock();reply(w,200,m);return}
 case "/api/overview":a.overview(w,r);return
 case "/api/domains/check":a.domainCheck(w,r);return
 case "/api/login-blocks":a.loginBlocks(w,r,u);return
 case "/api/history":if r.Method=="GET"{a.historyAPI(w,r);return}
 case "/api/host":if r.Method=="GET"{reply(w,200,hostInfo());return}
 case "/api/reboot":if r.Method=="POST"&&admin(w,u){a.reboot(w,r,u)};return
 case "/api/password":if r.Method=="POST"{a.password(w,r,u)};return
 case "/api/sessions":a.sessions(w,r,u);return
 case "/api/processes":a.processAPI(w,r,u);return
 case "/api/ports":a.ports(w,r);return
 case "/api/files":if u.Role=="viewer"{problem(w,403,errors.New("file access denied"));return};a.files(w,r,u);return
 case "/api/terminal":if u.Role=="admin"||(u.Role=="operator"&&u.Terminal){a.terminal(w,r,u)}else{problem(w,403,errors.New("terminal permission required"))};return
 case "/api/sites":a.sites(w,r,u);return
 case "/api/services":a.services(w,r,u);return
 case "/api/logs":if u.Role=="viewer"{problem(w,403,errors.New("log access denied"));return};a.streamLogs(w,r);return
 case "/api/firewall":if admin(w,u){a.firewall(w,r,u)};return
 case "/api/mail/test":if r.Method=="POST"&&admin(w,u){if err:=a.sendMail(Notice{"Yuanyang mail test","Mail delivery is configured."});err!=nil{problem(w,400,err)}else{reply(w,200,map[string]bool{"ok":true})};return}
 case "/api/tasks":if admin(w,u){a.tasks(w,r,u)};return
 case "/api/run":if admin(w,u){a.runDetail(w,r)};return
 case "/api/users":if admin(w,u){a.users(w,r,u)};return
 case "/api/security/totp":a.totp(w,r,u);return
 case "/api/security":if admin(w,u){reply(w,200,map[string]any{"https":a.requestScheme(r)=="https","terminal":"administrator or explicitly granted operator","firewall":"UFW with 60-second rollback","notice":"TLS reverse proxy must overwrite forwarded headers"})};return
 case "/api/settings":if admin(w,u){a.settings(w,r,u)};return
 case "/api/gateway":a.gateway(w,r,u);return
 case "/api/certificates":if u.Role!="viewer"{a.certificates(w,r,u)}else{problem(w,403,errors.New("certificate access denied"))};return
 case "/api/diagnostics":a.diagnostics(w,r,u);return
 case "/api/alerts":if r.Method=="POST"{var in struct{ID int};if !body(w,r,&in){return};if err:=a.exec("UPDATE alerts SET ack_by=?,ack_at=? WHERE id=?",u.Name,time.Now().Unix(),in.ID);err!=nil{problem(w,500,err);return};a.audit(u.Name,"alert.acknowledge",strconv.Itoa(in.ID),"ok");reply(w,200,map[string]bool{"ok":true});return};if r.Method=="GET"{a.listTable(w,r,"alerts");return}
 case "/api/audit","/api/runs":if r.Method=="GET"{if !admin(w,u){return};table:=strings.TrimPrefix(r.URL.Path,"/api/");a.listTable(w,r,table);return}
 }
 problem(w,404,errors.New("endpoint or method not found"))
}
func page(r *http.Request)(int,int){limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));if limit<1{limit=50};if limit>200{limit=200};offset,_:=strconv.Atoi(r.URL.Query().Get("offset"));if offset<0{offset=0};return limit,offset}
func(a *App) listTable(w http.ResponseWriter,r *http.Request,table string){
 limit,offset:=page(r);columns:="*";if table=="runs"{columns="id,task_id,started,finished,substr(output,1,4096) AS output,status,exit_code"};where:=" WHERE 1=1";args:=[]any{};if table=="audit"{for _,k:=range []string{"user","action","result"}{if v:=r.URL.Query().Get(k);v!=""{where+=" AND "+k+" LIKE ?";args=append(args,"%"+v+"%")}};for _,f:=range []struct{key,op string}{{"since",">="},{"until","<="}}{if n,e:=strconv.ParseInt(r.URL.Query().Get(f.key),10,64);e==nil{where+=" AND time "+f.op+" ?";args=append(args,n)}}};if table=="alerts"{switch r.URL.Query().Get("state"){case "active":where+=" AND active=1";case "recovered":where+=" AND active=0"}};args=append(args,limit,offset);rows,err:=a.db.Query("SELECT "+columns+" FROM "+table+where+" ORDER BY id DESC LIMIT ? OFFSET ?",args...);if err!=nil{problem(w,500,err);return};defer rows.Close();cols,_:=rows.Columns();items:=[]map[string]any{}
 for rows.Next(){v:=make([]any,len(cols));p:=make([]any,len(cols));for i:=range v{p[i]=&v[i]};if err=rows.Scan(p...);err!=nil{problem(w,500,err);return};m:=map[string]any{};for i,k:=range cols{m[k]=v[i]};items=append(items,m)};reply(w,200,items)
}
func fail(err error)error{if err==nil{return nil};return fmt.Errorf("operation failed: %w",err)}
