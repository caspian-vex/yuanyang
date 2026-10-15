package panel

import (
 "crypto/rand"
 "database/sql"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"

 "golang.org/x/crypto/bcrypt"
)

// InitializeAdmin is an offline command. Run as the service user while stopped.
// Existing accounts are never changed; the generated password is only printed once.
func InitializeAdmin(data string, output io.Writer) error {
 if err := os.MkdirAll(data,0700); err != nil { return err }
 db,err := sql.Open("sqlite",filepath.Join(data,"panel.db"))
 if err != nil { return err }; defer db.Close()
 db.SetMaxOpenConns(1)
 if _,err = db.Exec(initialSchema); err != nil { return err }
 if err = os.Chmod(filepath.Join(data,"panel.db"),0600); err != nil { return err }
 tx,err := db.Begin(); if err != nil { return err }; defer tx.Rollback()
 var count int
 if err = tx.QueryRow("SELECT count(*) FROM users").Scan(&count); err != nil { return err }
 if count > 0 {
  var name string
  if err = tx.QueryRow("SELECT name FROM users WHERE role='admin' ORDER BY id LIMIT 1").Scan(&name); err != nil { return errors.New("existing database has no administrator; use local recovery") }
  _,err = fmt.Fprintf(output,"账号：%s\n密码：保留原密码（升级不会重置）\n",name)
  return err
 }
 var random [20]byte
 if _,err = rand.Read(random[:]); err != nil { return err }
 password := hex.EncodeToString(random[:])
 hash,err := bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost); if err != nil { return err }
 if _,err = tx.Exec("INSERT INTO users(name,password,role) VALUES('admin',?,'admin')",string(hash)); err != nil { return err }
 if err = tx.Commit(); err != nil { return err }
 // Remove credentials from the former browser-based initialization flow.
 if err = os.Remove(filepath.Join(data,"setup-token")); err != nil && !os.IsNotExist(err) { return err }
 _,err = fmt.Fprintf(output,"账号：admin\n密码：%s\n请保存此密码；登录后可在个人账号中修改。\n",password)
 return err
}
