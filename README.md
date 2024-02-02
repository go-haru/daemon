# daemon

[![Go Reference](https://pkg.go.dev/badge/github.com/go-haru/daemon.svg)](https://pkg.go.dev/github.com/go-haru/daemon)
[![License](https://img.shields.io/github/license/go-haru/daemon)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/go-haru/daemon.svg?style=flat-square)](https://github.com/go-haru/daemon/releases)
[![Go Test](https://github.com/go-haru/daemon/actions/workflows/go.yml/badge.svg)](https://github.com/go-haru/daemon/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-haru/daemon)](https://goreportcard.com/report/github.com/go-haru/daemon)

daemon is runtime for wrapped lifecycle (we call it Applet) of continuously running code. By using DAG, applets start and shutdown by dependency tree order. 

## Types

you need to wrap your code by implementing following two interface:

### ID

is unique identifier for each Applet, expected to be `comparible` and `fmt.Stringer`, e.g.

```go
import "encoding/hex"

type AppID [4]byte

func (id AppID) String() string { return hex.EncodeToString(id[:]) }
```

PS: external uid implement can also be nice choice, like:

* [google/uuid](https://pkg.go.dev/github.com/google/uuid#UUID)
* [segmentio/ksuid](https://pkg.go.dev/github.com/segmentio/ksuid#KSUID)
* [rs/xid](https://pkg.go.dev/github.com/rs/xid#ID)

### (option A.) Applet

is interface for wrapping your goroutine's lifecycle.

```go
// please implement this interface
type Applet[id ID] interface {
    // unique identifier for this Applet
    Identity() id
    // applets expected to be ready before this applet start
    Depends() []id

    // init or clear applet status, typically for maps and channels
    Initialize() error
    // start and block until underlying operation exit.
    // implementation shall call `onReady()` to inform daemon this applet
	// started with normal status and can start other applet. 
    Serve(onReady func()) error
    // tell and block until underlying operation quit, 
    // if ctx canceled, stop blocking and return `ctx.Error()`.
    Shutdown(ctx context.Context) error
    
    // called before `Shutdown(ctx)`, if false then skip
    Serving() bool
    // called after `Serve(onReady)` quited, implementation shall call
    // `onQuit.ReInitialize/ReStart/Fail(...)` to tell daemon what's going on.
    // (see OnQuitCtx)
    OnQuit(onQuit OnQuitCtx)
}
```

`OnQuitCtx` is implemented by daemon and expected to be called it in `Applet.OnQuit`

```go
type OnQuitCtx interface {
    // tell daemon whether to schedule applet's restart, false if omitted
    ReStart(yes bool)
    // tell daemon whether call `Applet.Initialize()` before restart, false if omitted
    ReInitialize(yes bool)
    // tell daemon whether applet quited for unexpected reason, (true, nil) if omitted
    // for normal reason, like context canceled when app shutting down:
    // >    ctx.Fail(false, nil)
    // for abnormal reason, and want to keep original error output:
    // >    ctx.Fail(true, nil)
    // or if you want wrapping error for logging:
    // >    ctx.Fail(true, function(err error) error {
    // >        return fmt.Errorf("%w: %v", masqueradingErr, err)
    // >    })
    Fail(yes bool, filter func(err error) error)
}
```

### (option B.) cancelableCore

is wrapper of Applet, suitable for lifecycle which managed by context's done signal.

```go
type cancelableCore interface {
    // same as `Applet.Initialize`
    Initialize() error
    // generally same as `Applet.Serve`, but implementation shall quit when
    // serveCtx is done
    Serve(serveCtx context.Context, onStartup func()) error
    // same as `Applet.Shutdown`
    Shutdown(stopCtx context.Context) error
}
```

then call `NewCancelableApplet` to wrap it as `Applet`

```go
func NewCancelableApplet[id ID](core cancelableCore, appId id, deps ...id) Applet[id]
```

## Usage

As description above, you need to provide your implementation first, let's assume we have factory function:

```go
function newApplet1() daemon.Applet { /* ... */ }

function newApplet2() daemon.Applet { /* ... */ }
```

To register `Applet` to daemon and start them:

on main goroutine:

PS: check [options.go](./options.go) for all option provider 

```go
var serviced = daemon.NewDaemon[AppID](/* options omitted */)

err = serviceDaemon.Register(newApplet1())
// !do err check here

err = serviceDaemon.Register(newApplet2())
// !do err check here

err = serviceDaemon.Init()
// !do err check here

err = serviceDaemon.Serve()
// !do err check here
// here will block until all applet quited
```

program quit event (like received ctrl+c):

```go
var gracefulShtdownWaitSec = time.Second * 15 // CHANGE according actual needs
var ctx, done = context.WithTimeout(..., gracefulShtdownWaitSec)
err = serviceDaemon.Shutdown(ctx)
// !do err check here
// here will block until all applet quited or ctx is done
```

## Contributing

For convenience of PM, please commit all issue to [Document Repo](https://github.com/go-haru/go-haru/issues).

## License

This project is licensed under the `Apache License Version 2.0`.

Use and contributions signify your agreement to honor the terms of this [LICENSE](./LICENSE).

Commercial support or licensing is conditionally available through organization email.
