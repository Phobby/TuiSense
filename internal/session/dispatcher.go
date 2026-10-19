package session

import (
	"context"
	"sync"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/logging"
)

// Dispatcher queues, coalesces, and asynchronously executes controller write commands
// so rapid slider or color-picker drags never block the Limoni UI render loop.
type Dispatcher struct {
	mu       sync.Mutex
	pending  map[string]pendingCommand
	order    []string
	wakeCh   chan struct{}
	mgr      *Manager
	logger   *logging.Logger
	onResult func(cmd device.Command, err error)
}

// pendingCommand binds a command to the device that was active when it was issued,
// so switching devices never redirects queued writes to the wrong controller.
type pendingCommand struct {
	deviceKey string
	cmd       device.Command
}

// NewDispatcher creates a new asynchronous command dispatcher attached to Manager.
func NewDispatcher(mgr *Manager, logger *logging.Logger, onResult func(cmd device.Command, err error)) *Dispatcher {
	return &Dispatcher{
		pending:  make(map[string]pendingCommand),
		wakeCh:   make(chan struct{}, 1),
		mgr:      mgr,
		logger:   logger,
		onResult: onResult,
	}
}

// Dispatch enqueues a command for the active device, coalescing it with any
// pending command of the same category for that device.
func (d *Dispatcher) Dispatch(cmd device.Command) {
	devKey := d.mgr.ActiveKey()
	key := devKey + "|" + cmd.CoalesceKey()
	d.mu.Lock()
	if _, exists := d.pending[key]; !exists {
		d.order = append(d.order, key)
	}
	d.pending[key] = pendingCommand{deviceKey: devKey, cmd: cmd}
	d.mu.Unlock()

	select {
	case d.wakeCh <- struct{}{}:
	default:
	}
}

// Start runs the background worker loop that drains coalesced commands every ~16ms.
func (d *Dispatcher) Start(ctx context.Context) {
	coalesceTicker := time.NewTicker(16 * time.Millisecond)
	defer coalesceTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.wakeCh:
			d.flushBatch(ctx)
		case <-coalesceTicker.C:
			d.flushBatch(ctx)
		}
	}
}

func (d *Dispatcher) flushBatch(ctx context.Context) {
	d.mu.Lock()
	if len(d.order) == 0 {
		d.mu.Unlock()
		return
	}
	batch := make([]pendingCommand, 0, len(d.order))
	for _, k := range d.order {
		batch = append(batch, d.pending[k])
	}
	d.order = d.order[:0]
	clear(d.pending)
	d.mu.Unlock()

	for _, pc := range batch {
		cmd := pc.cmd
		ctrl := d.mgr.Device(pc.deviceKey)
		if ctrl == nil {
			if d.onResult != nil {
				d.onResult(cmd, nil)
			}
			continue
		}
		err := ctrl.ExecuteCommand(ctx, cmd)
		if err != nil {
			if d.logger != nil {
				d.logger.Error("DISPATCH", "%s failed: %v", cmd.Describe(), err)
			}
		} else {
			if d.logger != nil {
				d.logger.Debug("DISPATCH", "Sent %s -> [%s]", cmd.Describe(), ctrl.Telemetry().LastOutputHex)
			}
		}
		if d.onResult != nil {
			d.onResult(cmd, err)
		}
	}
}

// FlushNow synchronously sends every queued command (used during shutdown).
func (d *Dispatcher) FlushNow() {
	d.flushBatch(context.Background())
}
