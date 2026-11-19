package runs
import ("net/http"; "os"; "testing"; "time")
func TestZZServe(t *testing.T) {
	b := newLab(t)
	srv := &http.Server{Addr: "127.0.0.1:8844", Handler: b.handler}
	go srv.ListenAndServe()
	os.WriteFile(os.Getenv("ZZ_IDS"), []byte(b.lookup+"\n"+b.review), 0o644)
	time.Sleep(4 * time.Minute)
}
