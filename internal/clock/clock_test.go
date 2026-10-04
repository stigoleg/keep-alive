package clock

import (
	"testing"
	"time"
)

var epoch = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

func TestFakeNowAdvances(t *testing.T) {
	c := NewFake(epoch)
	if !c.Now().Equal(epoch) {
		t.Fatalf("Now() = %v, want %v", c.Now(), epoch)
	}
	c.Advance(90 * time.Second)
	if want := epoch.Add(90 * time.Second); !c.Now().Equal(want) {
		t.Fatalf("Now() = %v, want %v", c.Now(), want)
	}
}

func TestFakeTimerFiresAtDeadline(t *testing.T) {
	c := NewFake(epoch)
	tm := c.NewTimer(time.Minute)

	c.Advance(59 * time.Second)
	select {
	case <-tm.C():
		t.Fatal("timer fired early")
	default:
	}

	c.Advance(time.Second)
	select {
	case got := <-tm.C():
		if want := epoch.Add(time.Minute); !got.Equal(want) {
			t.Fatalf("fired at %v, want %v", got, want)
		}
	default:
		t.Fatal("timer did not fire at deadline")
	}
}

func TestFakeTimerStopAndReset(t *testing.T) {
	c := NewFake(epoch)
	tm := c.NewTimer(time.Minute)
	if !tm.Stop() {
		t.Fatal("Stop() on active timer = false, want true")
	}
	if tm.Stop() {
		t.Fatal("second Stop() = true, want false")
	}
	c.Advance(2 * time.Minute)
	select {
	case <-tm.C():
		t.Fatal("stopped timer fired")
	default:
	}

	tm.Reset(30 * time.Second)
	c.Advance(30 * time.Second)
	select {
	case <-tm.C():
	default:
		t.Fatal("reset timer did not fire")
	}
}

func TestFakeTickerFiresRepeatedly(t *testing.T) {
	c := NewFake(epoch)
	tk := c.NewTicker(10 * time.Second)
	defer tk.Stop()

	for i := 1; i <= 3; i++ {
		c.Advance(10 * time.Second)
		select {
		case got := <-tk.C():
			if want := epoch.Add(time.Duration(i) * 10 * time.Second); !got.Equal(want) {
				t.Fatalf("tick %d at %v, want %v", i, got, want)
			}
		default:
			t.Fatalf("tick %d missing", i)
		}
	}

	tk.Stop()
	c.Advance(time.Minute)
	select {
	case <-tk.C():
		t.Fatal("stopped ticker fired")
	default:
	}
}

func TestFakeTickerDropsTicksForSlowReader(t *testing.T) {
	c := NewFake(epoch)
	tk := c.NewTicker(time.Second)
	defer tk.Stop()

	c.Advance(5 * time.Second)
	<-tk.C()
	select {
	case <-tk.C():
		t.Fatal("ticker buffered more than one tick")
	default:
	}
}

func TestFakeAfter(t *testing.T) {
	c := NewFake(epoch)
	ch := c.After(time.Second)
	c.Advance(time.Second)
	select {
	case <-ch:
	default:
		t.Fatal("After channel did not fire")
	}
}

func TestFakeWaitersCountsActiveTimers(t *testing.T) {
	c := NewFake(epoch)
	tm := c.NewTimer(time.Minute)
	tk := c.NewTicker(time.Second)
	if got := c.Waiters(); got != 2 {
		t.Fatalf("Waiters() = %d, want 2", got)
	}
	tm.Stop()
	tk.Stop()
	if got := c.Waiters(); got != 0 {
		t.Fatalf("Waiters() = %d, want 0", got)
	}
}

func TestRealClock(t *testing.T) {
	c := Real()
	before := time.Now()
	if c.Now().Before(before) {
		t.Fatal("real Now() went backwards")
	}
	tm := c.NewTimer(time.Millisecond)
	select {
	case <-tm.C():
	case <-time.After(time.Second):
		t.Fatal("real timer did not fire")
	}
	tk := c.NewTicker(time.Millisecond)
	defer tk.Stop()
	select {
	case <-tk.C():
	case <-time.After(time.Second):
		t.Fatal("real ticker did not fire")
	}
}

func TestFakeResetDiscardsStaleTick(t *testing.T) {
	c := NewFake(epoch)
	tm := c.NewTimer(time.Second)
	c.Advance(time.Second) // fires; nobody reads
	tm.Reset(time.Minute)
	select {
	case <-tm.C():
		t.Fatal("stale tick delivered after Reset")
	default:
	}
}
