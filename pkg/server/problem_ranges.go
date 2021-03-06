// Copyright 2017 The Cockroach Authors.
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

package server

import (
	"context"
	"sort"

	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/server/serverpb"
	"github.com/znbasedb/znbase/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *statusServer) ProblemRanges(
	ctx context.Context, req *serverpb.ProblemRangesRequest,
) (*serverpb.ProblemRangesResponse, error) {
	ctx = s.AnnotateCtx(ctx)

	response := &serverpb.ProblemRangesResponse{
		NodeID:           s.gossip.NodeID.Get(),
		ProblemsByNodeID: make(map[roachpb.NodeID]serverpb.ProblemRangesResponse_NodeProblems),
	}

	isLiveMap := s.nodeLiveness.GetIsLiveMap()
	// If there is a specific nodeID requested, limited the responses to
	// just that node.
	if len(req.NodeID) > 0 {
		requestedNodeID, _, err := s.parseNodeID(req.NodeID)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		isLiveMap = storage.IsLiveMap{
			requestedNodeID: storage.IsLiveMapEntry{IsLive: true},
		}
	}

	type nodeResponse struct {
		nodeID roachpb.NodeID
		resp   *serverpb.RangesResponse
		err    error
	}

	responses := make(chan nodeResponse)
	// TODO(bram): consider abstracting out this repeated pattern.
	for nodeID := range isLiveMap {
		nodeID := nodeID
		if err := s.stopper.RunAsyncTask(
			ctx, "server.statusServer: requesting remote ranges",
			func(ctx context.Context) {
				status, err := s.dialNode(ctx, nodeID)
				var rangesResponse *serverpb.RangesResponse
				if err == nil {
					req := &serverpb.RangesRequest{}
					rangesResponse, err = status.Ranges(ctx, req)
				}
				response := nodeResponse{
					nodeID: nodeID,
					resp:   rangesResponse,
					err:    err,
				}

				select {
				case responses <- response:
					// Response processed.
				case <-ctx.Done():
					// Context completed, response no longer needed.
				}
			}); err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}

	for remainingResponses := len(isLiveMap); remainingResponses > 0; remainingResponses-- {
		select {
		case resp := <-responses:
			if resp.err != nil {
				response.ProblemsByNodeID[resp.nodeID] = serverpb.ProblemRangesResponse_NodeProblems{
					ErrorMessage: resp.err.Error(),
				}
				continue
			}
			var problems serverpb.ProblemRangesResponse_NodeProblems
			for _, info := range resp.resp.Ranges {
				if len(info.ErrorMessage) != 0 {
					response.ProblemsByNodeID[resp.nodeID] = serverpb.ProblemRangesResponse_NodeProblems{
						ErrorMessage: info.ErrorMessage,
					}
					continue
				}
				if info.Problems.Unavailable {
					problems.UnavailableRangeIDs =
						append(problems.UnavailableRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.LeaderNotLeaseHolder {
					problems.RaftLeaderNotLeaseHolderRangeIDs =
						append(problems.RaftLeaderNotLeaseHolderRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.NoRaftLeader {
					problems.NoRaftLeaderRangeIDs =
						append(problems.NoRaftLeaderRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.Underreplicated {
					problems.UnderreplicatedRangeIDs =
						append(problems.UnderreplicatedRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.Overreplicated {
					problems.OverreplicatedRangeIDs =
						append(problems.OverreplicatedRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.NoLease {
					problems.NoLeaseRangeIDs =
						append(problems.NoLeaseRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.QuiescentEqualsTicking {
					problems.QuiescentEqualsTickingRangeIDs =
						append(problems.QuiescentEqualsTickingRangeIDs, info.State.Desc.RangeID)
				}
				if info.Problems.RaftLogTooLarge {
					problems.RaftLogTooLargeRangeIDs =
						append(problems.RaftLogTooLargeRangeIDs, info.State.Desc.RangeID)
				}
			}
			sort.Sort(roachpb.RangeIDSlice(problems.UnavailableRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.RaftLeaderNotLeaseHolderRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.NoRaftLeaderRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.NoLeaseRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.UnderreplicatedRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.OverreplicatedRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.QuiescentEqualsTickingRangeIDs))
			sort.Sort(roachpb.RangeIDSlice(problems.RaftLogTooLargeRangeIDs))
			response.ProblemsByNodeID[resp.nodeID] = problems
		case <-ctx.Done():
			return nil, status.Errorf(codes.DeadlineExceeded, ctx.Err().Error())
		}
	}

	return response, nil
}
