package gotemplates

import "time"

type Ticker struct {
	d time.Duration
	t *time.Ticker
	f func()
	s chan struct{}
}

func NewTicker(durationMS int, callback func()) *Ticker {
	return &Ticker{
		d: time.Duration(durationMS) * time.Millisecond,
		t: nil,
		f: callback,
		s: make(chan struct{}),
	}
}

// go t.Start()
func (t *Ticker) Start() {
	t.t = time.NewTicker(t.d)
	defer t.t.Stop()

	for {
		select {
		case <-t.t.C:
			go t.f()
		case <-t.s:
			return
		}
	}
}

func (t *Ticker) Stop() {
	if t.t != nil {
		t.s <- struct{}{}
	}
}

func (t *Ticker) Reset() {
	if t.t != nil {
		t.t.Reset(t.d)
	}
}
