package daemon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultAppletClosureMaxRetry = 5
	DefaultAppletRestartInterval = time.Second
)

var (
	ErrDaemonIsInitialized      = fmt.Errorf("daemon is initialized")
	ErrDaemonNotInitialized     = fmt.Errorf("daemon not initialized")
	ErrDaemonRegisterIsFrozen   = fmt.Errorf("daemon register is frozen")
	ErrAppletIdentityConflict   = fmt.Errorf("applet identity conflict")
	ErrAppletQuitsWithoutReason = fmt.Errorf("applet quits without reason")
	ErrAppletCircularDependency = fmt.Errorf("applet circular dependency")
)

type daemonStatusCtx struct {
	didInit bool
	halting bool
	ctx     context.Context
	cancel  func()
}

func newDaemonStatusCtx() daemonStatusCtx {
	var cc = daemonStatusCtx{}
	cc.ctx, cc.cancel = context.WithCancel(context.Background())
	return cc
}

func (c *daemonStatusCtx) running() bool {
	return !c.halting && !(c.ctx != nil && errors.Is(context.Canceled, c.ctx.Err()))
}

func (c *daemonStatusCtx) setHalting() {
	c.halting = true
}

func (c *daemonStatusCtx) halt() {
	c.halting = true
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *daemonStatusCtx) haltSig() <-chan struct{} {
	return c.ctx.Done()
}

func (c *daemonStatusCtx) setInitialized() bool {
	if c.didInit {
		return false
	}
	c.didInit = true
	return true
}

func (c *daemonStatusCtx) initialized() bool {
	return c.didInit
}

type daemonAppletsGraph[id ID] Daemon[id]

func (g *daemonAppletsGraph[id]) nodes() []id {
	var keys = make([]id, 0, len(g.applets))
	for key := range g.applets {
		keys = append(keys, key)
	}
	return keys
}

func (g *daemonAppletsGraph[id]) neighbors(appId id) []id {
	if applet, exist := g.applets[appId]; exist {
		return applet.Depends()
	}
	return nil
}

type Daemon[id ID] struct {
	serveMaxRetry   int
	closureMaxRetry int
	restartInterval time.Duration
	logger          logger
	applets         map[id]Applet[id]
	status          daemonStatusCtx
}

func NewDaemon[id ID](opts ...OptionPayload[id]) *Daemon[id] {
	var d = &Daemon[id]{
		serveMaxRetry:   DefaultAppletClosureMaxRetry,
		closureMaxRetry: DefaultAppletClosureMaxRetry,
		restartInterval: DefaultAppletRestartInterval,
		logger:          defaultLoggerType{},
		status:          newDaemonStatusCtx(),
	}
	for _, opt := range opts {
		opt.apply(d)
	}
	return d
}

func (d *Daemon[id]) Register(applets ...Applet[id]) error {
	if d.status.initialized() {
		return ErrDaemonRegisterIsFrozen
	}
	if d.applets == nil {
		d.applets = make(map[id]Applet[id], len(applets))
	}
	for _, applet := range applets {
		var identity = applet.Identity()
		if _, exist := d.applets[identity]; exist {
			return fmt.Errorf("%w, applet = %v", ErrAppletIdentityConflict, identity)
		}
		d.applets[identity] = applet
	}
	return nil
}

func (d *Daemon[id]) Init() error {
	if !d.status.setInitialized() {
		return ErrDaemonIsInitialized
	}
	var applets, err = d.sortDependency(empty[id](), false)
	if err != nil {
		return err
	}
	for _, applet := range applets {
		if err = applet.Initialize(); err != nil {
			return fmt.Errorf("%w: applet = %s", err, applet.Identity())
		}
	}
	return nil
}

func (d *Daemon[id]) sortDependency(target id, reverse bool) ([]Applet[id], error) {
	if d.applets == nil {
		return []Applet[id]{}, nil
	}
	var isDAG bool
	var idList []id
	if target == empty[id]() {
		idList, isDAG = newDAGVisitor[id]((*daemonAppletsGraph[id])(d)).All()
	} else {
		idList, isDAG = newDAGVisitor[id]((*daemonAppletsGraph[id])(d)).Single(target)
	}
	if !isDAG {
		return nil, ErrAppletCircularDependency
	}
	var appletsList = make([]Applet[id], len(idList))
	if reverse {
		for i, appId := range idList {
			appletsList[len(idList)-1-i], _ = d.applets[appId]
		}
	} else {
		for i, appId := range idList {
			appletsList[i], _ = d.applets[appId]
		}
	}
	return appletsList, nil
}

func (d *Daemon[id]) Serve() (err error) {
	if !d.status.initialized() {
		return ErrDaemonNotInitialized
	}
	defer d.status.halt()
	var applets []Applet[id]
	if applets, err = d.sortDependency(empty[id](), false); err != nil {
		return err
	}
	var wg sync.WaitGroup
	var errs errorCollection
	var escapeCtx, escape = context.WithCancel(context.Background())
	defer escape()
startupLoop:
	for _, applet := range applets {
		wg.Add(1)
		var bootstrapCtx, bootstrapDone = context.WithCancel(context.Background())
		go func(_app Applet[id]) {
			var _err = d.serveApplet(_app, escapeCtx.Done(), bootstrapDone)
			if _err != nil {
				errs.append(fmt.Errorf("[applet::%s] %w", _app.Identity(), _err))
				escape()
			}
			wg.Done()
			bootstrapDone()
		}(applet)
		select {
		case <-escapeCtx.Done():
			break startupLoop
		case <-bootstrapCtx.Done():
			continue
		case <-time.After(time.Minute):
			errs.append(fmt.Errorf("applet startup timeout: id = %s", applet.Identity()))
			escape()
		}
	}
	wg.Wait()
	return errs.batch()
}

func (d *Daemon[id]) ServeAsync() <-chan error {
	var waitChan = make(chan error)
	go func() {
		waitChan <- d.Serve()
		close(waitChan)
	}()
	return waitChan
}

func (d *Daemon[id]) Shutdown(ctx context.Context) (err error) {
	if !d.status.running() {
		return nil
	}
	d.status.setHalting()
	var applets []Applet[id]
	if applets, err = d.sortDependency(empty[id](), true); err != nil {
		return err
	}
	var errs errorCollection
	for i := len(applets) - 1; i >= 0; i-- {
		if err = d.shutdownApplet(applets[i], ctx); err != nil {
			errs.append(fmt.Errorf("%w: applet = %s", err, applets[i].Identity()))
		}
	}
	return errs.batch()
}

func (d *Daemon[id]) serveApplet(applet Applet[id], escape <-chan struct{}, startOk context.CancelFunc) (err error) {
	var lifeCount int
	var appStarted atomicBool
	var appIdentity = applet.Identity()
	var appWrapper = &appletServeWrapper[id]{applet: applet, startOk: func() {
		if appStarted.CompareAndSwap(false, true) {
			startOk()
			d.logger.Infof("applet %q started", applet.Identity())
		}
	}}
	var appRestartInterval = ternary(d.restartInterval != 0, d.restartInterval, DefaultAppletRestartInterval)
	var appRestartMaxRetry = ternary(d.serveMaxRetry > 0, d.serveMaxRetry, DefaultAppletClosureMaxRetry)
retryLoop:
	for lifeCount = 0; d.status.running(); lifeCount++ {
		switch {
		case lifeCount >= appRestartMaxRetry:
			// exceed max fail times
			d.logger.Errorf("applet %q bootstrap failed after maximum(%d) retries.", appIdentity, appRestartMaxRetry)
			err = ternary(appWrapper.lastErr != nil, appWrapper.lastErr, ErrAppletQuitsWithoutReason)
			return err
		case lifeCount > 0:
			if appWrapper.fail {
				err = ternary(appWrapper.lastErr != nil, appWrapper.lastErr, ErrAppletQuitsWithoutReason)
				return err
			}
			if !appWrapper.reStart {
				return nil
			}
		}
		time.Sleep(appRestartInterval)
		if !d.status.running() {
			return nil
		}
		select {
		case <-escape:
			return nil
		case <-d.status.haltSig():
			d.logger.Warnf("applet %q halt without closure", appIdentity)
			break retryLoop
		case err = <-async(appWrapper.serve):
			applet.OnQuit(appWrapper)
			var actDesc string
			if d.status.running() {
				actDesc = appWrapper.Description(appRestartInterval)
			} else {
				actDesc = "daemon shutting down"
			}
			if err != nil {
				d.logger.Warnf("applet %q quit with error (%s): %v", appIdentity, actDesc, err)
			} else {
				d.logger.Warnf("applet %q quit without error (%s)", appIdentity, actDesc)
			}
		}
	}
	return nil
}

func (d *Daemon[id]) shutdownApplet(applet Applet[id], ctx context.Context) (err error) {
	for i := 0; true; i++ {
		switch {
		case i >= ternary(d.closureMaxRetry > 0, d.closureMaxRetry, DefaultAppletClosureMaxRetry):
			return errors.New("shutdown failed after repeatedly retries")
		case !applet.Serving():
			return nil
		}
		d.logger.Warnf("applet %q is going to shutdown", applet.Identity())
		if err = applet.Shutdown(ctx); err == nil {
			return nil
		} else {
			d.logger.Warnf("applet %q shutdown fail: %v", applet.Identity(), err)
		}
		time.Sleep(ternary(d.restartInterval != 0, d.restartInterval, DefaultAppletRestartInterval))
	}
	return err
}

func (d *Daemon[id]) Halt() { d.status.halt() }
