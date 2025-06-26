package karpserver

import (
	"context"
	"errors"
	"time"
)

const MaxDelta = ^uint64(0) >> 1

type FlowWindow struct {
	ctx     context.Context
	head    uint64
	tail    uint64
	timer   *time.Timer
	timeout time.Duration
	ids     chan uint64
}

func NewFlowWindow(ctx context.Context, maxDelta uint, timeout time.Duration) *FlowWindow {

	if uint64(maxDelta) >= MaxDelta {
		panic("maxDelta must be less than 2^63 to avoid wrap-around undefined behavior")
	}

	fw := &FlowWindow{
		ctx:     ctx,
		timeout: timeout,
		ids:     make(chan uint64, maxDelta),
	}

	fw.Chase(0)

	return fw
}

func (fw *FlowWindow) Next() (uint64, error) {

	if fw.timer == nil {
		fw.timer = time.NewTimer(fw.timeout)
	} else {
		fw.timer.Reset(fw.timeout)
	}

	select {
	case id := <-fw.ids:
		fw.timer.Stop()
		return id, nil

	case <-fw.ctx.Done():
		return 0, fw.ctx.Err()

	case <-fw.timer.C:
		return 0, errors.New("message round-trip timeout")
	}

}

func (fw *FlowWindow) Chase(id uint64) {

	// id is less then tail
	if (id - fw.tail) >= MaxDelta {
		return
	}

	// id is more then head
	if (fw.head - id) >= MaxDelta {
		return
	}

	fw.tail = id

	for range uint64(cap(fw.ids)) - (fw.head - fw.tail) {
		fw.ids <- fw.head
		fw.head++
	}
}
