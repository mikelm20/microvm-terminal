package learnerbot

import (
	"context"
	"testing"

	"github.com/mikelm20/learn-platform/control-plane/internal/lesson"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// newEvaluatorRig wires a fresh EventBus, subscribes to step_satisfied
// events (before the evaluator starts so we do not miss its publishes),
// starts the evaluator in a goroutine, and returns everything the caller
// needs to drive the smoke test.
func newEvaluatorRig(t *testing.T, l *lesson.Lesson) (*session.EventBus, <-chan session.GuestEvent, context.CancelFunc, func()) {
	t.Helper()
	bus := session.NewEventBus()
	_, ch, unsub := bus.Subscribe(256)
	ev := lesson.NewEvaluator(nil, bus, l)
	ctx, cancel := context.WithCancel(context.Background())
	go ev.Run(ctx)
	return bus, ch, cancel, unsub
}
