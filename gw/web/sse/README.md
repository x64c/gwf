# sse

Server-Sent Events: one-way streams from the server to one client over ordinary HTTP (`text/event-stream`). The
`Service` owns every open stream in the process — their lifetimes, and how long each frame may take to write — so
streams take part in the framework's lifecycle like any other service work: stopping the service ends them, and while
it is stopped new ones are refused. What a stream sends is never the service's: each `Serve` call brings its `Source`.

## Pieces

| | |
|---|---|
| `Conf` | `config/.web-sse.json`: `frame_write_timeout_secs`, `retry_millis` — both required |
| `Service` | `Start` / `Stop` / `Terminate`; `Stats` |
| `Serve` | runs one stream on the request's response until it ends |
| `Source` | what one stream sends: `Items` of any type, `ToEvent`, an optional `EndNotice`, the `Backlog`, the `Heartbeat` |
| `Event` | one frame: `ID` (the client's `Last-Event-ID`), `Name` (the client-side listener), `Data` (may span lines) |
| `Heartbeat` | what a quiet stream sends: after `Silence` without a write, a comment, or the given `Event` |
| `RefuseStream` | answer a stream request with a status other than 200: the client closes for good |
| `ComeBackAfter` | answer a stream request with a stream that ends at once: the client returns after the wait |
| `LastEventID` | the ID the client says it last received |

## A stream's life

```go
src := sse.Source[Item]{
	Items:     items,                          // the producer's channel
	ToEvent:   func(it Item) sse.Event { … },  // the app's translation
	Backlog:   missedSince(sse.LastEventID(r, "lastEventId")),
	Heartbeat: sse.Heartbeat{Silence: 15 * time.Second},
}
err := sse.Serve(service, w, r, src)
```

1. The handler decides the caller may have the stream (authorization is the app's), prepares its source, and loads
   what the client missed if it says where it was.
2. `Serve` answers 204 while the service is not running, and 500 when the `ResponseWriter` it is given hides write
   deadlines — a wrapper in front that passes on a writer of its own must give it `Unwrap() http.ResponseWriter`,
   since every frame sets a deadline and flushes through `http.ResponseController`. Otherwise it sends the headers and
   the `Backlog`, then a frame per item and the heartbeat after each silence.
3. The stream ends when `Items` or `EndNotice` closes, when the web service starts its drain, when the service stops,
   when the client leaves, or when a write fails or does not finish within `frame_write_timeout_secs`. On the first
   three the client is told to reconnect after `retry_millis`.

## Reconnecting

A conforming client (`EventSource`) treats every end of a stream as an interruption: it waits its reconnection time
(`retry:` sets it) and asks again, sending `Last-Event-ID` — the `id` of the last event it received. It stops only
when an answer is not a 200 stream; `RefuseStream` gives that answer, `ComeBackAfter` asks it to return after a wait.
A page that closes a stream and opens a new one starts without a last ID and has to pass it in the URL, which is what
`LastEventID`'s parameter reads — after the header, which is always the latest.

## Heartbeat

A comment line keeps proxies on the path from closing a quiet stream, and a client ignores it. A page that wants to
notice a silent break — a connection that died without telling anyone — needs a heartbeat it can see: give the
`Heartbeat` an `Event` and listen for its name.
