// Copyright 2015  The Cockroach Authors.
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

package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/server/serverpb"
	"github.com/znbasedb/znbase/pkg/util/retry"
)

const (
	localTimeFormat = "2006-01-02 15:04:05.999999-07:00"
)

var lsNodesColumnHeaders = []string{
	"id",
}

var lsNodesCmd = &cobra.Command{
	Use:   "ls",
	Short: "lists the IDs of all nodes in the cluster",
	Long: `
Display the node IDs for all active (that is, running and not decommissioned) members of the cluster.
To retrieve the IDs for inactive members, see 'node status --decommission'.
	`,
	Args: cobra.NoArgs,
	RunE: MaybeDecorateGRPCError(runLsNodes),
}

func runLsNodes(cmd *cobra.Command, args []string) error {
	conn, err := getPasswordAndMakeSQLClient("znbase node ls")
	if err != nil {
		return err
	}
	defer conn.Close()

	if cliCtx.cmdTimeout != 0 {
		if err := conn.Exec(fmt.Sprintf("SET statement_timeout=%d", cliCtx.cmdTimeout), nil); err != nil {
			return err
		}
	}

	_, rows, err := runQuery(
		conn,
		makeQuery(`SELECT node_id FROM zbdb_internal.gossip_liveness
               WHERE decommissioning = false OR split_part(expiration,',',1)::decimal > (now()-timestamp'1970-01-01')::decimal`),
		false,
	)
	if err != nil {
		return err
	}

	return printQueryOutput(os.Stdout, lsNodesColumnHeaders, newRowSliceIter(rows, "r"))
}

var baseNodeColumnHeaders = []string{
	"id",
	"address",
	"build",
	"started_at",
	"updated_at",
	"is_available",
	"is_live",
}

var statusNodesColumnHeadersForRanges = []string{
	"replicas_leaders",
	"replicas_leaseholders",
	"ranges",
	"ranges_unavailable",
	"ranges_underreplicated",
}

var statusNodesColumnHeadersForStats = []string{
	"live_bytes",
	"key_bytes",
	"value_bytes",
	"intent_bytes",
	"system_bytes",
}

var statusNodesColumnHeadersForDecommission = []string{
	"gossiped_replicas",
	"is_decommissioning",
	"is_draining",
}

var statusNodeCmd = &cobra.Command{
	Use:   "status [<node id>]",
	Short: "shows the status of a node or all nodes",
	Long: `
If a node ID is specified, this will show the status for the corresponding node. If no node ID
is specified, this will display the status for all nodes in the cluster.
	`,
	Args: cobra.MaximumNArgs(1),
	RunE: MaybeDecorateGRPCError(runStatusNode),
}

// runUpdateStore update store state
func runUpdateStore(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, finish, err := getAdminClient(ctx)
	if err != nil {
		return err
	}
	defer finish()
	store := args[0]
	comma := strings.Index(store, "=")
	if comma == -1 {
		return errors.Errorf("There is something wrong with the input store. The reasonable input should be 'storeID = 'NUM'','NUM' is the ID of the store you want to modify \n for example : \n 'storeID = 1'")
	}
	storeValue := store[comma+1:]
	storePrefix := store[:comma]
	// Check whether the input store format is correct.
	if storePrefix != "storeID" {
		return errors.Errorf("The input format of store  is wrong")
	}
	storeID, err := strconv.ParseInt(storeValue, 10, 32)
	if err != nil {
		return errors.Errorf("could not parse store id %s", storeValue)
	}
	state := args[1]
	comma = strings.Index(state, "=")
	if comma == -1 {
		return errors.Errorf("There is something wrong with the input state. The reasonable input should be 'state = 'STA''.'STA' is the state of the store you want to modify \n for example :\n 'state = ENABLE'")
	}
	stateValue := state[comma+1:]
	statePrefix := state[:comma]
	// Check whether the input state format is correct.
	if statePrefix != "state" {
		return errors.Errorf("The input format of state  is wrong")
	}
	// Check whether value is available.
	if !base.IsAvailable(stateValue) {
		return errors.Errorf("not valid state  %s", stateValue)
	}
	StateVal := base.TransformStringToStoreState(stateValue)
	var response *serverpb.StoreStateResponse
	if response, err = c.UpdateStoreState(ctx, &serverpb.StoreStateRequest{StoreID: roachpb.StoreID(storeID), State: StateVal}); err != nil {
		return err
	}
	res := reflect.ValueOf(response).Elem()
	var cols []string
	var row []string
	for i := 0; i < res.NumField(); i++ {
		cols = append(cols, res.Type().Field(i).Name)
		row = append(row, fmt.Sprintf("%v", res.Field(i).Interface()))
	}
	_ = printQueryOutput(os.Stdout, cols, newRowSliceIter([][]string{row}, ""))

	return nil
}

func runStatusNode(cmd *cobra.Command, args []string) error {
	_, rows, err := runStatusNodeInner(nodeCtx.statusShowDecommission || nodeCtx.statusShowAll, args)
	if err != nil {
		return err
	}

	sliceIter := newRowSliceIter(rows, getStatusNodeAlignment())
	return printQueryOutput(os.Stdout, getStatusNodeHeaders(), sliceIter)
}

func runStatusNodeInner(showDecommissioned bool, args []string) ([]string, [][]string, error) {

	joinUsingID := func(queries []string) (query string) {
		for i, q := range queries {
			if i == 0 {
				query = q
				continue
			}
			query = "(" + query + ") LEFT JOIN (" + q + ") USING (id)"
		}
		return
	}

	maybeAddActiveNodesFilter := func(query string) string {
		if !showDecommissioned {
			query += " WHERE decommissioning = false OR split_part(expiration,',',1)::decimal > (now()-timestamp'1970-01-01')::decimal"
		}
		return query
	}

	baseQuery := maybeAddActiveNodesFilter(
		`SELECT node_id AS id,
            address,
            build_tag AS build,
            started_at,
            updated_at,
            CASE WHEN split_part(expiration,',',1)::decimal > (now()-timestamp'1970-01-01')::decimal
                 THEN true
                 ELSE false
                 END AS is_available,
            ifnull(is_live, false)
     FROM zbdb_internal.gossip_liveness LEFT JOIN zbdb_internal.gossip_nodes USING (node_id)`,
	)

	const rangesQuery = `
SELECT node_id AS id,
       sum((metrics->>'replicas.leaders')::DECIMAL)::INT AS replicas_leaders,
       sum((metrics->>'replicas.leaseholders')::DECIMAL)::INT AS replicas_leaseholders,
       sum((metrics->>'replicas')::DECIMAL)::INT AS ranges,
       sum((metrics->>'ranges.unavailable')::DECIMAL)::INT AS ranges_unavailable,
       sum((metrics->>'ranges.underreplicated')::DECIMAL)::INT AS ranges_underreplicated
FROM zbdb_internal.kv_store_status
GROUP BY node_id`

	const statsQuery = `
SELECT node_id AS id,
       sum((metrics->>'livebytes')::DECIMAL)::INT AS live_bytes,
       sum((metrics->>'keybytes')::DECIMAL)::INT AS key_bytes,
       sum((metrics->>'valbytes')::DECIMAL)::INT AS value_bytes,
       sum((metrics->>'intentbytes')::DECIMAL)::INT AS intent_bytes,
       sum((metrics->>'sysbytes')::DECIMAL)::INT AS system_bytes
FROM zbdb_internal.kv_store_status
GROUP BY node_id`

	const decommissionQuery = `
SELECT node_id AS id,
       ranges AS gossiped_replicas,
       decommissioning AS is_decommissioning,
       draining AS is_draining
FROM zbdb_internal.gossip_liveness LEFT JOIN zbdb_internal.gossip_nodes USING (node_id)`

	conn, err := getPasswordAndMakeSQLClient("znbase node status")
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	queriesToJoin := []string{baseQuery}

	if nodeCtx.statusShowAll || nodeCtx.statusShowRanges {
		queriesToJoin = append(queriesToJoin, rangesQuery)
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowStats {
		queriesToJoin = append(queriesToJoin, statsQuery)
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowDecommission {
		queriesToJoin = append(queriesToJoin, decommissionQuery)
	}

	if cliCtx.cmdTimeout != 0 {
		if err := conn.Exec(fmt.Sprintf("SET statement_timeout=%d", cliCtx.cmdTimeout), nil); err != nil {
			return nil, nil, err
		}
	}

	queryString := "SELECT * FROM (" + joinUsingID(queriesToJoin) + ")"

	switch len(args) {
	case 0:
		query := makeQuery(queryString + " ORDER BY id")
		return runQuery(conn, query, false)
	case 1:
		nodeID, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, nil, errors.Errorf("could not parse node_id %s", args[0])
		}
		query := makeQuery(queryString+" WHERE id = $1", nodeID)
		headers, rows, err := runQuery(conn, query, false)
		if err != nil {
			return nil, nil, err
		}
		if len(rows) == 0 {
			return nil, nil, fmt.Errorf("Error: node %d doesn't exist", nodeID)
		}
		return headers, rows, nil
	default:
		return nil, nil, errors.Errorf("expected no arguments or a single node ID")
	}
}

func getStatusNodeHeaders() []string {
	headers := baseNodeColumnHeaders

	if nodeCtx.statusShowAll || nodeCtx.statusShowRanges {
		headers = append(headers, statusNodesColumnHeadersForRanges...)
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowStats {
		headers = append(headers, statusNodesColumnHeadersForStats...)
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowDecommission {
		headers = append(headers, statusNodesColumnHeadersForDecommission...)
	}
	return headers
}

func getStatusNodeAlignment() string {
	align := "rllll"
	if nodeCtx.statusShowAll || nodeCtx.statusShowRanges {
		align += "rrrrrr"
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowStats {
		align += "rrrrrr"
	}
	if nodeCtx.statusShowAll || nodeCtx.statusShowDecommission {
		align += decommissionResponseAlignment()
	}
	return align
}

var decommissionNodesColumnHeaders = []string{
	"id",
	"is_live",
	"replicas",
	"is_decommissioning",
	"is_draining",
}

var decommissionNodeCmd = &cobra.Command{
	Use:   "decommission <node id 1> [<node id 2> ...]",
	Short: "decommissions the node(s)",
	Long: `
Marks the nodes with the supplied IDs as decommissioning.
This will cause leases and replicas to be removed from these nodes.`,
	Args: cobra.MinimumNArgs(1),
	RunE: MaybeDecorateGRPCError(runDecommissionNode),
}

func parseNodeIDs(strNodeIDs []string) ([]roachpb.NodeID, error) {
	nodeIDs := make([]roachpb.NodeID, 0, len(strNodeIDs))
	for _, str := range strNodeIDs {
		i, err := strconv.ParseInt(str, 10, 32)
		if err != nil {
			return nil, errors.Errorf("unable to parse %s: %s", str, err)
		}
		nodeIDs = append(nodeIDs, roachpb.NodeID(i))
	}
	return nodeIDs, nil
}

func runDecommissionNode(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, finish, err := getAdminClient(ctx)
	if err != nil {
		return err
	}
	defer finish()

	return runDecommissionNodeImpl(ctx, c, nodeCtx.nodeDecommissionWait, args)
}

func runDecommissionNodeImpl(
	ctx context.Context, c serverpb.AdminClient, wait nodeDecommissionWaitType, args []string,
) error {
	if wait == nodeDecommissionWaitLive {
		fmt.Fprintln(stderr, "\n--wait=live is deprecated and is treated as --wait=all")
	}
	nodeIDs, err := parseNodeIDs(args)
	if err != nil {
		return err
	}
	minReplicaCount := int64(math.MaxInt64)
	opts := retry.Options{
		InitialBackoff: 5 * time.Millisecond,
		Multiplier:     2,
		MaxBackoff:     20 * time.Second,
	}

	prevResponse := serverpb.DecommissionStatusResponse{}
	for r := retry.StartWithCtx(ctx, opts); r.Next(); {
		req := &serverpb.DecommissionRequest{
			NodeIDs:         nodeIDs,
			Decommissioning: true,
		}
		resp, err := c.Decommission(ctx, req)
		if err != nil {
			fmt.Fprintln(stderr)
			return errors.Wrap(err, "while trying to mark as decommissioning")
		}

		if !reflect.DeepEqual(&prevResponse, resp) {
			fmt.Fprintln(stderr)
			if err := printDecommissionStatus(*resp); err != nil {
				return err
			}
			prevResponse = *resp
		} else {
			fmt.Fprintf(stderr, ".")
		}
		var replicaCount int64
		allDecommissioning := true
		for _, status := range resp.Status {
			replicaCount += status.ReplicaCount
			allDecommissioning = allDecommissioning && status.Decommissioning
		}
		if replicaCount == 0 && allDecommissioning {
			fmt.Fprintln(os.Stdout, "\nNo more data reported on target nodes. "+
				"Please verify cluster health before removing the nodes.")
			return nil
		}
		if wait == nodeDecommissionWaitNone {
			return nil
		}
		if replicaCount < minReplicaCount {
			minReplicaCount = replicaCount
			r.Reset()
		}
	}
	return errors.New("maximum number of retries exceeded")
}

func decommissionResponseAlignment() string {
	return "rcrcc"
}

// decommissionResponseValueToRows converts DecommissionStatusResponse_Status to
// SQL-like result rows, so that we can pretty-print them.
func decommissionResponseValueToRows(
	statuses []serverpb.DecommissionStatusResponse_Status,
) [][]string {
	// Create results that are like the results for SQL results, so that we can pretty-print them.
	var rows [][]string
	for _, node := range statuses {
		rows = append(rows, []string{
			strconv.FormatInt(int64(node.NodeID), 10),
			strconv.FormatBool(node.IsLive),
			strconv.FormatInt(node.ReplicaCount, 10),
			strconv.FormatBool(node.Decommissioning),
			strconv.FormatBool(node.Draining),
		})
	}
	return rows
}

var recommissionNodeCmd = &cobra.Command{
	Use:   "recommission <node id 1> [<node id 2> ...]",
	Short: "recommissions the node(s)",
	Long: `
For the nodes with the supplied IDs, resets the decommissioning states,
signaling the affected nodes to participate in the cluster again.
	`,
	Args: cobra.MinimumNArgs(1),
	RunE: MaybeDecorateGRPCError(runRecommissionNode),
}

func printDecommissionStatus(resp serverpb.DecommissionStatusResponse) error {
	return printQueryOutput(os.Stdout, decommissionNodesColumnHeaders,
		newRowSliceIter(decommissionResponseValueToRows(resp.Status), decommissionResponseAlignment()))
}

func runRecommissionNode(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs, err := parseNodeIDs(args)
	if err != nil {
		return err
	}

	c, finish, err := getAdminClient(ctx)
	if err != nil {
		return err
	}
	defer finish()

	req := &serverpb.DecommissionRequest{
		NodeIDs:         nodeIDs,
		Decommissioning: false,
	}
	resp, err := c.Decommission(ctx, req)
	if err != nil {
		return err
	}
	return printDecommissionStatus(*resp)
}

// Sub-commands for node command.
var nodeCmds = []*cobra.Command{
	lsNodesCmd,
	statusNodeCmd,
	decommissionNodeCmd,
	recommissionNodeCmd,
}

var nodeCmd = &cobra.Command{
	Use:   "node [command]",
	Short: "list, inspect or remove nodes",
	Long:  "List, inspect or remove nodes.",
	RunE:  usageAndErr,
}
var storeCmd = &cobra.Command{
	Use:   "store [command]",
	Short: "Modify the state of the store",
	Long:  "Modify the state of the store",
	RunE:  usageAndErr,
}
var stateCmd = &cobra.Command{
	Use:   "state storeID=[<store id>] state=[<state id>]",
	Short: "update store state",
	Long: `
If a node ID is specified, this will show the status for the corresponding node. If no node ID
is specified, this will display the status for all nodes in the cluster.
	`,
	Args: cobra.MaximumNArgs(2),
	RunE: MaybeDecorateGRPCError(runUpdateStore),
}
var storeCmds = []*cobra.Command{
	stateCmd,
}

func init() {
	nodeCmd.AddCommand(nodeCmds...)
	storeCmd.AddCommand(storeCmds...)
}
