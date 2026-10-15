package panel

import (
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha1"
 "crypto/subtle"
 "encoding/base32"
 "encoding/binary"
 "errors"
 "fmt"
 "net/http"
 "time"
 "golang.org/x/crypto/bcrypt"
)
func validTOTP(secret,code string,now time.Time)bool{
 key,err:=base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret);if err!=nil||len(code)!=6{return false}
 for d:=-1;d<=1;d++{var b [8]byte;binary.BigEndian.PutUint64(b[:],uint64(now.Unix()/30+int64(d)));h:=hmac.New(sha1.New,key);h.Write(b[:]);sum:=h.Sum(nil);o:=sum[19]&15;n:=binary.BigEndian.Uint32(sum[o:o+4])&0x7fffffff;if subtle.ConstantTimeCompare([]byte(fmt.Sprintf("%06d",n%1000000)),[]byte(code))==1{return true}}
 return false
}
func(a *App) totp(w http.ResponseWriter,r *http.Request,u User){
 if r.Method!="POST"{problem(w,405,errors.New("POST required"));return}
 var in struct{Action,Password,Secret,Code string};if !body(w,r,&in){return};var hash,current string;a.db.QueryRow("SELECT password,totp FROM users WHERE id=?",u.ID).Scan(&hash,&current)
 if bcrypt.CompareHashAndPassword([]byte(hash),[]byte(in.Password))!=nil{problem(w,403,errors.New("password verification failed"));return}
 if current!=""{plain,err:=a.unseal(current);if err!=nil{problem(w,500,err);return};current=string(plain)}
 switch in.Action{
 case "prepare":if current!=""{problem(w,409,errors.New("disable existing TOTP before enrolling another device"));return};b:=make([]byte,20);if _,err:=rand.Read(b);err!=nil{problem(w,500,err);return};secret:=base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b);sealed,err:=a.seal([]byte(encode(map[string]any{"secret":secret,"expires":time.Now().Add(5*time.Minute).Unix()})));if err!=nil{problem(w,500,err);return};if err=a.exec("INSERT OR REPLACE INTO settings VALUES(?,?)",fmt.Sprintf("totp-pending-%d",u.ID),sealed);err!=nil{problem(w,500,err);return};reply(w,200,map[string]string{"secret":secret});return
 case "enable":
 var raw string;a.db.QueryRow("SELECT value FROM settings WHERE key=?",fmt.Sprintf("totp-pending-%d",u.ID)).Scan(&raw);plain,_:=a.unseal(raw);var pending struct{Secret string;Expires int64};decodeJSON(string(plain),&pending)
 if current!=""||pending.Expires<time.Now().Unix()||!validTOTP(pending.Secret,in.Code,time.Now()){problem(w,400,errors.New("invalid or expired code"));return};sealed,err:=a.seal([]byte(pending.Secret));if err!=nil{problem(w,500,err);return};if err=a.exec("UPDATE users SET totp=? WHERE id=?",sealed,u.ID);err!=nil{problem(w,500,err);return}
 case "disable":if !validTOTP(current,in.Code,time.Now()){problem(w,403,errors.New("invalid code"));return};a.exec("UPDATE users SET totp='' WHERE id=?",u.ID)
 default:problem(w,400,errors.New("invalid action"));return
 };a.exec("DELETE FROM settings WHERE key=?",fmt.Sprintf("totp-pending-%d",u.ID));a.exec("DELETE FROM sessions WHERE user_id=?",u.ID);a.audit(u.Name,"totp",in.Action,"ok");reply(w,200,map[string]bool{"ok":true})
}
