// Copyright 2019  The Cockroach Authors.

package vecflow

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/opentracing/opentracing-go"
	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/rpc/nodedialer"
	"github.com/znbasedb/znbase/pkg/sql/coltypes/conv"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/rowexec"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec/execerror"
	"github.com/znbasedb/znbase/pkg/sql/flowinfra"
	"github.com/znbasedb/znbase/pkg/sql/flowinfra/vecflow/vecrpc"
	"github.com/znbasedb/znbase/pkg/sql/sessiondata"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/mon"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/tracing"
)

type vectorizedFlow struct {
	*flowinfra.FlowBase
	// operatorConcurrency is set if any operators are executed in parallel.
	operatorConcurrency bool

	// streamingMemAccounts are the memory accounts that are tracking the static
	// memory usage of the whole vectorized flow as well as all dynamic memory of
	// the streaming components.
	streamingMemAccounts []*mon.BoundAccount

	// bufferingMemMonitors are the memory monitors of the buffering components.
	bufferingMemMonitors []*mon.BytesMonitor
	// bufferingMemAccounts are the memory accounts that are tracking the dynamic
	// memory usage of the buffering components.
	bufferingMemAccounts []*mon.BoundAccount
}

var _ flowinfra.Flow = &vectorizedFlow{}

var vectorizedFlowPool = sync.Pool{
	New: func() interface{} {
		return &vectorizedFlow{}
	},
}

// NewVectorizedFlow creates a new vectorized flow given the flow base.
func NewVectorizedFlow(base *flowinfra.FlowBase) flowinfra.Flow {
	vf := vectorizedFlowPool.Get().(*vectorizedFlow)
	vf.FlowBase = base
	return vf
}

// Setup is part of the Flow interface.
func (f *vectorizedFlow) Setup(
	ctx context.Context, spec *distsqlpb.FlowSpec, opt flowinfra.FuseOpt,
) error {
	f.SetSpec(spec)
	log.VEventf(ctx, 1, "setting up vectorize flow %s", f.ID.Short())
	recordingStats := false
	if sp := opentracing.SpanFromContext(ctx); sp != nil && tracing.IsRecording(sp) {
		recordingStats = true
	}
	helper := &vectorizedFlowCreatorHelper{f: f.FlowBase}
	creator := newVectorizedFlowCreator(
		helper,
		vectorizedRemoteComponentCreator{},
		recordingStats,
		f.GetWaitGroup(),
		f.GetSyncFlowConsumer(),
		f.GetFlowCtx().Cfg.NodeDialer,
		f.GetID(),
	)
	_, err := creator.setupFlow(ctx, f.GetFlowCtx(), spec.Processors, opt)
	if err == nil {
		f.operatorConcurrency = creator.operatorConcurrency
		f.streamingMemAccounts = append(f.streamingMemAccounts, creator.streamingMemAccounts...)
		f.bufferingMemMonitors = append(f.bufferingMemMonitors, creator.bufferingMemMonitors...)
		f.bufferingMemAccounts = append(f.bufferingMemAccounts, creator.bufferingMemAccounts...)
		log.VEventf(ctx, 1, "vectorized flow setup succeeded")
		return nil
	}
	// It is (theoretically) possible that some of the memory monitoring
	// infrastructure was created even in case of an error, and we need to clean
	// that up.
	for _, memAcc := range creator.streamingMemAccounts {
		memAcc.Close(ctx)
	}
	for _, memAcc := range creator.bufferingMemAccounts {
		memAcc.Close(ctx)
	}
	for _, memMonitor := range creator.bufferingMemMonitors {
		memMonitor.Stop(ctx)
	}
	log.VEventf(ctx, 1, "failed to vectorize: %s", err)
	return err
}

// IsVectorized is part of the Flow interface.
func (f *vectorizedFlow) IsVectorized() bool {
	return true
}

// ConcurrentExecution is part of the Flow interface.
func (f *vectorizedFlow) ConcurrentExecution() bool {
	return f.operatorConcurrency || f.FlowBase.ConcurrentExecution()
}

// SetFlowCtxCanParallel is part of the Flow interface.
func (f *vectorizedFlow) SetFlowCtxCanParallel(b bool) {
	f.FlowCtx.CanParallel = b
}

// Release releases this vectorizedFlow back to the pool.
func (f *vectorizedFlow) Release() {
	*f = vectorizedFlow{}
	vectorizedFlowPool.Put(f)
}

// Cleanup is part of the Flow interface.
func (f *vectorizedFlow) Cleanup(ctx context.Context, skipCheck bool) {
	// This cleans up all the memory monitoring of the vectorized flow.
	for _, memAcc := range f.streamingMemAccounts {
		memAcc.Close(ctx)
	}
	for _, memAcc := range f.bufferingMemAccounts {
		memAcc.Close(ctx)
	}
	for _, memMonitor := range f.bufferingMemMonitors {
		memMonitor.Stop(ctx)
	}
	f.FlowBase.Cleanup(ctx, skipCheck)
	f.Release()
}

// wrapWithVectorizedStatsCollector creates a new exec.VectorizedStatsCollector
// that wraps op and connects the newly created wrapper with those
// corresponding to operators in inputs (the latter must have already been
// wrapped).
func wrapWithVectorizedStatsCollector(
	op vecexec.Operator, inputs []vecexec.Operator, pspec *distsqlpb.ProcessorSpec,
) (*vecexec.VectorizedStatsCollector, error) {
	inputWatch := timeutil.NewStopWatch()
	vsc := vecexec.NewVectorizedStatsCollector(op, pspec.ProcessorID, len(inputs) == 0, inputWatch)
	for _, input := range inputs {
		sc, ok := input.(*vecexec.VectorizedStatsCollector)
		if !ok {
			return nil, errors.New("unexpectedly an input is not collecting stats")
		}
		sc.SetOutputWatch(inputWatch)
	}
	return vsc, nil
}

// finishVectorizedStatsCollectors finishes the given stats collectors and
// outputs their stats to the trace contained in the ctx's span.
func finishVectorizedStatsCollectors(
	ctx context.Context,
	deterministicStats bool,
	vectorizedStatsCollectors []*vecexec.VectorizedStatsCollector,
	procIDs []int32,
) {
	spansByProcID := make(map[int32]opentracing.Span)
	for _, pid := range procIDs {
		// We're creating a new span for every processor setting the
		// appropriate tag so that it is displayed correctly on the flow
		// diagram.
		// TODO(yuzefovich): these spans are created and finished right
		// away which is not the way they are supposed to be used, so this
		// should be fixed.
		_, spansByProcID[pid] = tracing.ChildSpan(ctx, fmt.Sprintf("operator for processor %d", pid))
		spansByProcID[pid].SetTag(distsqlpb.ProcessorIDTagKey, pid)
	}
	for _, vsc := range vectorizedStatsCollectors {
		// TODO(yuzefovich): I'm not sure whether there are cases when
		// multiple operators correspond to a single processor. We might
		// need to do some aggregation here in that case.
		vsc.FinalizeStats()
		if deterministicStats {
			vsc.VectorizedStats.Time = 0
		}
		if vsc.ID < 0 {
			// Ignore stats collectors not associated with a processor.
			continue
		}
		tracing.SetSpanStats(spansByProcID[vsc.ID], &vsc.VectorizedStats)
	}
	for _, sp := range spansByProcID {
		sp.Finish()
	}
}

type runFn func(context.Context, context.CancelFunc)

// flowCreatorHelper contains all the logic needed to add the vectorized
// infrastructure to be run asynchronously as well as to perform some sanity
// checks.
type flowCreatorHelper interface {
	// addStreamEndpoint stores information about an inbound stream.
	addStreamEndpoint(distsqlpb.StreamID, *vecrpc.Inbox, *sync.WaitGroup)
	// checkInboundStreamID checks that the provided stream ID has not been seen
	// yet.
	checkInboundStreamID(distsqlpb.StreamID) error
	// accumulateAsyncComponent stores a component (either a router or an outbox)
	// to be run asynchronously.
	accumulateAsyncComponent(runFn)
	// addMaterializer adds a materializer to the flow.
	addMaterializer(*vecexec.Materializer)
	// getCancelFlowFn returns a flow cancellation function.
	getCancelFlowFn() context.CancelFunc
}

// opDAGWithMetaSources is a helper struct that stores an operator DAG as well
// as the metadataSources in this DAG that need to be drained.
type opDAGWithMetaSources struct {
	rootOperator    vecexec.Operator
	metadataSources []distsqlpb.MetadataSource
}

// remoteComponentCreator is an interface that abstracts the constructors for
// several components in a remote flow. Mostly for testing purposes.
type remoteComponentCreator interface {
	newOutbox(
		allocator *vecexec.Allocator,
		input vecexec.Operator,
		typs []coltypes.T,
		metadataSources []distsqlpb.MetadataSource,
	) (*vecrpc.Outbox, error)
	newInbox(allocator *vecexec.Allocator, typs []coltypes.T, streamID distsqlpb.StreamID) (*vecrpc.Inbox, error)
}

type vectorizedRemoteComponentCreator struct{}

func (vectorizedRemoteComponentCreator) newOutbox(
	allocator *vecexec.Allocator,
	input vecexec.Operator,
	typs []coltypes.T,
	metadataSources []distsqlpb.MetadataSource,
) (*vecrpc.Outbox, error) {
	return vecrpc.NewOutbox(allocator, input, typs, metadataSources)
}

func (vectorizedRemoteComponentCreator) newInbox(
	allocator *vecexec.Allocator, typs []coltypes.T, streamID distsqlpb.StreamID,
) (*vecrpc.Inbox, error) {
	return vecrpc.NewInbox(allocator, typs, streamID)
}

// vectorizedFlowCreator performs all the setup of vectorized flows. Depending
// on embedded flowCreatorHelper, it can either do the actual setup in order
// to run the flow or do the setup needed to check that the flow is supported
// through the vectorized engine.
type vectorizedFlowCreator struct {
	flowCreatorHelper
	remoteComponentCreator

	streamIDToInputOp              map[distsqlpb.StreamID]opDAGWithMetaSources
	recordingStats                 bool
	vectorizedStatsCollectorsQueue []*vecexec.VectorizedStatsCollector
	procIDs                        []int32
	waitGroup                      *sync.WaitGroup
	syncFlowConsumer               runbase.RowReceiver
	nodeDialer                     *nodedialer.Dialer
	flowID                         distsqlpb.FlowID

	// numOutboxes counts how many exec.Outboxes have been set up on this node.
	// It must be accessed atomically.
	numOutboxes       int32
	materializerAdded bool

	// leaves accumulates all operators that have no further outputs on the
	// current node, for the purposes of EXPLAIN output.
	leaves []runbase.OpNode
	// operatorConcurrency is set if any operators are executed in parallel.
	operatorConcurrency bool
	// streamingMemAccounts contains all memory accounts of the non-buffering
	// components in the vectorized flow.
	streamingMemAccounts []*mon.BoundAccount
	// bufferingMemMonitors contains all memory monitors of the buffering
	// components in the vectorized flow.
	bufferingMemMonitors []*mon.BytesMonitor
	// bufferingMemAccounts contains all memory accounts of the buffering
	// components in the vectorized flow.
	bufferingMemAccounts []*mon.BoundAccount
}

func newVectorizedFlowCreator(
	helper flowCreatorHelper,
	componentCreator remoteComponentCreator,
	recordingStats bool,
	waitGroup *sync.WaitGroup,
	syncFlowConsumer runbase.RowReceiver,
	nodeDialer *nodedialer.Dialer,
	flowID distsqlpb.FlowID,
) *vectorizedFlowCreator {
	return &vectorizedFlowCreator{
		flowCreatorHelper:              helper,
		remoteComponentCreator:         componentCreator,
		streamIDToInputOp:              make(map[distsqlpb.StreamID]opDAGWithMetaSources),
		recordingStats:                 recordingStats,
		vectorizedStatsCollectorsQueue: make([]*vecexec.VectorizedStatsCollector, 0, 2),
		procIDs:                        make([]int32, 0, 2),
		waitGroup:                      waitGroup,
		syncFlowConsumer:               syncFlowConsumer,
		nodeDialer:                     nodeDialer,
		flowID:                         flowID,
	}
}

// newStreamingMemAccount creates a new memory account bound to the monitor in
// flowCtx and accumulates it into streamingMemAccounts slice.
func (s *vectorizedFlowCreator) newStreamingMemAccount(flowCtx *runbase.FlowCtx) *mon.BoundAccount {
	streamingMemAccount := flowCtx.EvalCtx.Mon.MakeBoundAccount()
	s.streamingMemAccounts = append(s.streamingMemAccounts, &streamingMemAccount)
	return &streamingMemAccount
}

// setupRemoteOutputStream sets up an Outbox that will operate according to
// the given StreamEndpointSpec. It will also drain all MetadataSources in the
// metadataSourcesQueue.
func (s *vectorizedFlowCreator) setupRemoteOutputStream(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	op vecexec.Operator,
	outputTyps []coltypes.T,
	stream *distsqlpb.StreamEndpointSpec,
	metadataSourcesQueue []distsqlpb.MetadataSource,
) (runbase.OpNode, error) {
	outbox, err := s.remoteComponentCreator.newOutbox(
		vecexec.NewAllocator(ctx, s.newStreamingMemAccount(flowCtx)),
		op, outputTyps, metadataSourcesQueue,
	)
	if err != nil {
		return nil, err
	}
	atomic.AddInt32(&s.numOutboxes, 1)
	run := func(ctx context.Context, cancelFn context.CancelFunc) {
		outbox.Run(ctx, s.nodeDialer, stream.TargetNodeID, s.flowID, stream.StreamID, cancelFn)
		currentOutboxes := atomic.AddInt32(&s.numOutboxes, -1)
		// When the last Outbox on this node exits, we want to make sure that
		// everything is shutdown; namely, we need to call cancelFn if:
		// - it is the last Outbox
		// - there is no root materializer on this node (if it were, it would take
		// care of the cancellation itself)
		// - cancelFn is non-nil (it can be nil in tests).
		// Calling cancelFn will cancel the context that all infrastructure on this
		// node is listening on, so it will shut everything down.
		if currentOutboxes == 0 && !s.materializerAdded && cancelFn != nil {
			cancelFn()
		}
	}
	s.accumulateAsyncComponent(run)
	return outbox, nil
}

// setupRouter sets up a vectorized hash router according to the output router
// spec. If the outputs are local, these are added to s.streamIDToInputOp to be
// used as inputs in further planning. metadataSourcesQueue is passed along to
// any outboxes created to be drained, or stored in streamIDToInputOp for any
// local outputs to pass that responsibility along. In any case,
// metadataSourcesQueue will always be fully consumed.
// NOTE: This method supports only BY_HASH routers. Callers should handle
// PASS_THROUGH routers separately.
func (s *vectorizedFlowCreator) setupRouter(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	input vecexec.Operator,
	outputTyps []coltypes.T,
	output *distsqlpb.OutputRouterSpec,
	metadataSourcesQueue []distsqlpb.MetadataSource,
) error {
	if output.Type != distsqlpb.OutputRouterSpec_BY_HASH {
		return errors.Errorf("vectorized output router type %s unsupported", output.Type)
	}

	// TODO(asubiotto): Change hashRouter's hashCols to be uint32s.
	hashCols := make([]int, len(output.HashColumns))
	for i := range hashCols {
		hashCols[i] = int(output.HashColumns[i])
	}
	hashRouterMemMonitor := runbase.NewLimitedMonitor(
		ctx, flowCtx.EvalCtx.Mon, flowCtx.Cfg, "hash-router-limited",
	)
	hashRouterMemAccount := hashRouterMemMonitor.MakeBoundAccount()
	s.bufferingMemMonitors = append(s.bufferingMemMonitors, hashRouterMemMonitor)
	s.bufferingMemAccounts = append(s.bufferingMemAccounts, &hashRouterMemAccount)
	router, outputs := vecexec.NewHashRouter(
		vecexec.NewAllocator(ctx, &hashRouterMemAccount), input, outputTyps,
		hashCols, len(output.Streams),
	)
	runRouter := func(ctx context.Context, _ context.CancelFunc) {
		router.Run(ctx)
	}
	s.accumulateAsyncComponent(runRouter)

	// Append the router to the metadata sources.
	metadataSourcesQueue = append(metadataSourcesQueue, router)

	foundLocalOutput := false
	for i, op := range outputs {
		stream := &output.Streams[i]
		switch stream.Type {
		case distsqlpb.StreamEndpointSpec_SYNC_RESPONSE:
			return errors.Errorf("unexpected sync response output when setting up router")
		case distsqlpb.StreamEndpointSpec_REMOTE:
			if _, err := s.setupRemoteOutputStream(
				ctx, flowCtx, op, outputTyps, stream, metadataSourcesQueue,
			); err != nil {
				return err
			}
		case distsqlpb.StreamEndpointSpec_LOCAL:
			foundLocalOutput = true
			if s.recordingStats {
				// Wrap local outputs with vectorized stats collectors when recording
				// stats. This is mostly for compatibility but will provide some useful
				// information (e.g. output stall time).
				var err error
				op, err = wrapWithVectorizedStatsCollector(
					op, nil /* inputs */, &distsqlpb.ProcessorSpec{ProcessorID: -1},
				)
				if err != nil {
					return err
				}
			}
			s.streamIDToInputOp[stream.StreamID] = opDAGWithMetaSources{rootOperator: op, metadataSources: metadataSourcesQueue}
		}
		// Either the metadataSourcesQueue will be drained by an outbox or we
		// created an opDAGWithMetaSources to pass along these metadataSources. We don't need to
		// worry about metadata sources for following iterations of the loop.
		metadataSourcesQueue = nil
	}
	if !foundLocalOutput {
		// No local output means that our router is a leaf node.
		s.leaves = append(s.leaves, router)
	}
	return nil
}

// setupInput sets up one or more input operators (local or remote) and a
// synchronizer to expose these separate streams as one exec.Operator which is
// returned. If s.recordingStats is true, these inputs and synchronizer are
// wrapped in stats collectors if not done so, although these stats are not
// exposed as of yet. Inboxes that are created are also returned as
// []distqlpb.MetadataSource so that any remote metadata can be read through
// calling DrainMeta.
func (s *vectorizedFlowCreator) setupInput(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	input distsqlpb.InputSyncSpec,
	opt flowinfra.FuseOpt,
) (op vecexec.Operator, _ []distsqlpb.MetadataSource, _ error) {
	inputStreamOps := make([]vecexec.Operator, 0, len(input.Streams))
	metaSources := make([]distsqlpb.MetadataSource, 0, len(input.Streams))
	for _, inputStream := range input.Streams {
		switch inputStream.Type {
		case distsqlpb.StreamEndpointSpec_LOCAL:
			in := s.streamIDToInputOp[inputStream.StreamID]
			inputStreamOps = append(inputStreamOps, in.rootOperator)
			metaSources = append(metaSources, in.metadataSources...)
		case distsqlpb.StreamEndpointSpec_REMOTE:
			// If the input is remote, the input operator does not exist in
			// streamIDToInputOp. Create an inbox.
			if err := s.checkInboundStreamID(inputStream.StreamID); err != nil {
				return nil, nil, err
			}
			typs, _ := conv.FromColumnType1s(input.ColumnTypes)
			inbox, err := s.remoteComponentCreator.newInbox(
				vecexec.NewAllocator(ctx, s.newStreamingMemAccount(flowCtx)),
				typs, inputStream.StreamID,
			)
			if err != nil {
				return nil, nil, err
			}
			s.addStreamEndpoint(inputStream.StreamID, inbox, s.waitGroup)
			metaSources = append(metaSources, inbox)
			op = inbox
			if s.recordingStats {
				op, err = wrapWithVectorizedStatsCollector(
					inbox,
					nil, /* inputs */
					// TODO(asubiotto): Vectorized stats collectors currently expect a
					// processor ID. These stats will not be shown until we extend stats
					// collectors to take in a stream ID.
					&distsqlpb.ProcessorSpec{
						ProcessorID: -1,
					},
				)
				if err != nil {
					return nil, nil, err
				}
			}
			inputStreamOps = append(inputStreamOps, op)
		default:
			return nil, nil, errors.Errorf("unsupported input stream type %s", inputStream.Type)
		}
	}
	op = inputStreamOps[0]
	if len(inputStreamOps) > 1 {
		statsInputs := inputStreamOps
		typs, err := conv.FromColumnType1s(input.ColumnTypes)
		if err != nil {
			return nil, nil, err
		}
		if input.Type == distsqlpb.InputSyncSpec_ORDERED {
			op = vecexec.NewOrderedSynchronizer(
				vecexec.NewAllocator(ctx, s.newStreamingMemAccount(flowCtx)),
				inputStreamOps, typs, distsqlpb.ConvertToColumnOrdering(input.Ordering),
			)
		} else {
			if opt == flowinfra.FuseAggressively {
				op = vecexec.NewSerialUnorderedSynchronizer(inputStreamOps, typs)
			} else {
				op = vecexec.NewParallelUnorderedSynchronizer(inputStreamOps, typs, s.waitGroup)
				s.operatorConcurrency = true
			}
			// Don't use the unordered synchronizer's inputs for stats collection
			// given that they run concurrently. The stall time will be collected
			// instead.
			statsInputs = nil
		}
		if s.recordingStats {
			// TODO(asubiotto): Once we have IDs for synchronizers, plumb them into
			// this stats collector to display stats.
			var err error
			op, err = wrapWithVectorizedStatsCollector(op, statsInputs, &distsqlpb.ProcessorSpec{ProcessorID: -1})
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return op, metaSources, nil
}

// setupOutput sets up any necessary infrastructure according to the output
// spec of pspec. The metadataSourcesQueue is fully consumed by either
// connecting it to a component that can drain these MetadataSources (root
// materializer or outbox) or storing it in streamIDToInputOp with the given op
// to be processed later.
// NOTE: The caller must not reuse the metadataSourcesQueue.
func (s *vectorizedFlowCreator) setupOutput(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	pspec *distsqlpb.ProcessorSpec,
	op vecexec.Operator,
	opOutputTypes []coltypes.T,
	metadataSourcesQueue []distsqlpb.MetadataSource,
) error {
	output := &pspec.Output[0]
	if output.Type != distsqlpb.OutputRouterSpec_PASS_THROUGH {
		return s.setupRouter(
			ctx,
			flowCtx,
			op,
			opOutputTypes,
			output,
			// Pass in a copy of the queue to reset metadataSourcesQueue for
			// further appends without overwriting.
			metadataSourcesQueue,
		)
	}

	if len(output.Streams) != 1 {
		return errors.Errorf("unsupported multi outputstream proc (%d streams)", len(output.Streams))
	}
	outputStream := &output.Streams[0]
	switch outputStream.Type {
	case distsqlpb.StreamEndpointSpec_LOCAL:
		s.streamIDToInputOp[outputStream.StreamID] = opDAGWithMetaSources{rootOperator: op, metadataSources: metadataSourcesQueue}
	case distsqlpb.StreamEndpointSpec_REMOTE:
		// Set up an Outbox. Note that we pass in a copy of metadataSourcesQueue
		// so that we can reset it below and keep on writing to it.
		if s.recordingStats {
			// If recording stats, we add a metadata source that will generate all
			// stats data as metadata for the stats collectors created so far.
			vscs := append([]*vecexec.VectorizedStatsCollector(nil), s.vectorizedStatsCollectorsQueue...)
			s.vectorizedStatsCollectorsQueue = s.vectorizedStatsCollectorsQueue[:0]
			metadataSourcesQueue = append(
				metadataSourcesQueue,
				distsqlpb.CallbackMetadataSource{
					DrainMetaCb: func(ctx context.Context) []distsqlpb.ProducerMetadata {
						// TODO(asubiotto): Who is responsible for the recording of the
						// parent context?
						// Start a separate recording so that GetRecording will return
						// the recordings for only the child spans containing stats.
						ctx, span := tracing.ChildSpanSeparateRecording(ctx, "")
						finishVectorizedStatsCollectors(ctx, flowCtx.Cfg.TestingKnobs.DeterministicStats, vscs, s.procIDs)
						return []distsqlpb.ProducerMetadata{{TraceData: tracing.GetRecording(span)}}
					},
				},
			)
		}
		outbox, err := s.setupRemoteOutputStream(ctx, flowCtx, op, opOutputTypes, outputStream, metadataSourcesQueue)
		if err != nil {
			return err
		}
		// An outbox is a leaf: there's nothing that sees it as an input on this
		// node.
		s.leaves = append(s.leaves, outbox)
	case distsqlpb.StreamEndpointSpec_SYNC_RESPONSE:
		if s.syncFlowConsumer == nil {
			return errors.New("syncFlowConsumer unset, unable to create materializer")
		}
		// Make the materializer, which will write to the given receiver.
		columnTypes := s.syncFlowConsumer.Types()
		var outputStatsToTrace func()
		if s.recordingStats {
			// Make a copy given that vectorizedStatsCollectorsQueue is reset and
			// appended to.
			vscq := append([]*vecexec.VectorizedStatsCollector(nil), s.vectorizedStatsCollectorsQueue...)
			outputStatsToTrace = func() {
				finishVectorizedStatsCollectors(
					ctx, flowCtx.Cfg.TestingKnobs.DeterministicStats, vscq, s.procIDs,
				)
			}
		}
		typs, _ := sqlbase.ToType1s(columnTypes)
		proc, err := vecexec.NewMaterializer(
			flowCtx,
			pspec.ProcessorID,
			op,
			typs,
			&distsqlpb.PostProcessSpec{},
			s.syncFlowConsumer,
			metadataSourcesQueue,
			outputStatsToTrace,
			s.getCancelFlowFn,
		)
		if err != nil {
			return err
		}
		s.vectorizedStatsCollectorsQueue = s.vectorizedStatsCollectorsQueue[:0]
		// A materializer is a leaf.
		s.leaves = append(s.leaves, proc)
		s.addMaterializer(proc)
		s.materializerAdded = true
	default:
		return errors.Errorf("unsupported output stream type %s", outputStream.Type)
	}
	return nil
}

func (s *vectorizedFlowCreator) setupFlow(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	processorSpecs []distsqlpb.ProcessorSpec,
	opt flowinfra.FuseOpt,
) (leaves []runbase.OpNode, err error) {
	streamIDToSpecIdx := make(map[distsqlpb.StreamID]int)
	// queue is a queue of indices into processorSpecs, for topologically
	// ordered processing.
	queue := make([]int, 0, len(processorSpecs))
	for i := range processorSpecs {
		hasLocalInput := false
		for j := range processorSpecs[i].Input {
			input := &processorSpecs[i].Input[j]
			for k := range input.Streams {
				stream := &input.Streams[k]
				streamIDToSpecIdx[stream.StreamID] = i
				if stream.Type != distsqlpb.StreamEndpointSpec_REMOTE {
					hasLocalInput = true
				}
			}
		}
		if hasLocalInput {
			continue
		}
		// Queue all processors with either no inputs or remote inputs.
		queue = append(queue, i)
	}

	inputs := make([]vecexec.Operator, 0, 2)
	for len(queue) > 0 {
		pspec := &processorSpecs[queue[0]]
		queue = queue[1:]
		if len(pspec.Output) > 1 {
			return nil, errors.Errorf("unsupported multi-output proc (%d outputs)", len(pspec.Output))
		}

		// metadataSourcesQueue contains all the MetadataSources that need to be
		// drained. If in a given loop iteration no component that can drain
		// metadata from these sources is found, the metadataSourcesQueue should be
		// added as part of one of the last unconnected inputDAGs in
		// streamIDToInputOp. This is to avoid cycles.
		metadataSourcesQueue := make([]distsqlpb.MetadataSource, 0, 1)
		inputs = inputs[:0]
		for i := range pspec.Input {
			input, metadataSources, err := s.setupInput(ctx, flowCtx, pspec.Input[i], opt)
			if err != nil {
				return nil, err
			}
			metadataSourcesQueue = append(metadataSourcesQueue, metadataSources...)
			inputs = append(inputs, input)
		}

		args := vecexec.NewColOperatorArgs{
			Spec:                 pspec,
			Inputs:               inputs,
			StreamingMemAccount:  s.newStreamingMemAccount(flowCtx),
			ProcessorConstructor: rowexec.NewProcessor,
		}
		result, err := vecexec.NewColOperator(ctx, flowCtx, args)
		// Even when err is non-nil, it is possible that the buffering memory
		// monitor and account have been created, so we always want to accumulate
		// them for a proper cleanup.
		s.bufferingMemMonitors = append(s.bufferingMemMonitors, result.BufferingOpMemMonitors...)
		s.bufferingMemAccounts = append(s.bufferingMemAccounts, result.BufferingOpMemAccounts...)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to vectorize execution plan")
		}
		if flowCtx.Cfg != nil && flowCtx.Cfg.TestingKnobs.EnableVectorizedInvariantsChecker {
			result.Op = vecexec.NewInvariantsChecker(result.Op, len(result.ColumnTypes))
		}
		if flowCtx.EvalCtx.SessionData.VectorizeMode == sessiondata.VectorizeAuto &&
			!result.IsStreaming {
			return nil, errors.Errorf("non-streaming operator encountered when vectorize=auto")
		}
		// We created a streaming memory account when calling NewColOperator above,
		// so there is definitely at least one memory account, and it doesn't
		// matter which one we grow.
		if err = s.streamingMemAccounts[0].Grow(ctx, int64(result.InternalMemUsage)); err != nil {
			return nil, errors.Wrapf(err, "not enough memory to setup vectorized plan")
		}
		metadataSourcesQueue = append(metadataSourcesQueue, result.MetadataSources...)

		op := result.Op
		if s.recordingStats {
			vsc, err := wrapWithVectorizedStatsCollector(op, inputs, pspec)
			if err != nil {
				return nil, err
			}
			s.vectorizedStatsCollectorsQueue = append(s.vectorizedStatsCollectorsQueue, vsc)
			s.procIDs = append(s.procIDs, pspec.ProcessorID)
			op = vsc
		}

		if flowCtx.EvalCtx.SessionData.VectorizeMode == sessiondata.VectorizeAuto &&
			pspec.Output[0].Type == distsqlpb.OutputRouterSpec_BY_HASH {
			// exec.HashRouter can do unlimited buffering, and it is present in the
			// flow, so we don't want to run such a flow via the vectorized engine
			// when vectorize=auto.
			return nil, errors.Errorf("hash router encountered when vectorize=auto")
		}
		opOutputTypes, _ := sqlbase.ToColtysFormtype1s(result.ColumnTypes)
		if err != nil {
			return nil, err
		}
		if err = s.setupOutput(
			ctx, flowCtx, pspec, op, opOutputTypes, metadataSourcesQueue,
		); err != nil {
			return nil, err
		}

		// Now queue all outputs from this op whose inputs are already all
		// populated.
	NEXTOUTPUT:
		for i := range pspec.Output {
			for j := range pspec.Output[i].Streams {
				stream := &pspec.Output[i].Streams[j]
				if stream.Type != distsqlpb.StreamEndpointSpec_LOCAL {
					continue
				}
				procIdx, ok := streamIDToSpecIdx[stream.StreamID]
				if !ok {
					return nil, errors.Errorf("couldn't find stream %d", stream.StreamID)
				}
				outputSpec := &processorSpecs[procIdx]
				for k := range outputSpec.Input {
					for l := range outputSpec.Input[k].Streams {
						stream := outputSpec.Input[k].Streams[l]
						if stream.Type == distsqlpb.StreamEndpointSpec_REMOTE {
							// Remote streams are not present in streamIDToInputOp. The
							// Inboxes that consume these streams are created at the same time
							// as the operator that needs them, so skip the creation check for
							// this input.
							continue
						}
						if _, ok := s.streamIDToInputOp[stream.StreamID]; !ok {
							continue NEXTOUTPUT
						}
					}
				}
				// We found an input op for every single stream in this output. Queue
				// it for processing.
				queue = append(queue, procIdx)
			}
		}
	}

	if len(s.vectorizedStatsCollectorsQueue) > 0 {
		execerror.VectorizedInternalPanic("not all vectorized stats collectors have been processed")
	}
	return s.leaves, nil
}

type vectorizedInboundStreamHandler struct {
	*vecrpc.Inbox
}

var _ flowinfra.InboundStreamHandler = vectorizedInboundStreamHandler{}

// Run is part of the InboundStreamHandler interface.
func (s vectorizedInboundStreamHandler) Run(
	ctx context.Context,
	stream distsqlpb.DistSQL_FlowStreamServer,
	_ *distsqlpb.ProducerMessage,
	_ *flowinfra.FlowBase,
) error {
	return s.RunWithStream(ctx, stream)
}

// Timeout is part of the InboundStreamHandler interface.
func (s vectorizedInboundStreamHandler) Timeout(err error) {
	s.Inbox.Timeout(err)
}

// vectorizedFlowCreatorHelper is a flowCreatorHelper that sets up all the
// vectorized infrastructure to be actually run.
type vectorizedFlowCreatorHelper struct {
	f *flowinfra.FlowBase
}

var _ flowCreatorHelper = &vectorizedFlowCreatorHelper{}

func (r *vectorizedFlowCreatorHelper) addStreamEndpoint(
	streamID distsqlpb.StreamID, inbox *vecrpc.Inbox, wg *sync.WaitGroup,
) {
	r.f.AddRemoteStream(streamID, flowinfra.NewInboundStreamInfo(
		vectorizedInboundStreamHandler{inbox},
		wg,
	))
}

func (r *vectorizedFlowCreatorHelper) checkInboundStreamID(sid distsqlpb.StreamID) error {
	return r.f.CheckInboundStreamID(sid)
}

func (r *vectorizedFlowCreatorHelper) accumulateAsyncComponent(run runFn) {
	r.f.AddStartable(
		flowinfra.StartableFn(func(ctx context.Context, wg *sync.WaitGroup, cancelFn context.CancelFunc) {
			if wg != nil {
				wg.Add(1)
			}
			go func() {
				run(ctx, cancelFn)
				if wg != nil {
					wg.Done()
				}
			}()
		}))
}

func (r *vectorizedFlowCreatorHelper) addMaterializer(m *vecexec.Materializer) {
	processors := make([]runbase.Processor, 1)
	processors[0] = m
	r.f.SetProcessors(processors)
}

func (r *vectorizedFlowCreatorHelper) getCancelFlowFn() context.CancelFunc {
	return r.f.GetCancelFlowFn()
}

// noopFlowCreatorHelper is a flowCreatorHelper that only performs sanity
// checks.
type noopFlowCreatorHelper struct {
	inboundStreams map[distsqlpb.StreamID]struct{}
}

var _ flowCreatorHelper = &noopFlowCreatorHelper{}

func newNoopFlowCreatorHelper() *noopFlowCreatorHelper {
	return &noopFlowCreatorHelper{
		inboundStreams: make(map[distsqlpb.StreamID]struct{}),
	}
}

func (r *noopFlowCreatorHelper) addStreamEndpoint(
	streamID distsqlpb.StreamID, _ *vecrpc.Inbox, _ *sync.WaitGroup,
) {
	r.inboundStreams[streamID] = struct{}{}
}

func (r *noopFlowCreatorHelper) checkInboundStreamID(sid distsqlpb.StreamID) error {
	if _, found := r.inboundStreams[sid]; found {
		return errors.Errorf("inbound stream %d already exists in map", sid)
	}
	return nil
}

func (r *noopFlowCreatorHelper) accumulateAsyncComponent(runFn) {}

func (r *noopFlowCreatorHelper) addMaterializer(*vecexec.Materializer) {}

func (r *noopFlowCreatorHelper) getCancelFlowFn() context.CancelFunc {
	return nil
}

// SupportsVectorized checks whether flow is supported by the vectorized engine
// and returns an error if it isn't. Note that it does so by setting up the
// full flow without running the components asynchronously.
// It returns a list of the leaf operators of all flows for the purposes of
// EXPLAIN output.
func SupportsVectorized(
	ctx context.Context,
	flowCtx *runbase.FlowCtx,
	processorSpecs []distsqlpb.ProcessorSpec,
	fuseOpt flowinfra.FuseOpt,
) (leaves []runbase.OpNode, err error) {
	creator := newVectorizedFlowCreator(
		newNoopFlowCreatorHelper(),
		vectorizedRemoteComponentCreator{},
		false,                 /* recordingStats */
		nil,                   /* waitGroup */
		&runbase.RowChannel{}, /* syncFlowConsumer */
		nil,                   /* nodeDialer */
		distsqlpb.FlowID{},
	)
	// We create an unlimited memory account because we're interested whether the
	// flow is supported via the vectorized engine in general (without paying
	// attention to the memory since it is node-dependent in the distributed
	// case).
	memoryMonitor := mon.MakeMonitor(
		"supports-vectorized",
		mon.MemoryResource,
		nil,           /* curCount */
		nil,           /* maxHist */
		-1,            /* increment */
		math.MaxInt64, /* noteworthy */
		flowCtx.Cfg.Settings,
	)
	memoryMonitor.Start(ctx, nil, mon.MakeStandaloneBudget(math.MaxInt64))
	defer memoryMonitor.Stop(ctx)
	defer func() {
		for _, memAcc := range creator.streamingMemAccounts {
			memAcc.Close(ctx)
		}
		for _, memAcc := range creator.bufferingMemAccounts {
			memAcc.Close(ctx)
		}
		for _, memMon := range creator.bufferingMemMonitors {
			memMon.Stop(ctx)
		}
	}()
	if vecErr := execerror.CatchVecRuntimeError(func() {
		leaves, err = creator.setupFlow(ctx, flowCtx, processorSpecs, fuseOpt)
	}); vecErr != nil {
		return leaves, vecErr
	}
	return leaves, err
}

// VectorizeAlwaysException is an object that returns whether or not execution
// should continue if vectorize=experimental_always and an error occurred when
// setting up the vectorized flow. Consider the case in which
// vectorize=experimental_always. The user must be able to unset this session
// variable without getting an error.
type VectorizeAlwaysException interface {
	// IsException returns whether this object should be an exception to the rule
	// that an inability to run this node in a vectorized flow should produce an
	// error.
	// TODO(asubiotto): This is the cleanest way I can think of to not error out
	// on SET statements when running with vectorize = experimental_always. If
	// there is a better way, we should get rid of this interface.
	IsException() bool
}
