package panel

import (
 "context"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rand"
 "crypto/tls"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/pem"
 "errors"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"

 "golang.org/x/crypto/acme"
)
type Certificate struct{Name string `json:"name"`;Domains []string `json:"domains"`;Expires time.Time `json:"expires"`;Issuer string `json:"issuer"`;Automatic bool `json:"automatic"`;Email string `json:"email"`}
func validateCertificate(cert,key string)(*x509.Certificate,error){pair,err:=tls.X509KeyPair([]byte(cert),[]byte(key));if err!=nil{return nil,err};c,err:=x509.ParseCertificate(pair.Certificate[0]);if err!=nil{return nil,err};if time.Now().Before(c.NotBefore)||!time.Now().Before(c.NotAfter){return nil,errors.New("certificate is not currently valid")};return c,nil}
func installCertificate(in HelperRequest)error{
 if in.Action=="delete"{if !siteName.MatchString(in.Name){return errors.New("invalid name")};files,_:=filepath.Glob("/etc/nginx/conf.d/yuanyang-*.conf");files=append(files,gatewayConfig);for _,p:=range files{if strings.Contains(readFile(p),"/etc/yuanyang/certificates/"+in.Name+".crt"){return errors.New("certificate is referenced by a site")}};for _,suffix:=range []string{".crt",".key"}{if err:=os.Remove("/etc/yuanyang/certificates/"+in.Name+suffix);err!=nil&&!os.IsNotExist(err){return err}};return nil}
 if !siteName.MatchString(in.Name){return errors.New("invalid certificate name")};if _,err:=validateCertificate(in.Certificate,in.PrivateKey);err!=nil{return err};dir:="/etc/yuanyang/certificates";if err:=os.MkdirAll(dir,0700);err!=nil{return err};certPath:=filepath.Join(dir,in.Name+".crt");keyPath:=filepath.Join(dir,in.Name+".key");oldCert,certErr:=os.ReadFile(certPath);oldKey,keyErr:=os.ReadFile(keyPath)
 restore:=func(){if certErr==nil{writeAtomic(certPath,oldCert,0644)}else{os.Remove(certPath)};if keyErr==nil{writeAtomic(keyPath,oldKey,0600)}else{os.Remove(keyPath)}}
 if err:=writeAtomic(keyPath,[]byte(in.PrivateKey),0600);err!=nil{return err};if err:=writeAtomic(certPath,[]byte(in.Certificate),0644);err!=nil{restore();return err}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel();if _,err:=execCombined(ctx,"/usr/sbin/nginx","-t");err!=nil{restore();return err};if _,err:=execCombined(ctx,"/usr/bin/systemctl","reload","nginx");err!=nil{restore();execCombined(ctx,"/usr/bin/systemctl","reload","nginx");return err};return nil
}
func(a *App) certificates(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="GET"{files,_:=filepath.Glob(filepath.Join(a.data,"certificates","*.json"));items:=[]Certificate{};for _,p:=range files{if len(items)>=200{break};var c Certificate;if decodeJSON(readFile(p),&c)==nil{items=append(items,c)}};reply(w,200,items);return}
 if r.Method!="POST"&&r.Method!="DELETE"{w.WriteHeader(405);return};var in struct{Name,Certificate,PrivateKey,Action,Email string;Agree bool};if !body(w,r,&in){return};if !siteName.MatchString(in.Name){problem(w,400,errors.New("invalid site name"));return}
 a.mutations.Lock();defer a.mutations.Unlock()
 if r.Method=="DELETE"{out,err:=a.privileged(r.Context(),HelperRequest{Operation:"certificate",Name:in.Name,Action:"delete"});if err==nil{err=os.Remove(filepath.Join(a.data,"certificates",in.Name+".json"));if os.IsNotExist(err){err=nil}};a.audit(u.Name,"certificate.delete",in.Name,result(err));if err!=nil{problem(w,400,fmt.Errorf("%s: %w",out,err));return};reply(w,200,map[string]bool{"ok":true});return}
 var raw string;if a.db.QueryRow("SELECT domains FROM sites WHERE name=?",in.Name).Scan(&raw)!=nil{problem(w,404,errors.New("create a site first"));return};var domains []string;decodeJSON(raw,&domains)
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Minute);defer cancel();cert,key:=in.Certificate,in.PrivateKey;var err error
 if in.Action=="issue"{if !in.Agree||in.Email==""{problem(w,400,errors.New("ACME contact email and terms agreement required"));return};cert,key,err=a.obtain(ctx,domains,in.Email)}else if in.Action!="import"{problem(w,400,errors.New("invalid action"));return}
 if err==nil{err=a.saveCertificate(ctx,in.Name,cert,key,in.Action=="issue",in.Email,domains)};a.audit(u.Name,"certificate."+in.Action,in.Name,result(err));if err!=nil{problem(w,400,err);return};reply(w,200,map[string]bool{"ok":true})
}
func(a *App) saveCertificate(ctx context.Context,name,cert,key string,automatic bool,email string,domains []string)error{
 parsed,err:=validateCertificate(cert,key);if err!=nil{return err};for _,d:=range domains{if err=parsed.VerifyHostname(d);err!=nil{return err}}
 out,err:=a.privileged(ctx,HelperRequest{Operation:"certificate",Name:name,Certificate:cert,PrivateKey:key});if err!=nil{return fmt.Errorf("%s: %w",out,err)}
 dir:=filepath.Join(a.data,"certificates");if err=os.MkdirAll(dir,0700);err!=nil{return err};c:=Certificate{Name:name,Domains:parsed.DNSNames,Expires:parsed.NotAfter,Issuer:parsed.Issuer.CommonName,Automatic:automatic,Email:email};if err=writeAtomic(filepath.Join(dir,name+".json"),[]byte(encode(c)),0600);err!=nil{return err};a.alertState("certificate:"+name,"Certificate valid until "+c.Expires.String(),false);return nil
}
func(a *App) obtain(ctx context.Context,domains []string,email string)(string,string,error){
 accountPath:=filepath.Join(a.data,"acme-account");var accountKey *ecdsa.PrivateKey
 encrypted,err:=os.ReadFile(accountPath);if os.IsNotExist(err){accountKey,err=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);if err!=nil{return "","",err};der,_:=x509.MarshalECPrivateKey(accountKey);s,e:=a.seal(der);if e!=nil{return "","",e};if e=writeAtomic(accountPath,[]byte(s),0600);e!=nil{return "","",e}}else if err==nil{der,e:=a.unseal(string(encrypted));if e!=nil{return "","",e};accountKey,err=x509.ParseECPrivateKey(der)};if err!=nil{return "","",err}
 directory:=os.Getenv("YUANYANG_ACME_DIRECTORY");if directory==""{directory=acme.LetsEncryptURL};if directory!=acme.LetsEncryptURL&&directory!="https://acme-staging-v02.api.letsencrypt.org/directory"{return "","",errors.New("unsupported ACME directory")};client:=&acme.Client{Key:accountKey,DirectoryURL:directory,HTTPClient:&http.Client{Timeout:30*time.Second}}
 _,err=client.Register(ctx,&acme.Account{Contact:[]string{"mailto:"+email}},acme.AcceptTOS);if err!=nil{if !errors.Is(err,acme.ErrAccountAlreadyExists){return "","",err};if _,err=client.GetReg(ctx,"");err!=nil{return "","",err}}
 ids:=make([]acme.AuthzID,len(domains));for i,d:=range domains{ids[i]=acme.AuthzID{Type:"dns",Value:d}}
 order,err:=client.AuthorizeOrder(ctx,ids);if err!=nil{return "","",err};challengeDir:=filepath.Join(a.data,"acme",".well-known","acme-challenge");if err=os.MkdirAll(challengeDir,0755);err!=nil{return "","",err}
 for _,uri:=range order.AuthzURLs{auth,err:=client.GetAuthorization(ctx,uri);if err!=nil{return "","",err};if auth.Status==acme.StatusValid{continue};var challenge *acme.Challenge;for _,c:=range auth.Challenges{if c.Type=="http-01"{challenge=c;break}};if challenge==nil{return "","",errors.New("HTTP-01 challenge unavailable")};if challenge.Token==""||len(challenge.Token)>256||strings.ContainsAny(challenge.Token,"./\\"){return "","",errors.New("invalid ACME challenge token")};response,err:=client.HTTP01ChallengeResponse(challenge.Token);if err!=nil{return "","",err};p:=filepath.Join(challengeDir,challenge.Token);if filepath.Base(p)!=challenge.Token{return "","",errors.New("invalid ACME token")};if err=writeAtomic(p,[]byte(response),0644);err!=nil{return "","",err};defer os.Remove(p);if _,err=client.Accept(ctx,challenge);err!=nil{return "","",err};if _,err=client.WaitAuthorization(ctx,uri);err!=nil{return "","",err}}
 order,err=client.WaitOrder(ctx,order.URI);if err!=nil{return "","",err};key,err:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);if err!=nil{return "","",err};csr,err:=x509.CreateCertificateRequest(rand.Reader,&x509.CertificateRequest{Subject:pkix.Name{CommonName:domains[0]},DNSNames:domains},key);if err!=nil{return "","",err};chain,_,err:=client.CreateOrderCert(ctx,order.FinalizeURL,csr,true);if err!=nil{return "","",err};var cert []byte;for _,der:=range chain{cert=append(cert,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:der})...)};der,_:=x509.MarshalECPrivateKey(key);return string(cert),string(pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY",Bytes:der})),nil
}
func(a *App) renewCertificates(){
 timer:=time.NewTimer(time.Minute);defer timer.Stop();for{select{case <-a.ctx.Done():return;case <-timer.C:
 files,_:=filepath.Glob(filepath.Join(a.data,"certificates","*.json"));for _,p:=range files{var c Certificate;if decodeJSON(readFile(p),&c)!=nil{continue};if time.Until(c.Expires)>30*24*time.Hour{continue};if !c.Automatic{a.alertState("certificate:"+c.Name,"Certificate expires: "+c.Expires.String(),true);continue};a.mutations.Lock();ctx,cancel:=context.WithTimeout(a.ctx,5*time.Minute);cert,key,err:=a.obtain(ctx,c.Domains,c.Email);if err==nil{err=a.saveCertificate(ctx,c.Name,cert,key,true,c.Email,c.Domains)};cancel();a.mutations.Unlock();a.audit("system","certificate.renew",c.Name,result(err));a.alertState("certificate:"+c.Name,"Certificate renewal: "+result(err),err!=nil)}
 timer.Reset(24*time.Hour)
 }}}
