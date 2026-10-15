package panel

import (
 "bytes"
 "crypto/subtle"
 "errors"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "sync"
 "time"

 "github.com/gorilla/websocket"
 "golang.org/x/crypto/ssh"
)
type socketWriter struct{conn *websocket.Conn;mu *sync.Mutex}
func(s socketWriter)Write(p []byte)(int,error){s.mu.Lock();defer s.mu.Unlock();s.conn.SetWriteDeadline(time.Now().Add(10*time.Second));err:=s.conn.WriteMessage(websocket.BinaryMessage,p);return len(p),err}
func(a *App) terminal(w http.ResponseWriter,r *http.Request,u User){
 if r.Method!="GET"{w.WriteHeader(405);return};select{case a.terminals<-struct{}{}:defer func(){<-a.terminals}();default:problem(w,429,errors.New("terminal limit reached"));return}
 upgrader:=websocket.Upgrader{ReadBufferSize:4096,WriteBufferSize:4096,CheckOrigin:func(r *http.Request)bool{o,err:=url.Parse(r.Header.Get("Origin"));return err==nil&&o.Host==r.Host}}
 ws,err:=upgrader.Upgrade(w,r,nil);if err!=nil{return};defer ws.Close();ws.SetReadLimit(32<<10);ws.SetReadDeadline(time.Now().Add(15*time.Second))
 var in struct{Username,Password,PrivateKey,Passphrase,CSRF string;Cols,Rows int};if ws.ReadJSON(&in)!=nil{return};if subtle.ConstantTimeCompare([]byte(in.CSRF),[]byte(u.CSRF))!=1{return}
 var methods []ssh.AuthMethod;if in.Password!=""{methods=append(methods,ssh.Password(in.Password))};if in.PrivateKey!=""{var key ssh.Signer;var e error;if in.Passphrase!=""{key,e=ssh.ParsePrivateKeyWithPassphrase([]byte(in.PrivateKey),[]byte(in.Passphrase))}else{key,e=ssh.ParsePrivateKey([]byte(in.PrivateKey))};if e!=nil{ws.WriteMessage(websocket.TextMessage,[]byte("invalid private key or passphrase"));return};methods=append(methods,ssh.PublicKeys(key))}
 // Trust the actual host's public key, never silently accept a changed SSH identity.
 keys:=[]ssh.PublicKey{};for _,path:=range []string{"/etc/ssh/ssh_host_ed25519_key.pub","/etc/ssh/ssh_host_ecdsa_key.pub","/etc/ssh/ssh_host_rsa_key.pub"}{public,e:=os.ReadFile(path);if e!=nil{continue};if key,_,_,_,e:=ssh.ParseAuthorizedKey(public);e==nil{keys=append(keys,key)}};if len(keys)==0{ws.WriteMessage(websocket.TextMessage,[]byte("cannot read local SSH public host keys"));return}
 config:=&ssh.ClientConfig{User:in.Username,Auth:methods,HostKeyCallback:func(_ string,_ net.Addr,key ssh.PublicKey)error{for _,trusted:=range keys{if bytes.Equal(trusted.Marshal(),key.Marshal()){return nil}};return errors.New("local SSH host identity mismatch")},Timeout:10*time.Second}
 raw,e:=net.DialTimeout("tcp","127.0.0.1:22",10*time.Second);if e!=nil{ws.WriteMessage(websocket.TextMessage,[]byte("local SSH connection failed"));return};defer raw.Close();raw.SetDeadline(time.Now().Add(10*time.Second));c,ch,req,e:=ssh.NewClientConn(raw,"127.0.0.1:22",config);if e!=nil{ws.WriteMessage(websocket.TextMessage,[]byte("SSH authentication failed"));return};raw.SetDeadline(time.Time{});client:=ssh.NewClient(c,ch,req);defer client.Close();session,e:=client.NewSession();if e!=nil{return};defer session.Close()
 if in.Cols<20||in.Cols>500{in.Cols=100};if in.Rows<5||in.Rows>200{in.Rows=30};if session.RequestPty("xterm-256color",in.Rows,in.Cols,ssh.TerminalModes{ssh.ECHO:1,ssh.TTY_OP_ISPEED:14400,ssh.TTY_OP_OSPEED:14400})!=nil{return}
 stdin,e:=session.StdinPipe();if e!=nil{return};var mu sync.Mutex;out:=socketWriter{ws,&mu};session.Stdout=out;session.Stderr=out;if session.Shell()!=nil{return};a.audit(u.Name,"terminal.connect",in.Username,"ok");defer a.audit(u.Name,"terminal.disconnect",in.Username,"closed")
 done:=make(chan struct{});cookie,_:=r.Cookie("session");go func(){ticker:=time.NewTicker(30*time.Second);defer ticker.Stop();for{select{case <-a.ctx.Done():ws.Close();client.Close();return;case <-done:return;case <-ticker.C:var active bool;var created int64;err:=a.db.QueryRow("SELECT (u.role='admin' OR (u.role='operator' AND u.terminal=1)),s.created FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND u.disabled=0",cookie.Value).Scan(&active,&created);if err!=nil||!active||time.Now().Unix()-created>int64(a.intSetting("session.maxHours",12)*3600){ws.Close();client.Close();return}}}}();defer close(done)
 go func(){session.Wait();ws.Close()}()
 lastTouch:=time.Now();for{ws.SetReadDeadline(time.Now().Add(time.Duration(a.intSetting("session.idleMinutes",30))*time.Minute));kind,p,e:=ws.ReadMessage();if e!=nil{return};if time.Since(lastTouch)>time.Minute{a.exec("UPDATE sessions SET seen=? WHERE token=?",time.Now().Unix(),cookie.Value);lastTouch=time.Now()};if kind==websocket.BinaryMessage{if _,e=stdin.Write(p);e!=nil{return}}else{var resize struct{Cols,Rows int};if decodeJSON(string(p),&resize)==nil&&resize.Cols>=20&&resize.Cols<=500&&resize.Rows>=5&&resize.Rows<=200{session.WindowChange(resize.Rows,resize.Cols)}}}
}
var _ io.Writer=socketWriter{}
