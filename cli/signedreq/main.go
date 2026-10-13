package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/x64c/gwf/auth/jwtassert"
)

// args is every input the tool takes, in one place, so -conf and the command
// line speak the same vocabulary: each field's json name is its flag name.
type args struct {
	App     string   `json:"app"`     // app root whose config/.web-authn-jwtassert.json states aud/max_age/id
	Client  string   `json:"client"`  // client name (the conf's key) to sign as
	ID      string   `json:"id"`      // client id, when -app is not reachable
	Aud     string   `json:"aud"`     // audience, when -app is not reachable
	MaxAge  int      `json:"max_age"` // seconds, when -app is not reachable
	Kid     string   `json:"kid"`     // key id; must match the server's <kid>_public.pem
	Key     string   `json:"key"`     // private key PEM path
	Base    string   `json:"base"`    // scheme://host to send to; default: the audience
	Headers []string `json:"headers"` // extra "Name: value" request headers
}

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(v string) error {
	if !strings.Contains(v, ":") {
		return fmt.Errorf("header must be \"Name: value\", got %q", v)
	}
	*h = append(*h, v)
	return nil
}

func main() {
	var (
		a        args
		confPath = flag.String("conf", "", "JSON file supplying any of the flags below (same names, without the dash); an explicit flag wins")
		body     = flag.String("d", "", "request body; @path reads a file")
		signOnly = flag.Bool("sign-only", false, "print the Authorization value for this exact request and send nothing")
		showRes  = flag.Bool("i", false, "print response status and headers to stderr")
		verbose  = flag.Bool("v", false, "print what was signed (never the client id, never the key)")
		headers  headerFlags
	)
	flag.StringVar(&a.App, "app", "", "app root holding config/.web-authn-jwtassert.json — read for id/audience/max_age so they cannot drift from what the server enforces")
	flag.StringVar(&a.Client, "client", "", "client name in that conf to sign as")
	flag.StringVar(&a.ID, "id", "", "client id, instead of -app/-client")
	flag.StringVar(&a.Aud, "aud", "", "audience, instead of -app/-client")
	flag.IntVar(&a.MaxAge, "max-age", 0, "assertion lifetime in seconds, instead of -app/-client")
	flag.StringVar(&a.Kid, "kid", "", "key id; must match the <kid>_public.pem the server holds")
	flag.StringVar(&a.Key, "key", "", "private key PEM path")
	flag.StringVar(&a.Base, "base", "", "scheme://host to send to (default: the audience)")
	flag.Var(&headers, "H", "extra request header \"Name: value\" (repeatable)")
	flag.Usage = usage
	flag.Parse()

	if *confPath != "" {
		if err := mergeConf(&a, *confPath, headers); err != nil {
			fail("%v", err)
		}
	}
	a.Headers = append(a.Headers, headers...)

	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}
	method, target := strings.ToUpper(flag.Arg(0)), flag.Arg(1)
	if !strings.HasPrefix(target, "/") {
		fail("TARGET must start with \"/\" — it is the request target, not a URL")
	}

	signer, err := buildSigner(&a)
	if err != nil {
		fail("%v", err)
	}

	var payload []byte
	if *body != "" {
		if strings.HasPrefix(*body, "@") {
			if payload, err = os.ReadFile((*body)[1:]); err != nil {
				fail("body file: %v", err)
			}
		} else {
			payload = []byte(*body)
		}
	}

	sign := func() string {
		authValue, err := signer.SignRequest(method, target, payload, nil)
		if err != nil {
			fail("sign: %v", err)
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "signed: %s %s  aud=%s  kid=%s  ttl=%ds  body=%d bytes\n",
				method, target, signer.Audience, signer.Kid, int(signer.MaxAge/time.Second), len(payload))
		}
		return authValue
	}
	authValue := sign()
	if *signOnly {
		fmt.Println(authValue)
		return
	}

	base := a.Base
	if base == "" {
		base = signer.Audience // the audience IS the verifying side's identity
	}
	newRequest := func(authValue string) *http.Request {
		req, err := http.NewRequest(method, strings.TrimRight(base, "/")+target, bytes.NewReader(payload))
		if err != nil {
			fail("request: %v", err)
		}
		req.Header.Set("Authorization", authValue)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for _, h := range a.Headers {
			name, value, _ := strings.Cut(h, ":")
			req.Header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
		}
		return req
	}
	showHead := func(res *http.Response) {
		if *showRes {
			fmt.Fprintf(os.Stderr, "HTTP %s\n", res.Status)
			for k, v := range res.Header {
				fmt.Fprintf(os.Stderr, "%s: %s\n", k, strings.Join(v, ", "))
			}
			fmt.Fprintln(os.Stderr)
		}
	}

	req := newRequest(authValue)
	res, release, err := send(req)
	if err != nil {
		fail("send: %v", err)
	}
	showHead(res)

	// An event stream is held the way a browser's EventSource holds one: each event is printed as it comes; when
	// the stream ends or the connection fails, the same request is sent again — freshly signed, since an assertion
	// is admitted once — after the stream's reconnection time, with Last-Event-ID; any answer that is not an event
	// stream ends it, and is printed and judged like any other answer.
	if isEventStream(res) {
		stream := eventStream{lastEventID: req.Header.Get("Last-Event-ID"), reconnect: 3 * time.Second}
		for isEventStream(res) {
			err = stream.read(res.Body, os.Stdout)
			release()
			cause := "the server ended it"
			if err != nil {
				cause = err.Error()
			}
			for {
				fmt.Fprintf(os.Stderr, "stream ended (%s); reopening in %v\n", cause, stream.reconnect)
				time.Sleep(stream.reconnect)
				req = newRequest(sign())
				req.Header.Del("Last-Event-ID")
				if stream.lastEventID != "" {
					req.Header.Set("Last-Event-ID", stream.lastEventID)
				}
				if res, release, err = send(req); err == nil {
					break
				}
				cause = "reopening failed: " + err.Error()
			}
			showHead(res)
		}
		fmt.Fprintf(os.Stderr, "stream not reopened: HTTP %s\n", res.Status)
	}
	defer release()
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	os.Stdout.Write(out)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		fmt.Println()
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		fmt.Fprintf(os.Stderr, "refused: HTTP %s\n", res.Status)
		os.Exit(1)
	}
}

// send makes one exchange and cuts it after 30 s, headers and body together — unless the answer is an event stream,
// whose body has no end to wait for. release frees the exchange once its body is done with.
func send(req *http.Request) (res *http.Response, release func(), err error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	cut := time.AfterFunc(30*time.Second, func() { cancel(errors.New("no complete answer within 30 s")) })
	res, err = http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		cut.Stop()
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		cancel(nil)
		return nil, nil, err
	}
	if isEventStream(res) {
		cut.Stop()
	}
	return res, func() { cut.Stop(); cancel(nil) }, nil
}

// isEventStream reports whether res opens an event stream: status 200 with Content-Type text/event-stream, the only
// answer a browser's EventSource holds.
func isEventStream(res *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	return res.StatusCode == http.StatusOK && err == nil && mediaType == "text/event-stream"
}

// mergeConf fills only the fields the command line left empty: a flag the
// caller typed is the caller's decision and the file may not overrule it.
func mergeConf(a *args, path string, typed headerFlags) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("conf: %w", err)
	}
	var fromFile args
	if err = json.Unmarshal(b, &fromFile, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("conf %s: %w", path, err)
	}
	if a.App == "" {
		a.App = fromFile.App
	}
	if a.Client == "" {
		a.Client = fromFile.Client
	}
	if a.ID == "" {
		a.ID = fromFile.ID
	}
	if a.Aud == "" {
		a.Aud = fromFile.Aud
	}
	if a.MaxAge == 0 {
		a.MaxAge = fromFile.MaxAge
	}
	if a.Kid == "" {
		a.Kid = fromFile.Kid
	}
	if a.Key == "" {
		a.Key = fromFile.Key
	}
	if a.Base == "" {
		a.Base = fromFile.Base
	}
	a.Headers = append(a.Headers, fromFile.Headers...)
	return nil
}

// buildSigner resolves the client's identity either from the app's own conf
// (preferred: the server reads that same file, so nothing can drift) or from
// values given directly, for an app whose conf this host cannot read.
func buildSigner(a *args) (*jwtassert.Signer, error) {
	if a.Kid == "" || a.Key == "" {
		return nil, fmt.Errorf("-kid and -key are required")
	}
	id, aud, maxAge := a.ID, a.Aud, a.MaxAge
	switch {
	case a.App != "":
		if a.Client == "" {
			return nil, fmt.Errorf("-client is required with -app")
		}
		confs, err := jwtassert.LoadClientConfs(a.App)
		if err != nil {
			return nil, fmt.Errorf("client conf: %w", err)
		}
		c, ok := confs[a.Client]
		if !ok {
			return nil, fmt.Errorf("client %q is not in %s/config/.web-authn-jwtassert.json", a.Client, a.App)
		}
		id, aud, maxAge = c.ID, c.Audience, c.MaxAge
	case id == "" || aud == "" || maxAge == 0:
		return nil, fmt.Errorf("give -app with -client, or all of -id, -aud and -max-age")
	}
	pemBytes, err := os.ReadFile(a.Key)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	return &jwtassert.Signer{
		ID: id, Audience: aud, Kid: a.Kid,
		PrivateKey: key, MaxAge: time.Duration(maxAge) * time.Second,
	}, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `signedreq — one HTTP request to a framework app, signed as a jwtassert client; an
event-stream answer is held as a browser holds one, each event printed as a JSON line.

usage: signedreq [flags] METHOD TARGET

  METHOD  GET, POST, ...
  TARGET  path plus optional ?query, exactly as the server will see it

examples:
  signedreq -conf ~/.config/signedreq/foo.json GET /svc/ping
  signedreq -conf ~/.config/signedreq/foo.json -d '{"note":"bar"}' POST /svc/notes
  signedreq -app /srv/foo -client bar -kid bar-20260101 -key /keys/bar_private.pem \
         GET /svc/ping

flags:
`)
	flag.PrintDefaults()
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}
