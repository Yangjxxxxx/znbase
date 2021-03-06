// Copyright 2019  The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package requestbatcher

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/stop"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"golang.org/x/sync/errgroup"
)

type batchResp struct {
	br *roachpb.BatchResponse
	pe *roachpb.Error
}

type batchSend struct {
	ba       roachpb.BatchRequest
	respChan chan<- batchResp
}

type chanSender chan batchSend

func (c chanSender) Send(
	ctx context.Context, ba roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	respChan := make(chan batchResp)
	select {
	case c <- batchSend{ba: ba, respChan: respChan}:
	case <-ctx.Done():
		return nil, roachpb.NewError(ctx.Err())
	}
	resp := <-respChan
	return resp.br, resp.pe
}

func TestBatcherSendOnSizeWithReset(t *testing.T) {
	// This test ensures that when a single batch ends up sending due to size
	// constrains its timer is successfully canceled and does not lead to a
	// nil panic due to an attempt to send a batch due to the old timer.
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	sc := make(chanSender)
	// The challenge with populating this timeout is that if we set it too short
	// then there's a chance that the batcher will send based on time and not
	// size which somewhat defeats the purpose of the test in the first place.
	// If we set the timeout too long then the test will take a long time for no
	// good reason. Instead of erring on the side of being conservative with the
	// timeout we instead allow the test to pass successfully even if it doesn't
	// exercise the path we intended. This is better than having the test block
	// forever or fail. We don't expect that it will take 5ms in the common case
	// to send two messages on a channel and if it does, oh well, the logic below
	// deals with that too and at least the test doesn't fail or hang forever.
	const wait = 5 * time.Millisecond
	b := New(Config{
		MaxIdle:         wait,
		MaxWait:         wait,
		MaxMsgsPerBatch: 2,
		Sender:          sc,
		Stopper:         stopper,
	})
	var g errgroup.Group
	sendRequest := func(rangeID roachpb.RangeID, request roachpb.Request) {
		g.Go(func() error {
			_, err := b.Send(context.Background(), rangeID, request)
			return err
		})
	}
	sendRequest(1, &roachpb.GetRequest{})
	sendRequest(1, &roachpb.GetRequest{})
	s := <-sc
	s.respChan <- batchResp{}
	// See the comment above wait. In rare cases the batch will be sent before the
	// second request can be added. In this case we need to expect that another
	// request will be sent and handle it so that the test does not block forever.
	if len(s.ba.Requests) == 1 {
		t.Logf("batch was sent due to time rather than size constraints, passing anyway")
		s := <-sc
		s.respChan <- batchResp{}
	} else {
		time.Sleep(wait)
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}
}

// TestBatchesAtTheSameTime attempts to test that batches which seem to occur at
// exactly the same moment are eventually sent. Sometimes it may be the case
// that this test fails to exercise that path if the channel send to the
// goroutine happens to take more than 10ms but in that case both batches will
// definitely get sent and the test will pass. This test was added to account
// for a bug where the internal timer would not get set if two batches had the
// same deadline. This test failed regularly before that bug was fixed.
func TestBatchesAtTheSameTime(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	sc := make(chanSender)
	start := timeutil.Now()
	then := start.Add(10 * time.Millisecond)
	b := New(Config{
		MaxIdle: 20 * time.Millisecond,
		Sender:  sc,
		Stopper: stopper,
		NowFunc: func() time.Time { return then },
	})
	const N = 20
	sendChan := make(chan Response, N)
	for i := 0; i < N; i++ {
		assert.Nil(t, b.SendWithChan(context.Background(), sendChan, roachpb.RangeID(i), &roachpb.GetRequest{}))
	}
	for i := 0; i < N; i++ {
		bs := <-sc
		bs.respChan <- batchResp{}
	}
}

func TestBackpressure(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	sc := make(chanSender)
	b := New(Config{
		MaxIdle:                   50 * time.Millisecond,
		MaxWait:                   50 * time.Millisecond,
		MaxMsgsPerBatch:           1,
		Sender:                    sc,
		Stopper:                   stopper,
		InFlightBackpressureLimit: 3,
	})

	// These 3 should all send without blocking but should put the batcher into
	// back pressure.
	sendChan := make(chan Response, 6)
	assert.Nil(t, b.SendWithChan(context.Background(), sendChan, 1, &roachpb.GetRequest{}))
	assert.Nil(t, b.SendWithChan(context.Background(), sendChan, 2, &roachpb.GetRequest{}))
	assert.Nil(t, b.SendWithChan(context.Background(), sendChan, 3, &roachpb.GetRequest{}))
	var sent int64
	send := func() {
		assert.Nil(t, b.SendWithChan(context.Background(), sendChan, 4, &roachpb.GetRequest{}))
		atomic.AddInt64(&sent, 1)
	}
	go send()
	go send()
	canReply := make(chan struct{})
	reply := func(bs batchSend) {
		<-canReply
		bs.respChan <- batchResp{}
	}
	for i := 0; i < 3; i++ {
		bs := <-sc
		go reply(bs)
		// Shouldn't need to use atomics to read sent. A race would indicate a bug.
		assert.Equal(t, int64(0), sent)
	}
	// Allow one reply to fly which should not unblock the requests.
	canReply <- struct{}{}
	runtime.Gosched() // tickle the runtime in case there might be a timing bug
	assert.Equal(t, int64(0), sent)
	canReply <- struct{}{} // now the two requests should send
	testutils.SucceedsSoon(t, func() error {
		if numSent := atomic.LoadInt64(&sent); numSent != 2 {
			return fmt.Errorf("expected %d to have been sent, so far %d", 2, numSent)
		}
		return nil
	})
	close(canReply)
	reply(<-sc)
	reply(<-sc)
}

func TestBatcherSend(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	sc := make(chanSender)
	b := New(Config{
		MaxIdle:         50 * time.Millisecond,
		MaxWait:         50 * time.Millisecond,
		MaxMsgsPerBatch: 3,
		Sender:          sc,
		Stopper:         stopper,
	})
	var g errgroup.Group
	sendRequest := func(rangeID roachpb.RangeID, request roachpb.Request) {
		g.Go(func() error {
			_, err := b.Send(context.Background(), rangeID, request)
			return err
		})
	}
	// Send 3 requests to range 2 and 2 to range 1.
	// The 3rd range 2 request will trigger immediate sending due to the
	// MaxMsgsPerBatch configuration. The range 1 batch will be sent after the
	// MaxWait timeout expires.
	sendRequest(1, &roachpb.GetRequest{})
	sendRequest(2, &roachpb.GetRequest{})
	sendRequest(1, &roachpb.GetRequest{})
	sendRequest(2, &roachpb.GetRequest{})
	sendRequest(2, &roachpb.GetRequest{})
	// Wait for the range 2 request and ensure it contains 3 requests.
	s := <-sc
	assert.Len(t, s.ba.Requests, 3)
	s.respChan <- batchResp{}
	// Wait for the range 1 request and ensure it contains 2 requests.
	s = <-sc
	assert.Len(t, s.ba.Requests, 2)
	s.respChan <- batchResp{}
	// Make sure everything gets a response.
	if err := g.Wait(); err != nil {
		t.Fatalf("expected no errors, got %v", err)
	}
}

func TestSendAfterStopped(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	sc := make(chanSender)
	b := New(Config{
		Sender:  sc,
		Stopper: stopper,
	})
	stopper.Stop(context.Background())
	_, err := b.Send(context.Background(), 1, &roachpb.GetRequest{})
	assert.Equal(t, err, stop.ErrUnavailable)
}

func TestSendAfterCanceled(t *testing.T) {
	defer leaktest.AfterTest(t)()
	sc := make(chanSender)
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	b := New(Config{
		Sender:  sc,
		Stopper: stopper,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.Send(ctx, 1, &roachpb.GetRequest{})
	assert.Equal(t, err, ctx.Err())
}

func TestStopDuringSend(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	sc := make(chanSender, 1)
	b := New(Config{
		Sender:  sc,
		Stopper: stopper,
		MaxWait: 10 * time.Millisecond,
		MaxIdle: 10 * time.Millisecond,
	})
	errChan := make(chan error)
	go func() {
		_, err := b.Send(context.Background(), 1, &roachpb.GetRequest{})
		errChan <- err
	}()
	r := <-sc
	go stopper.Stop(context.Background())
	assert.Equal(t, <-errChan, stop.ErrUnavailable)
	r.respChan <- batchResp{}
}

func TestPanicWithNilStopper(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("failed to panic with a nil Stopper")
		}
	}()
	New(Config{Sender: make(chanSender)})
}

func TestTimeoutDisabled(t *testing.T) {
	defer leaktest.AfterTest(t)()
	stopper := stop.NewStopper()
	defer stopper.Stop(context.Background())
	sc := make(chanSender)
	b := New(Config{
		MaxMsgsPerBatch: 2,
		Sender:          sc,
		Stopper:         stopper,
	})
	var g errgroup.Group
	sendRequest := func(rangeID roachpb.RangeID, request roachpb.Request) {
		g.Go(func() error {
			_, err := b.Send(context.Background(), rangeID, request)
			return err
		})
	}
	// Send 3 requests to range 2 and 2 to range 1.
	// The 3rd range 2 request will trigger immediate sending due to the
	// MaxMsgsPerBatch configuration. The range 1 batch will be sent after the
	// MaxWait timeout expires.
	sendRequest(1, &roachpb.GetRequest{})
	select {
	case <-sc:
		t.Fatalf("RequestBatcher should not sent based on time")
	case <-time.After(10 * time.Millisecond):
	}
	sendRequest(1, &roachpb.GetRequest{})
	s := <-sc
	assert.Len(t, s.ba.Requests, 2)
	s.respChan <- batchResp{}
	// Make sure everything gets a response.
	if err := g.Wait(); err != nil {
		t.Fatalf("expected no errors, got %v", err)
	}
}

func TestPanicWithNilSender(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("failed to panic with a nil Sender")
		}
	}()
	New(Config{Stopper: stop.NewStopper()})
}
