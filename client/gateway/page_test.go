package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/client/gateway"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/store"
)

// A page in a browser calls the gateway from any origin: the browser's
// question before the call is answered with no token, each answer may be
// read by the page, and a call without a token is refused as any is.
func Test_client_gateway_answers_a_page_of_any_origin(t *testing.T) {
	s := newSeveral(t)
	editor := s.enrol("editor", "at:/**")
	server := httptest.NewServer(&gateway.Gateway{Callers: map[string]*client.Client{"alices": editor}, Interval: 10 * time.Millisecond})
	t.Cleanup(server.Close)

	ask := func(method, path, token string, headers map[string]string) (*http.Response, string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "https://app.example.com")
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	// The browser's question before a call with a token and JSON.
	asked, body := ask(http.MethodOptions, "/runs", "", map[string]string{
		"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "authorization,content-type"})
	if asked.StatusCode != http.StatusNoContent || body != "" {
		t.Fatalf("the question before a call was answered %d %q, not 204 with nothing", asked.StatusCode, body)
	}
	for name, want := range map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Authorization, Content-Type",
	} {
		if got := asked.Header.Get(name); got != want {
			t.Errorf("the question before a call was answered with %s %q, not %q", name, got, want)
		}
	}

	// A call with the token is answered, and the page may read the answer.
	shown, body := ask(http.MethodGet, "/client", "alices", nil)
	if shown.StatusCode != http.StatusOK || !strings.Contains(body, `"client":"editor"`) {
		t.Fatalf("a page's call with a token was answered %d %s", shown.StatusCode, body)
	}
	if got := shown.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("the answer to a page's call has Access-Control-Allow-Origin %q, not *", got)
	}

	// A call with no token is refused, and the page may read why. A cookie
	// is no token.
	refused, body := ask(http.MethodGet, "/client", "", map[string]string{"Cookie": "token=alices; Authorization=alices"})
	if refused.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "does not show a token") {
		t.Fatalf("a page's call with a cookie and no token was answered %d %s, not 401", refused.StatusCode, body)
	}
	if got := refused.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("the refusal of a page's call has Access-Control-Allow-Origin %q, not *", got)
	}
	// The answer permits no cookie to be sent.
	if got := refused.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("the gateway answered Access-Control-Allow-Credentials %q", got)
	}
}

// A request whose own time ends while the controller has yet to answer, as
// a serverless host's call does, is told so.
func Test_client_gateway_says_when_a_requests_time_ended(t *testing.T) {
	s := newSeveral(t)
	// A client of a controller that takes nothing.
	id, err := identity.New("lonely", identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := identity.New("absent", identity.Controller)
	if err != nil {
		t.Fatal(err)
	}
	lonely := client.New(id, absent.Card, s.local.Memory.As(store.Party{Kind: store.Client, Name: "lonely", Controller: "absent"}))
	g := &gateway.Gateway{Callers: map[string]*client.Client{"lonelys": lonely}, Interval: 10 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/values", strings.NewReader(`{"act": "set", "at": "/shop", "name": "REGION", "value": "here"}`))
	req.Header.Set("Authorization", "Bearer lonelys")
	req.Header.Set("Content-Type", "application/json")
	answer := httptest.NewRecorder()
	g.ServeHTTP(answer, req)
	if answer.Code != http.StatusGatewayTimeout || !strings.Contains(answer.Body.String(), "the request's time ended before the controller answered") {
		t.Fatalf("a request whose time ended was answered %d %s", answer.Code, answer.Body.String())
	}
}
