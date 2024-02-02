package daemon

import (
	"context"
	"errors"
)

var ErrAlreadyServing = errors.New("already serving")

func NewCancelableApplet[id ID](core cancelableCore, appId id, deps ...id) Applet[id] {
	return &cancelableWrapper[id]{
		id:   appId,
		deps: deps,
		core: core,
	}
}

type cancelableCore interface {
	// Initialize same as `Applet.Initialize`
	Initialize() error
	// Serve generally same as `Applet.Serve`, but implementation shall quit when
	// serveCtx is done
	Serve(serveCtx context.Context, onStartup func()) error
	// Shutdown same as `Applet.Shutdown`
	Shutdown(stopCtx context.Context) error
}

type cancelableWrapper[id ID] struct {
	id      id
	deps    []id
	status  atomicBool
	statCtx context.Context
	statCan context.CancelFunc
	core    cancelableCore
}

func (w *cancelableWrapper[id]) Identity() id { return w.id }

func (w *cancelableWrapper[id]) Depends() []id { return w.deps }

func (w *cancelableWrapper[id]) OnQuit(ctx OnQuitCtx) {
	ctx.ReStart(true)
	ctx.ReInitialize(true)
}

func (w *cancelableWrapper[id]) Initialize() (err error) {
	if w.status.Load() {
		return ErrAlreadyServing
	}
	w.statCtx, w.statCan = context.WithCancel(context.Background())
	return w.core.Initialize()
}

func (w *cancelableWrapper[id]) Shutdown(ctx context.Context) (err error) {
	w.statCan()
	return w.core.Shutdown(ctx)
}

func (w *cancelableWrapper[id]) Serving() bool { return w.status.Load() }

func (w *cancelableWrapper[id]) Serve(serveOK func()) (err error) {
	if !w.status.CompareAndSwap(false, true) {
		return ErrAlreadyServing
	}
	defer func() {
		w.status.Store(false)
	}()
	return w.core.Serve(w.statCtx, serveOK)
}
