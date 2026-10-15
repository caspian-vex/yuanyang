package panel

import (
 "errors"
 "net/http"
 "time"
 "golang.org/x/crypto/bcrypt"
)
func(a *App) verify(u User,password,code string)bool{var hash,secret string;if a.db.QueryRow("SELECT password,totp FROM users WHERE id=?",u.ID).Scan(&hash,&secret)!=nil{return false};if bcrypt.CompareHashAndPassword([]byte(hash),[]byte(password))!=nil{return false};if secret!=""{plain,err:=a.unseal(secret);if err!=nil||!validTOTP(string(plain),code,time.Now()){return false}};return true}
func(a *App) reboot(w http.ResponseWriter,r *http.Request,u User){var in struct{Password,Code string;Confirm bool};if !body(w,r,&in){return};if !in.Confirm||!a.verify(u,in.Password,in.Code){problem(w,403,errors.New("confirmation and identity verification required"));return};out,err:=a.privileged(r.Context(),HelperRequest{Operation:"reboot"});a.audit(u.Name,"server.reboot","localhost",result(err));if err!=nil{problem(w,400,errors.New(out));return};reply(w,202,map[string]bool{"scheduled":true})}
func(a *App) password(w http.ResponseWriter,r *http.Request,u User){var in struct{Password,Code,NewPassword string};if !body(w,r,&in){return};if !a.verify(u,in.Password,in.Code){problem(w,403,errors.New("identity verification failed"));return};if len(in.NewPassword)<12||len(in.NewPassword)>72{problem(w,400,errors.New("password must be 12–72 bytes"));return};hash,err:=bcrypt.GenerateFromPassword([]byte(in.NewPassword),bcrypt.DefaultCost);if err==nil{err=a.exec("UPDATE users SET password=? WHERE id=?",string(hash),u.ID)};if err!=nil{problem(w,500,err);return};a.exec("DELETE FROM sessions WHERE user_id=?",u.ID);a.audit(u.Name,"password.change",u.Name,"ok");reply(w,200,map[string]bool{"ok":true})}
func(a *App) sessions(w http.ResponseWriter,r *http.Request,u User){
 if r.Method=="POST"{var in struct{All bool};if !body(w,r,&in){return};a.exec("DELETE FROM sessions WHERE user_id=?",u.ID);a.audit(u.Name,"sessions.revoke",u.Name,"ok");reply(w,200,map[string]bool{"ok":true});return}
 if r.Method!="GET"{w.WriteHeader(405);return};rows,err:=a.db.Query("SELECT created,seen FROM sessions WHERE user_id=? ORDER BY created DESC LIMIT 100",u.ID);if err!=nil{problem(w,500,err);return};defer rows.Close();items:=[]map[string]int64{};for rows.Next(){var created,seen int64;rows.Scan(&created,&seen);items=append(items,map[string]int64{"created":created,"seen":seen})};reply(w,200,items)
}
