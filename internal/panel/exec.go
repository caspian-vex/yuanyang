package panel

import (
 "context"
 "os/exec"
 "os"
 "time"
)
func execCombined(ctx context.Context,name string,args ...string)(string,error){cmd:=exec.CommandContext(ctx,name,args...);cmd.Env=append(os.Environ(),"LC_ALL=C","LANG=C");cmd.WaitDelay=2*time.Second;out:=&cappedOutput{limit:64<<10};cmd.Stdout=out;cmd.Stderr=out;err:=cmd.Run();return out.String(),err}
