package panel

import (
 "context"
 "os"
 "os/exec"
 "strings"
 "time"
)
func(a *App) taskCommand(ctx context.Context,t Task)(string,error){
 select{case a.commands<-struct{}{}:defer func(){<-a.commands}();case <-ctx.Done():return "",ctx.Err()}
 exe,err:=os.Executable();if err!=nil{return "",err};cmd:=exec.CommandContext(ctx,"sudo","-n",exe,"helper");cmd.Stdin=strings.NewReader(encode(HelperRequest{Operation:"task",Name:t.Account,Config:t.Command,Timeout:t.Timeout,Workdir:t.Workdir,TaskID:t.ID}));cmd.WaitDelay=2*time.Second;out:=&cappedOutput{limit:64<<10};cmd.Stdout=out;cmd.Stderr=out;err=cmd.Run();return out.String(),err
}
