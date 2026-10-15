package panel

import (
 "database/sql"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "golang.org/x/crypto/bcrypt"
)
func Recover(data string,input io.Reader)error{
 if os.Geteuid()!=0{return errors.New("recovery requires root")};b,err:=io.ReadAll(io.LimitReader(input,100));if err!=nil{return err};password:=strings.TrimSpace(string(b));if len(password)<12||len(password)>72{return errors.New("password must be 12–72 bytes")};hash,err:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost);if err!=nil{return err};db,err:=sql.Open("sqlite",filepath.Join(data,"panel.db"));if err!=nil{return err};defer db.Close();tx,err:=db.Begin();if err!=nil{return err};defer tx.Rollback();res,err:=tx.Exec("UPDATE users SET password=?,totp='',disabled=0 WHERE id=(SELECT min(id) FROM users WHERE role='admin')",string(hash));if err!=nil{return err};n,_:=res.RowsAffected();if n!=1{return errors.New("no administrator found")};if _,err=tx.Exec("DELETE FROM sessions; DELETE FROM settings WHERE key IN ('security.allowIPs','files.root')");err!=nil{return err};return tx.Commit()
}
