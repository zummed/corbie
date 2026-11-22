package role

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// On a serverless host an HTTP server is a function the host calls once for
// each request: the call is the request, and what the function gives back is
// the answer. The host sends the answer on to the caller as it is written,
// so a stream is a stream there too.

// Answer answers the host's HTTP requests until the context ends: each call
// is one request, given to the handler, whose context ends a margin before
// the host's limit. report is told of a call that could not be answered.
func Answer(ctx context.Context, report func(error), handler http.Handler) error {
	return answerLambda(ctx, "http://"+os.Getenv(lambdaAPI), report, handler)
}

// called is an HTTP request as AWS Lambda gives one that came to a
// function's URL.
type called struct {
	RawPath         string            `json:"rawPath"`
	RawQueryString  string            `json:"rawQueryString"`
	Headers         map[string]string `json:"headers"`
	Cookies         []string          `json:"cookies"`
	Body            string            `json:"body"`
	IsBase64Encoded bool              `json:"isBase64Encoded"`
	RequestContext  struct {
		HTTP struct {
			Method string `json:"method"`
		} `json:"http"`
	} `json:"requestContext"`
}

func answerLambda(ctx context.Context, api string, report func(error), handler http.Handler) error {
	// A call can be long in coming, and an answer long in the writing: no
	// time limit. An answer's connection is closed when it is written, which
	// is how the host knows a streamed answer is whole.
	client := &http.Client{}
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/2018-06-01/runtime/invocation/next", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("the host could not be asked for its next call: %w", err)
		}
		event, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		id := resp.Header.Get("Lambda-Runtime-Aws-Request-Id")
		if err != nil || resp.StatusCode != http.StatusOK || id == "" {
			return fmt.Errorf("the host answered %d to being asked for its next call", resp.StatusCode)
		}
		ms, err := strconv.ParseInt(resp.Header.Get("Lambda-Runtime-Deadline-Ms"), 10, 64)
		if err != nil {
			return fmt.Errorf("the host did not say until when its call may run")
		}
		if err := answerOne(ctx, client, api+"/2018-06-01/runtime/invocation/"+id+"/response", time.UnixMilli(ms), event, handler); err != nil && report != nil {
			report(err)
		}
	}
	return nil
}

// answerOne gives one call to the handler and sends what it writes to the
// host as it is written.
func answerOne(ctx context.Context, client *http.Client, to string, until time.Time, event []byte, handler http.Handler) error {
	reader, writer := io.Pipe()
	post, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, to, reader)
	if err != nil {
		return err
	}
	post.Header.Set("Lambda-Runtime-Function-Response-Mode", "streaming")
	post.Header.Set("Content-Type", "application/vnd.awslambda.http-integration-response")
	post.TransferEncoding = []string{"chunked"}
	post.Close = true
	sent := make(chan error, 1)
	go func() {
		resp, err := client.Do(post)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				err = fmt.Errorf("the host answered %d to the answer", resp.StatusCode)
			}
		}
		reader.CloseWithError(err) // a write after the host is gone fails
		sent <- err
	}()
	a := &answer{header: http.Header{}, to: writer}

	var c called
	err = json.Unmarshal(event, &c)
	body := []byte(c.Body)
	if err == nil && c.IsBase64Encoded {
		body, err = base64.StdEncoding.DecodeString(c.Body)
	}
	var problem error
	switch {
	case err != nil || c.RequestContext.HTTP.Method == "":
		// The function was called by something other than its URL.
		problem = fmt.Errorf("a call that is not an HTTP request was given to an HTTP server: its function is called through its function URL")
		http.Error(a, problem.Error(), http.StatusBadRequest)
	default:
		at := c.RawPath
		if c.RawQueryString != "" {
			at += "?" + c.RawQueryString
		}
		left := max(time.Until(until), 0)
		within, cancel := context.WithDeadline(ctx, until.Add(-min(HostMargin, left/2)))
		r, err := http.NewRequestWithContext(within, c.RequestContext.HTTP.Method, at, strings.NewReader(string(body)))
		if err != nil {
			http.Error(a, "the request could not be read", http.StatusBadRequest)
			cancel()
			break
		}
		for name, value := range c.Headers {
			r.Header.Set(name, value)
		}
		if len(c.Cookies) > 0 {
			r.Header.Set("Cookie", strings.Join(c.Cookies, "; "))
		}
		r.Host = c.Headers["host"]
		r.RequestURI = at
		handler.ServeHTTP(a, r)
		cancel()
	}
	a.WriteHeader(http.StatusOK)
	writer.Close()
	if err := <-sent; err != nil {
		return fmt.Errorf("the host could not be answered: %w", err)
	}
	return problem
}

// An answer is what a handler writes, sent to the host as it is written:
// first the status and the headers, then eight zero bytes, then the body.
type answer struct {
	header http.Header
	to     *io.PipeWriter
	begun  bool
}

func (a *answer) Header() http.Header { return a.header }

func (a *answer) WriteHeader(status int) {
	if a.begun {
		return
	}
	a.begun = true
	headers := map[string]string{}
	for name, values := range a.header {
		// The host sets the length of what it sends on itself.
		if name != "Content-Length" {
			headers[name] = strings.Join(values, ", ")
		}
	}
	first, _ := json.Marshal(map[string]any{"statusCode": status, "headers": headers}, json.Deterministic(true))
	a.to.Write(append(first, make([]byte, 8)...))
}

func (a *answer) Write(p []byte) (int, error) {
	a.WriteHeader(http.StatusOK)
	return a.to.Write(p)
}

// Flush does nothing: each write is sent as it is made.
func (a *answer) Flush() {}
