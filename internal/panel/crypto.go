package panel

import (
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/base64"
 "errors"
 "os"
 "path/filepath"
)
func(a *App) key()([]byte,error){path:=filepath.Join(a.data,"secret.key");b,err:=os.ReadFile(path);if os.IsNotExist(err){b=make([]byte,32);if _,err=rand.Read(b);err!=nil{return nil,err};f,e:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return nil,e};_,err=f.Write(b);ce:=f.Close();if err==nil{err=ce}};if err!=nil{return nil,err};if len(b)!=32{return nil,errors.New("invalid encryption key")};return b,nil}
func(a *App) seal(b []byte)(string,error){key,err:=a.key();if err!=nil{return "",err};block,err:=aes.NewCipher(key);if err!=nil{return "",err};gcm,_:=cipher.NewGCM(block);nonce:=make([]byte,gcm.NonceSize());if _,err=rand.Read(nonce);err!=nil{return "",err};return base64.StdEncoding.EncodeToString(gcm.Seal(nonce,nonce,b,nil)),nil}
func(a *App) unseal(s string)([]byte,error){key,err:=a.key();if err!=nil{return nil,err};b,err:=base64.StdEncoding.DecodeString(s);if err!=nil{return nil,err};block,_:=aes.NewCipher(key);gcm,_:=cipher.NewGCM(block);n:=gcm.NonceSize();if len(b)<n{return nil,errors.New("invalid ciphertext")};return gcm.Open(nil,b[:n],b[n:],nil)}
