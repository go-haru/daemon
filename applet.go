package daemon

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

type ID interface {
	comparable
	fmt.Stringer
}

type Applet[id ID] interface {
	// Identity unique identifier for this Applet
	Identity() id
	// Depends applets expected to be ready before this applet start
	Depends() []id
	// Initialize init or clear applet status, typically for maps and channels
	Initialize() error
	// Serve start and block until underlying operation exit.
	// implementation shall call `onReady()` to inform daemon goes on.
	Serve(onReady func()) error
	// Shutdown tell and block until underlying operation quit,
	// if ctx canceled, stop blocking and return `ctx.Error()`.
	Shutdown(ctx context.Context) error

	// Serving called before `Shutdown(ctx)`, if false then skip
	Serving() bool
	// OnQuit called after `Serve(onReady)` quited, implementation shall call
	// `onQuit.ReInitialize/ReStart/Fail(...)` to tell daemon what's going on.
	// (see OnQuitCtx)
	OnQuit(onQuit OnQuitCtx)
}

type OnQuitCtx interface {
	// ReStart tell daemon whether to schedule applet's restart, false if omitted
	ReStart(yes bool)
	// ReInitialize tell daemon whether call `Applet.Initialize()` before restart, false if omitted
	ReInitialize(yes bool)
	// Fail tell daemon whether applet quited for unexpected reason, (true, nil) if omitted
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

type appletServeWrapper[id ID] struct {
	applet  Applet[id]
	startOk func()
	lastErr error
	reInit  bool
	reStart bool
	fail    bool
}

func (w *appletServeWrapper[id]) serve() (err error) {
	defer func() {
		if _err := recover(); _err != nil {
			err = fmt.Errorf("panic occurred: %v, trace:\n%s", anyAsErr(_err), string(debug.Stack()))
		}
		if err != nil {
			w.lastErr = err
		}
	}()
	if w.reInit {
		if err = w.applet.Initialize(); err != nil {
			return err
		}
	}
	if err = w.applet.Serve(w.startOk); err != nil {
		return err
	}
	return nil
}

func (w *appletServeWrapper[id]) LastError() error { return w.lastErr }

func (w *appletServeWrapper[id]) ReInitialize(yes bool) { w.reInit = yes }

func (w *appletServeWrapper[id]) ReStart(yes bool) { w.reStart = yes }

func (w *appletServeWrapper[id]) Fail(yes bool, filter func(err error) error) {
	if w.fail = yes; w.fail && filter != nil {
		w.lastErr = filter(w.lastErr)
	}
}

func (w *appletServeWrapper[id]) Description(after time.Duration) string {
	switch {
	case w.fail:
		return "fail and exit"
	case w.reStart:
		if w.reInit {
			return fmt.Sprintf("going to restart and re-init after %s", after)
		}
		return fmt.Sprintf("going to restart without re-init after %s", after)
	default:
		return "normally stop"
	}
}
