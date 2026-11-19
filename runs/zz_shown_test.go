package runs

import (
	"context"
	"encoding/json/v2"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestZZShownUnfolded(t *testing.T) {
	b := newLab(t)
	srv := httptest.NewServer(b.handler)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/run/"+b.lookup+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m struct {
			ID   string
			Data *shown
		}
		json.Unmarshal(data, &m)
		if m.ID != "shown" || m.Data == nil {
			continue
		}
		var in []string
		for _, st := range m.Data.Drawing.Flow.Stages {
			if st.Card.In != nil {
				in = append(in, st.Card.Name)
			}
		}
		t.Logf("%s shown row %s %s unfolded %v", time.Now().Format("05.000"), m.Data.Row.Run.Machine, m.Data.Row.Key, in)
	}
}
