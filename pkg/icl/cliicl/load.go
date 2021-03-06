// Copyright 2017 The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package cliicl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/znbasedb/znbase/pkg/blobs"
	"github.com/znbasedb/znbase/pkg/cli"
	"github.com/znbasedb/znbase/pkg/icl/dump"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/storage/dumpsink"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/humanizeutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

func init() {
	loadShowCmd := &cobra.Command{
		Use:   "show <basepath>",
		Short: "show backups",
		Long:  "Shows information about a SQL backup.",
		RunE:  cli.MaybeDecorateGRPCError(runLoadShow),
	}

	loadCmds := &cobra.Command{
		Use:   "load [command]",
		Short: "loading commands",
		Long:  `Commands for bulk loading external files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Usage()
		},
	}
	cli.AddCmd(loadCmds)
	loadCmds.AddCommand(loadShowCmd)
}

func runLoadShow(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("basepath argument is required")
	}

	ctx := context.Background()
	basepath := args[0]
	if !strings.Contains(basepath, "://") {
		basepath = dumpsink.MakeLocalSinkURI(basepath)
	}
	dumpSinkFromURI := func(ctx context.Context, uri string) (dumpsink.DumpSink, error) {
		return dumpsink.FromURI(ctx, uri, cluster.NoSettings, blobs.TestEmptyBlobClientFactory)
	}
	desc, err := dump.ReadDumpDescriptorFromURI(ctx, basepath, dumpSinkFromURI, nil, "")
	if err != nil {
		return err
	}
	start := timeutil.Unix(0, desc.StartTime.WallTime).Format(time.RFC3339Nano)
	end := timeutil.Unix(0, desc.EndTime.WallTime).Format(time.RFC3339Nano)
	fmt.Printf("StartTime: %s (%s)\n", start, desc.StartTime)
	fmt.Printf("EndTime: %s (%s)\n", end, desc.EndTime)
	fmt.Printf("DataSize: %d (%s)\n", desc.EntryCounts.DataSize, humanizeutil.IBytes(desc.EntryCounts.DataSize))
	fmt.Printf("Rows: %d\n", desc.EntryCounts.Rows)
	fmt.Printf("IndexEntries: %d\n", desc.EntryCounts.IndexEntries)
	fmt.Printf("SystemRecords: %d\n", desc.EntryCounts.SystemRecords)
	fmt.Printf("FormatVersion: %d\n", desc.FormatVersion)
	fmt.Printf("ClusterID: %s\n", desc.ClusterID)
	fmt.Printf("NodeID: %s\n", desc.NodeID)
	fmt.Printf("BuildInfo: %s\n", desc.BuildInfo.Short())
	fmt.Printf("Spans:\n")
	for _, s := range desc.Spans {
		fmt.Printf("	%s\n", s)
	}
	fmt.Printf("Files:\n")
	for _, f := range desc.Files {
		fmt.Printf("	%s:\n", f.Path)
		fmt.Printf("		Span: %s\n", f.Span)
		fmt.Printf("		Sha512: %0128x\n", f.Sha512)
		fmt.Printf("		DataSize: %d (%s)\n", f.EntryCounts.DataSize, humanizeutil.IBytes(f.EntryCounts.DataSize))
		fmt.Printf("		Rows: %d\n", f.EntryCounts.Rows)
		fmt.Printf("		IndexEntries: %d\n", f.EntryCounts.IndexEntries)
		fmt.Printf("		SystemRecords: %d\n", f.EntryCounts.SystemRecords)
	}
	fmt.Printf("Descriptors:\n")
	for _, d := range desc.Descriptors {
		if desc := d.Table(hlc.Timestamp{}); desc != nil {
			fmt.Printf("	%d: %s (table)\n", d.GetID(), d.GetName())
		}
		if desc := d.GetDatabase(); desc != nil {
			fmt.Printf("	%d: %s (database)\n", d.GetID(), d.GetName())
		}
	}
	return nil
}
