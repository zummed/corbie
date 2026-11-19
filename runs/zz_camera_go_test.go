package runs
import ("testing"; "github.com/zummed/corbie-ui/page/pagetest")
func TestZZCamera(t *testing.T) { b := newLab(t); pagetest.Run(t, pagetest.Config{Handler: b.handler, Page: "/run/" + b.lookup, Module: "runs/zz_camera_test.js"}) }
