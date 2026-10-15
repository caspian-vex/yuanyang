package panel

import (
 "bytes"
 "context"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func testApp(t *testing.T)*App{t.Helper();ctx,cancel:=context.WithCancel(context.Background());root:=t.TempDir();app,err:=New(ctx,t.TempDir(),root,t.TempDir());if err!=nil{t.Fatal(err)};t.Cleanup(func(){cancel();app.Close()});return app}
func TestPathConfinement(t *testing.T){for _,p:=range []string{"../etc/passwd","a/../b","a/..","a\\..\\b","a\x00b"}{if _,err:=safeName(p);err==nil{t.Errorf("accepted unsafe path %q",p)}};app:=testApp(t);outside:=t.TempDir();if err:=os.WriteFile(filepath.Join(outside,"secret"),[]byte("secret"),0600);err!=nil{t.Fatal(err)};if err:=os.Symlink(outside,filepath.Join(app.rootPath,"escape"));err!=nil{t.Skip("symlink unavailable")};if f,err:=app.root.Open("escape/secret");err==nil{f.Close();t.Fatal("escaped configured root")}}
func TestNginxInputValidation(t *testing.T){valid:=Site{Name:"example",Domains:[]string{"example.com"},Kind:"static",Root:"/var/www/example",Enabled:true};if err:=validateSite(valid);err!=nil{t.Fatal(err)};bad:=valid;bad.Root="/var/www/x; include /etc/passwd;";if validateSite(bad)==nil{t.Fatal("accepted configuration injection")};bad=valid;bad.Domains=[]string{"example.com;"};if validateSite(bad)==nil{t.Fatal("accepted domain injection")};valid.HTTPS=true;valid.Redirect=true;conf:=renderSite(valid);if !strings.Contains(conf,"listen 443 ssl")||!strings.Contains(conf,"return 301")||!strings.Contains(conf,"acme-challenge"){t.Fatal(conf)}}
func TestOutputBound(t *testing.T){out:=&cappedOutput{limit:1024};data:=bytes.Repeat([]byte("x"),2000);n,err:=out.Write(data);if n!=len(data)||err!=nil||out.b.Len()!=1024||!out.truncated{t.Fatal("output bound failed")}}
func TestEncryption(t *testing.T){app:=testApp(t);ciphertext,err:=app.seal([]byte("secret"));if err!=nil{t.Fatal(err)};if strings.Contains(ciphertext,"secret"){t.Fatal("plaintext leaked")};plain,err:=app.unseal(ciphertext);if err!=nil||string(plain)!="secret"{t.Fatal("round trip failed",err)};if _,err=app.unseal("broken");err==nil{t.Fatal("accepted invalid ciphertext")}}
func TestTOTP(t *testing.T){secret:="GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ";if !validTOTP(secret,"287082",time.Unix(59,0)){t.Fatal("RFC 6238 vector failed")};if validTOTP(secret,"000000",time.Unix(59,0)){t.Fatal("accepted invalid TOTP")}}
func TestGatewayConfigUsesManagedCertificate(t *testing.T){conf:=renderGateway(GatewayState{Domain:"panel.example.com",Port:8443,CertificateName:"panel-cert"});for _,part:=range []string{"listen 8443 ssl","server_name panel.example.com","/etc/yuanyang/certificates/panel-cert.crt","proxy_request_buffering off","X-Real-IP $remote_addr"}{if !strings.Contains(conf,part){t.Errorf("missing %q",part)}}}
func TestQueueBoundAndOverlap(t *testing.T){app:=&App{jobs:make(chan Task,1),running:map[int64]bool{123:true}};if app.enqueue(Task{ID:123})==nil{t.Fatal("overlap accepted")};if err:=app.enqueue(Task{ID:124});err!=nil{t.Fatal(err)};if app.enqueue(Task{ID:125})==nil{t.Fatal("unbounded queue")}}
func TestAuthAndCSRF(t *testing.T){app:=testApp(t);h:=app.Handler();req:=httptest.NewRequest("GET","/api/metrics",nil);w:=httptest.NewRecorder();h.ServeHTTP(w,req);if w.Code!=401{t.Fatalf("unauthenticated status: %d",w.Code)};if err:=app.exec("INSERT INTO users(id,name,password,role) VALUES(1,'reader','','viewer')");err!=nil{t.Fatal(err)};now:=time.Now().Unix();if err:=app.exec("INSERT INTO sessions VALUES('test',1,'csrf',?,?)",now,now);err!=nil{t.Fatal(err)}
 req=httptest.NewRequest("POST","/api/files",strings.NewReader(`{}`));req.AddCookie(&http.Cookie{Name:"session",Value:"test"});w=httptest.NewRecorder();h.ServeHTTP(w,req);if w.Code!=403{t.Fatal("missing CSRF accepted")};req=httptest.NewRequest("POST","/api/files",strings.NewReader(`{}`));req.AddCookie(&http.Cookie{Name:"session",Value:"test"});req.Header.Set("X-CSRF-Token","csrf");w=httptest.NewRecorder();h.ServeHTTP(w,req);if w.Code!=403{t.Fatal("viewer mutation accepted")}}
