// Copyright 2018  The Cockroach Authors.
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

package nodedialer

import (
	"context"
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	circuit "github.com/znbasedb/circuitbreaker"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/rpc"
	"github.com/znbasedb/znbase/pkg/storage/closedts"
	"github.com/znbasedb/znbase/pkg/storage/closedts/ctpb"
	"github.com/znbasedb/znbase/pkg/util/grpcutil"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/stop"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
	"google.golang.org/grpc"
)

// No more than one failure to connect to a given node will be logged in the given interval.
const logPerNodeFailInterval = time.Minute

type wrappedBreaker struct {
	*circuit.Breaker
	log.EveryN
}

// An AddressResolver translates NodeIDs into addresses.
type AddressResolver func(roachpb.NodeID) (net.Addr, error)

// A Dialer wraps an *rpc.Context for dialing based on node IDs. For each node,
// it maintains a circuit breaker that prevents rapid connection attempts and
// provides hints to the callers on whether to log the outcome of the operation.
type Dialer struct {
	rpcContext *rpc.Context
	resolver   AddressResolver

	breakers [rpc.NumConnectionClasses]syncutil.IntMap // map[roachpb.NodeID]*wrappedBreaker
}

// New initializes a Dialer.
func New(rpcContext *rpc.Context, resolver AddressResolver) *Dialer {
	return &Dialer{
		rpcContext: rpcContext,
		resolver:   resolver,
	}
}

// Stopper returns this node dialer's Stopper.
// TODO(bdarnell): This is a bit of a hack for kv/transport_race.go
func (n *Dialer) Stopper() *stop.Stopper {
	return n.rpcContext.Stopper
}

// Silence lint warning because this method is only used in race builds.
var _ = (*Dialer).Stopper

// Dial returns a grpc connection to the given node. It logs whenever the
// node first becomes unreachable or reachable.
func (n *Dialer) Dial(
	ctx context.Context, nodeID roachpb.NodeID, class rpc.ConnectionClass,
) (_ *grpc.ClientConn, err error) {
	if n == nil || n.resolver == nil {
		return nil, errors.New("no node dialer configured")
	}
	// Don't trip the breaker if we're already canceled.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	breaker := n.getBreaker(nodeID, class)
	addr, err := n.resolver(nodeID)
	if err != nil {
		err = errors.Wrapf(err, "failed to resolve n%d", nodeID)
		breaker.Fail(err)
		return nil, err
	}
	return n.dial(ctx, nodeID, addr, breaker, class)
}

// DialInternalClient is a specialization of Dial for callers that
// want a roachpb.InternalClient. This supports an optimization to
// bypass the network for the local node. Returns a context.Context
// which should be used when making RPC calls on the returned server
// (This context is annotated to mark this request as in-process and
// bypass ctx.Peer checks).
func (n *Dialer) DialInternalClient(
	ctx context.Context, nodeID roachpb.NodeID, class rpc.ConnectionClass,
) (context.Context, roachpb.InternalClient, error) {
	if n == nil || n.resolver == nil {
		return nil, nil, errors.New("no node dialer configured")
	}
	addr, err := n.resolver(nodeID)
	if err != nil {
		return nil, nil, err
	}
	if localClient := n.rpcContext.GetLocalInternalClientForAddr(addr.String()); localClient != nil {
		log.VEvent(ctx, 2, "sending request to local client")

		// Create a new context from the existing one with the "local request" field set.
		// This tells the handler that this is an in-process request, bypassing ctx.Peer checks.
		localCtx := grpcutil.NewLocalRequestContext(ctx)

		return localCtx, localClient, nil
	}
	log.VEventf(ctx, 2, "sending request to %s", addr)
	conn, err := n.dial(ctx, nodeID, addr, n.getBreaker(nodeID, class), class)
	if err != nil {
		return nil, nil, err
	}
	return ctx, roachpb.NewInternalClient(conn), err
}

// dial performs the dialing of the remove connection.
func (n *Dialer) dial(
	ctx context.Context,
	nodeID roachpb.NodeID,
	addr net.Addr,
	breaker *wrappedBreaker,
	class rpc.ConnectionClass,
) (_ *grpc.ClientConn, err error) {
	// Don't trip the breaker if we're already canceled.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if breaker != nil && !breaker.Ready() {
		err = errors.Wrapf(circuit.ErrBreakerOpen, "unable to dial n%d", nodeID)
		return nil, err
	}
	defer func() {
		// Enforce a minimum interval between warnings for failed connections.
		if err != nil && ctx.Err() == nil && breaker != nil && breaker.ShouldLog() {
			log.Infof(ctx, "unable to connect to n%d: %s", nodeID, err)
		}
	}()
	conn, err := n.rpcContext.GRPCDialNode(addr.String(), nodeID, class).Connect(ctx)
	if err != nil {
		// If we were canceled during the dial, don't trip the breaker.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		err = errors.Wrapf(err, "failed to connect to n%d at %v", nodeID, addr)
		if breaker != nil {
			breaker.Fail(err)
		}
		return nil, err
	}
	// Check to see if the connection is in the transient failure state. This can
	// happen if the connection already existed, but a recent heartbeat has
	// failed and we haven't yet torn down the connection.
	if err := grpcutil.ConnectionReady(conn); err != nil {
		err = errors.Wrapf(err, "failed to check for ready connection to n%d at %v", nodeID, addr)
		if breaker != nil {
			breaker.Fail(err)
		}
		return nil, err
	}

	// TODO(bdarnell): Reconcile the different health checks and circuit breaker
	// behavior in this file. Note that this different behavior causes problems
	// for higher-levels in the system. For example, DistSQL checks for
	// ConnHealth when scheduling processors, but can then see attempts to send
	// RPCs fail when dial fails due to an open breaker. Reset the breaker here
	// as a stop-gap before the reconciliation occurs.
	if breaker != nil {
		breaker.Success()
	}
	return conn, nil
}

// ConnHealth returns nil if we have an open connection to the given node
// that succeeded on its most recent heartbeat. See the method of the same
// name on rpc.Context for more details.
func (n *Dialer) ConnHealth(nodeID roachpb.NodeID, class rpc.ConnectionClass) error {
	if n == nil || n.resolver == nil {
		return errors.New("no node dialer configured")
	}
	if !n.getBreaker(nodeID, class).Ready() {
		return circuit.ErrBreakerOpen
	}
	addr, err := n.resolver(nodeID)
	if err != nil {
		return err
	}
	// TODO(bdarnell): GRPCDial should detect local addresses and return
	// a dummy connection instead of requiring callers to do this check.
	if n.rpcContext.GetLocalInternalClientForAddr(addr.String()) != nil {
		// The local client is always considered healthy.
		return nil
	}
	conn := n.rpcContext.GRPCDialNode(addr.String(), nodeID, class)
	return conn.Health()
}

// GetCircuitBreaker retrieves the circuit breaker for connections to the given
// node. The breaker should not be mutated as this affects all connections
// dialing to that node through this NodeDialer.
func (n *Dialer) GetCircuitBreaker(
	nodeID roachpb.NodeID, class rpc.ConnectionClass,
) *circuit.Breaker {
	return n.getBreaker(nodeID, class).Breaker
}

func (n *Dialer) getBreaker(nodeID roachpb.NodeID, class rpc.ConnectionClass) *wrappedBreaker {
	breakers := &n.breakers[class]
	value, ok := breakers.Load(int64(nodeID))
	if !ok {
		name := fmt.Sprintf("rpc %v [n%d]", n.rpcContext.Config.Addr, nodeID)
		breaker := &wrappedBreaker{Breaker: n.rpcContext.NewBreaker(name), EveryN: log.Every(logPerNodeFailInterval)}
		value, _ = breakers.LoadOrStore(int64(nodeID), unsafe.Pointer(breaker))
	}
	return (*wrappedBreaker)(value)
}

type dialerAdapter Dialer

func (da *dialerAdapter) Ready(nodeID roachpb.NodeID) bool {
	return (*Dialer)(da).GetCircuitBreaker(nodeID, rpc.DefaultClass).Ready()
}

func (da *dialerAdapter) Dial(ctx context.Context, nodeID roachpb.NodeID) (ctpb.Client, error) {
	c, err := (*Dialer)(da).Dial(ctx, nodeID, rpc.DefaultClass)
	if err != nil {
		return nil, err
	}
	return ctpb.NewClosedTimestampClient(c).Get(ctx)
}

var _ closedts.Dialer = (*Dialer)(nil).CTDialer()

// CTDialer wraps the NodeDialer into a closedts.Dialer.
func (n *Dialer) CTDialer() closedts.Dialer {
	return (*dialerAdapter)(n)
}
