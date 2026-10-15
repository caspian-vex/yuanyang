package panel

import (
 "context"
 "errors"
 "io"
 "net/http"
 "os"
 "path"
 "strings"
)

func safeName(p string)(string,error){p=strings.TrimPrefix(p,"/");if p==""{return ".",nil};if strings.Contains(p,"\\")||strings.ContainsRune(p,0)||path.IsAbs(p){return "",errors.New("invalid path")};for _,segment:=range strings.Split(p,"/"){if segment==".."{return "",errors.New("parent path traversal is forbidden")}};return path.Clean(p),nil}
func(a *App) files(w http.ResponseWriter,r *http.Request,u User){
 a.fileMu.RLock();defer a.fileMu.RUnlock()
 p,err:=safeName(r.URL.Query().Get("path"));if err!=nil{problem(w,400,err);return}
 if r.Method=="GET"{
 f,err:=openRead(a.root,p);if err!=nil{problem(w,400,err);return};defer f.Close();st,err:=f.Stat();if err!=nil{problem(w,400,err);return};if !st.IsDir()&&!st.Mode().IsRegular(){problem(w,400,errors.New("special files cannot be downloaded"));return}
 if !st.IsDir(){w.Header().Set("Content-Disposition","attachment; filename*=UTF-8''"+escapeName(st.Name()));http.ServeContent(w,r,st.Name(),st.ModTime(),f);return}
 limit,offset:=page(r);items:=[]map[string]any{};seen:=0
 for{entries,e:=f.ReadDir(128);for _,v:=range entries{if strings.HasPrefix(v.Name(),".")&&r.URL.Query().Get("hidden")!="true"{continue};if !strings.Contains(v.Name(),r.URL.Query().Get("q")){continue};if seen<offset{seen++;continue};info,err:=v.Info();if err!=nil{continue};items=append(items,map[string]any{"name":v.Name(),"directory":v.IsDir(),"size":info.Size(),"modified":info.ModTime(),"mode":info.Mode().String()});if len(items)==limit{reply(w,200,items);return}};if e!=nil{break}};reply(w,200,items);return
 }
 if r.Method=="PUT"{
 if p=="."{problem(w,400,errors.New("file path required"));return};select{case a.uploads<-struct{}{}:defer func(){<-a.uploads}();default:problem(w,429,errors.New("another upload is in progress"));return}
 // Stage inside the confined root; incomplete uploads never replace an existing file.
 temp:=path.Join(path.Dir(p),".upload-"+randomToken());f,err:=a.root.OpenFile(temp,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{problem(w,400,err);return};defer a.root.Remove(temp)
 r.Body=http.MaxBytesReader(w,r.Body,int64(a.intSetting("uploads.maxMB",100))<<20);_,err=io.CopyBuffer(f,r.Body,make([]byte,32<<10));if err==nil{err=f.Chmod(0644)};closeErr:=f.Close();if err==nil{err=closeErr};if err!=nil{problem(w,400,err);return}
 if _,err=a.root.Lstat(p);err==nil&&r.URL.Query().Get("overwrite")!="true"{problem(w,409,errors.New("file exists; explicit overwrite required"));return}
 // Rename is confined by os.Root, including symlink traversal.
 a.fileWrites.Lock();if r.URL.Query().Get("overwrite")=="true"{err=a.root.Rename(temp,p)}else{err=a.root.Link(temp,p)};a.fileWrites.Unlock();a.audit(u.Name,"file.upload",p,result(err));if err!=nil{problem(w,400,err);return};reply(w,201,map[string]bool{"ok":true});return
 }
 if r.Method!="POST"{w.WriteHeader(405);return}
 a.fileWrites.Lock();defer a.fileWrites.Unlock()
 var in struct{Action,Target string;Confirm bool};if !body(w,r,&in){return};if p=="."{problem(w,400,errors.New("cannot mutate root"));return}
 switch in.Action{
 case "mkdir":err=a.root.Mkdir(p,0755);if err==nil{err=a.root.Chmod(p,0755)}
 case "delete":if !in.Confirm{problem(w,400,errors.New("permanent deletion requires confirmation"));return};err=a.root.RemoveAll(p)
 case "move":var target string;target,err=safeName(in.Target);if err==nil{if _,e:=a.root.Lstat(target);!os.IsNotExist(e){err=errors.New("destination exists or is inaccessible")}else{err=a.root.Rename(p,target)}}
 case "copy":var target string;target,err=safeName(in.Target);if err==nil{err=a.copyFile(r.Context(),p,target)}
 default:err=errors.New("unsupported file action")
 };a.audit(u.Name,"file."+in.Action,p,result(err));if err!=nil{problem(w,400,err);return};reply(w,200,map[string]bool{"ok":true})
}
func(a *App) copyFile(ctx context.Context,src,dst string)error{if src==dst||strings.HasPrefix(dst,src+"/"){return errors.New("cannot copy into the source directory")};return a.copyEntry(ctx,src,dst,0,make([]byte,32<<10))}
func(a *App) copyEntry(ctx context.Context,src,dst string,depth int,buffer []byte)(err error){
 if err=ctx.Err();err!=nil{return err};if depth>64{return errors.New("directory copy depth exceeds 64")};st,err:=a.root.Lstat(src);if err!=nil{return err};if st.Mode()&os.ModeSymlink!=0{return errors.New("directory copy does not follow symbolic links")}
 if st.IsDir(){if err=a.root.Mkdir(dst,0700);err!=nil{return err};defer func(){if err!=nil{a.root.RemoveAll(dst)}}();if err=a.root.Chmod(dst,0700);err!=nil{return err};directory,e:=openRead(a.root,src);if e!=nil{return e};defer directory.Close();for{entries,e:=directory.ReadDir(128);for _,entry:=range entries{if err=a.copyEntry(ctx,path.Join(src,entry.Name()),path.Join(dst,entry.Name()),depth+1,buffer);err!=nil{return err}};if e==io.EOF{break};if e!=nil{return e}};return a.root.Chmod(dst,st.Mode().Perm())}
 if !st.Mode().IsRegular(){return errors.New("special files cannot be copied")};in,err:=openRead(a.root,src);if err!=nil{return err};defer in.Close();opened,err:=in.Stat();if err!=nil{return err};if !opened.Mode().IsRegular(){return errors.New("source file changed type")};out,err:=a.root.OpenFile(dst,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{return err};_,err=io.CopyBuffer(out,contextReader{ctx,in},buffer);if err==nil{err=out.Chmod(st.Mode().Perm())};closeErr:=out.Close();if err==nil{err=closeErr};if err!=nil{a.root.Remove(dst)};return err
}
type contextReader struct{ctx context.Context;r io.Reader}
func(c contextReader)Read(p []byte)(int,error){if err:=c.ctx.Err();err!=nil{return 0,err};return c.r.Read(p)}
