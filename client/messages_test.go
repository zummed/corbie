package client_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/store"
)

// A cancel, an event and an answer are each good for the life they are
// given, or for a few minutes when given none; and with no controller
// running, a caller waiting for the receipt of one is told it was not taken
// up once that life has passed, and not before.
func TestWaitForAReceiptEndsWhenTheMessageIsNoLongerGood(t *testing.T) {
	a := arrange(t)
	var ahead atomic.Int64
	a.Now = func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) }
	run := a.named("0000000001")
	// How long a message in the controller's inbox says it is good for.
	goodFor := func(message string) time.Duration {
		t.Helper()
		at := store.Message("controller", "client", message)
		data, mark, err := a.owner.Read(ctx, at)
		if err != nil || mark == store.None {
			t.Fatalf("the message %s is not in the controller's inbox: %v", message, err)
		}
		value, err := identity.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		return value.Until.Sub(value.Sent)
	}
	for what, send := range map[string]func(life time.Duration) (string, error){
		"a cancel":  func(life time.Duration) (string, error) { return a.Cancel(ctx, run, life) },
		"an event":  func(life time.Duration) (string, error) { return a.Send(ctx, run, "go", nil, life) },
		"an answer": func(life time.Duration) (string, error) { return a.Reply(ctx, run, "the-key", true, life) },
	} {
		for _, life := range []time.Duration{0, 10 * time.Second} {
			ahead.Store(0)
			message, err := send(life)
			if err != nil {
				t.Fatal(err)
			}
			want := life
			if life == 0 {
				want = instance.MessageLife
			}
			if got := goodFor(message); got != want {
				t.Errorf("%s sent with a life of %v is good for %v", what, life, got)
			}
			// The clock is moved once the wait has begun, which its first
			// look for the receipt shows.
			ahead.Store(0)
			looks := make(chan struct{}, 1)
			a.hooked.mu.Lock()
			a.hooked.before = func(string, string) error {
				select {
				case looks <- struct{}{}:
				default:
				}
				return nil
			}
			a.hooked.mu.Unlock()
			ended := make(chan error, 1)
			go func() {
				_, err := a.WaitReceipt(ctx, message, life, time.Millisecond)
				ended <- err
			}()
			<-looks
			// While the message is good the wait goes on, look after look.
			ahead.Store(int64(want - time.Second))
			for range 3 {
				select {
				case <-looks:
				case err := <-ended:
					t.Fatalf("a second before %s stops being good, the wait for its receipt ended: %v", what, err)
				case <-time.After(5 * time.Second):
					t.Fatalf("the wait for the receipt of %s stopped looking", what)
				}
			}
			// Once it is no longer good, the wait ends and says so.
			ahead.Store(int64(want + time.Second))
			select {
			case err := <-ended:
				if !errors.Is(err, client.ErrNotTaken) {
					t.Errorf("once %s was no longer good, the wait for its receipt gave %v", what, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("the wait for the receipt of %s went on after it stopped being good", what)
			}
		}
	}
}
