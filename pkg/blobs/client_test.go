package blobs

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/rpc"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/util/hlc"
)

func writeTestFile(t *testing.T, file string, content []byte) {
	err := os.MkdirAll(filepath.Dir(file), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(file, content, 0600)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlobClientReadFile(t *testing.T) {
	localNodeID := roachpb.NodeID(1)
	remoteNodeID := roachpb.NodeID(2)
	localExternalDir, remoteExternalDir, stopper, cleanUpFn := createTestResources(t)
	defer cleanUpFn()

	clock := hlc.NewClock(hlc.UnixNano, time.Nanosecond)
	rpcContext := rpc.NewInsecureTestingContext(clock, stopper)
	rpcContext.TestingAllowNamedRPCToAnonymousServer = true

	blobClientFactory := setUpService(t, rpcContext, localNodeID, remoteNodeID, localExternalDir, remoteExternalDir)

	localFileContent := []byte("local_file")
	remoteFileContent := []byte("remote_file")
	writeTestFile(t, filepath.Join(localExternalDir, "test/local.csv"), localFileContent)
	writeTestFile(t, filepath.Join(remoteExternalDir, "test/remote.csv"), remoteFileContent)

	for _, tc := range []struct {
		name        string
		nodeID      roachpb.NodeID
		filename    string
		fileContent []byte
		err         string
	}{
		{
			"read-remote-file",
			remoteNodeID,
			"test/remote.csv",
			remoteFileContent,
			"",
		},
		{
			"read-local-file",
			localNodeID,
			"test/local.csv",
			localFileContent,
			"",
		},
		{
			"read-dir-exists",
			remoteNodeID,
			"test",
			nil,
			"is a directory",
		},
		{
			"read-check-calling-clean",
			remoteNodeID,
			"../test/remote.csv",
			nil,
			"outside of external-io-dir is not allowed",
		},
		{
			"read-outside-extern-dir",
			remoteNodeID,
			// this file exists, but is not within remote node's externalIODir
			filepath.Join("../..", localExternalDir, "test/local.csv"),
			nil,
			"outside of external-io-dir is not allowed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			blobClient, err := blobClientFactory(ctx, tc.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := blobClient.ReadFile(ctx, tc.filename)
			if err != nil {
				if tc.err != "" && testutils.IsError(err, tc.err) {
					// correct error was returned
					return
				}
				t.Fatal(err)
			}
			// Check that fetched file content is correct
			content, err := ioutil.ReadAll(reader)
			if err != nil {
				t.Fatal(err, "unable to read fetched file")
			}
			if !bytes.Equal(content, tc.fileContent) {
				t.Fatal(fmt.Sprintf(`fetched file content incorrect, expected %s, got %s`, tc.fileContent, content))
			}
		})
	}
}

func TestBlobClientWriteFile(t *testing.T) {
	localNodeID := roachpb.NodeID(1)
	remoteNodeID := roachpb.NodeID(2)
	localExternalDir, remoteExternalDir, stopper, cleanUpFn := createTestResources(t)
	defer cleanUpFn()

	clock := hlc.NewClock(hlc.UnixNano, time.Nanosecond)
	rpcContext := rpc.NewInsecureTestingContext(clock, stopper)
	rpcContext.TestingAllowNamedRPCToAnonymousServer = true

	blobClientFactory := setUpService(t, rpcContext, localNodeID, remoteNodeID, localExternalDir, remoteExternalDir)

	for _, tc := range []struct {
		name               string
		nodeID             roachpb.NodeID
		filename           string
		fileContent        string
		destinationNodeDir string
	}{
		{
			"write-remote-file",
			remoteNodeID,
			"test/remote.csv",
			"remotefile",
			remoteExternalDir,
		},
		{
			"write-local-file",
			localNodeID,
			"test/local.csv",
			"localfile",
			localExternalDir,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			blobClient, err := blobClientFactory(ctx, tc.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			byteContent := []byte(tc.fileContent)
			err = blobClient.WriteFile(ctx, tc.filename, bytes.NewReader(byteContent))
			if err != nil {
				t.Fatal(err)
			}
			// Check that file is now in correct node
			content, err := ioutil.ReadFile(filepath.Join(tc.destinationNodeDir, tc.filename))
			if err != nil {
				t.Fatal(err, "unable to read fetched file")
			}
			if !bytes.Equal(content, byteContent) {
				t.Fatal(fmt.Sprintf(`fetched file content incorrect, expected %s, got %s`, tc.fileContent, content))
			}
		})
	}
}

func TestBlobClientList(t *testing.T) {
	localNodeID := roachpb.NodeID(1)
	remoteNodeID := roachpb.NodeID(2)
	localExternalDir, remoteExternalDir, stopper, cleanUpFn := createTestResources(t)
	defer cleanUpFn()

	clock := hlc.NewClock(hlc.UnixNano, time.Nanosecond)
	rpcContext := rpc.NewInsecureTestingContext(clock, stopper)
	rpcContext.TestingAllowNamedRPCToAnonymousServer = true

	blobClientFactory := setUpService(t, rpcContext, localNodeID, remoteNodeID, localExternalDir, remoteExternalDir)

	localFileNames := []string{"file/local/dataA.csv", "file/local/dataB.csv", "file/local/dataC.csv"}
	remoteFileNames := []string{"file/remote/A.csv", "file/remote/B.csv", "file/remote/C.csv"}
	for _, fileName := range localFileNames {
		fullPath := filepath.Join(localExternalDir, fileName)
		writeTestFile(t, fullPath, []byte("testLocalFile"))
	}
	for _, fileName := range remoteFileNames {
		fullPath := filepath.Join(remoteExternalDir, fileName)
		writeTestFile(t, fullPath, []byte("testRemoteFile"))
	}

	for _, tc := range []struct {
		name         string
		nodeID       roachpb.NodeID
		dirName      string
		expectedList []string
		err          string
	}{
		{
			"list-local",
			localNodeID,
			"file/local/",
			localFileNames,
			"",
		},
		{
			"list-remote",
			remoteNodeID,
			"file/remote/",
			remoteFileNames,
			"",
		},
		{
			"list-local-no-match",
			localNodeID,
			"file/doesnotexist/",
			[]string{},
			"no such file or directory",
		},
		{
			"list-remote-no-match",
			remoteNodeID,
			"file/doesnotexist/",
			[]string{},
			"no such file or directory",
		},
		{
			"list-empty-pattern",
			remoteNodeID,
			"",
			[]string{},
			"pattern cannot be empty",
		},
		{
			"list-outside-external-dir",
			remoteNodeID,
			"../", // will error out
			[]string{},
			"outside of external-io-dir is not allowed",
		},
		{
			"list-backout-external-dir",
			remoteNodeID,
			"..",
			[]string{},
			"outside of external-io-dir is not allowed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			blobClient, err := blobClientFactory(ctx, tc.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			list, err := blobClient.List(ctx, tc.dirName)
			if err != nil {
				if tc.err != "" && testutils.IsError(err, tc.err) {
					// correct error returned
					return
				}
				t.Fatal(err)
			}
			// Check that returned list matches expected list
			if len(list) != len(tc.expectedList) {
				t.Fatal(`listed incorrect number of files`, list)
			}
			for i, f := range list {
				f = tc.dirName + f
				if f != tc.expectedList[i] {
					t.Fatal("incorrect list returned ", list)
				}
			}
		})
	}
}

func TestBlobClientDeleteFrom(t *testing.T) {
	localNodeID := roachpb.NodeID(1)
	remoteNodeID := roachpb.NodeID(2)
	localExternalDir, remoteExternalDir, stopper, cleanUpFn := createTestResources(t)
	defer cleanUpFn()

	clock := hlc.NewClock(hlc.UnixNano, time.Nanosecond)
	rpcContext := rpc.NewInsecureTestingContext(clock, stopper)
	rpcContext.TestingAllowNamedRPCToAnonymousServer = true

	blobClientFactory := setUpService(t, rpcContext, localNodeID, remoteNodeID, localExternalDir, remoteExternalDir)

	localFileContent := []byte("local_file")
	remoteFileContent := []byte("remote_file")
	writeTestFile(t, filepath.Join(localExternalDir, "test/local.csv"), localFileContent)
	writeTestFile(t, filepath.Join(remoteExternalDir, "test/remote.csv"), remoteFileContent)
	writeTestFile(t, filepath.Join(remoteExternalDir, "test/remote2.csv"), remoteFileContent)

	for _, tc := range []struct {
		name     string
		nodeID   roachpb.NodeID
		filename string
		err      string
	}{
		{
			"delete-remote-file",
			remoteNodeID,
			"test/remote.csv",
			"",
		},
		{
			"delete-local-file",
			localNodeID,
			"test/local.csv",
			"",
		},
		{
			"delete-remote-file-does-not-exist",
			remoteNodeID,
			"test/doesnotexist",
			"no such file",
		},
		{
			"delete-directory-not-empty",
			remoteNodeID,
			"test",
			"directory not empty",
		},
		{
			"delete-directory-empty", // this should work
			localNodeID,
			"test",
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			blobClient, err := blobClientFactory(ctx, tc.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			err = blobClient.Delete(ctx, tc.filename)
			if err != nil {
				if tc.err != "" && testutils.IsError(err, tc.err) {
					// the correct error was returned
					return
				}
				t.Fatal(err)
			}

			_, err = ioutil.ReadFile(filepath.Join(localExternalDir, tc.filename))
			if err == nil {
				t.Fatal(err, "file should have been deleted")
			}
		})
	}
}

func TestBlobClientStat(t *testing.T) {
	localNodeID := roachpb.NodeID(1)
	remoteNodeID := roachpb.NodeID(2)
	localExternalDir, remoteExternalDir, stopper, cleanUpFn := createTestResources(t)
	defer cleanUpFn()

	clock := hlc.NewClock(hlc.UnixNano, time.Nanosecond)
	rpcContext := rpc.NewInsecureTestingContext(clock, stopper)
	rpcContext.TestingAllowNamedRPCToAnonymousServer = true

	blobClientFactory := setUpService(t, rpcContext, localNodeID, remoteNodeID, localExternalDir, remoteExternalDir)

	localFileContent := []byte("local_file")
	remoteFileContent := []byte("remote_file")
	writeTestFile(t, filepath.Join(localExternalDir, "test/local.csv"), localFileContent)
	writeTestFile(t, filepath.Join(remoteExternalDir, "test/remote.csv"), remoteFileContent)

	for _, tc := range []struct {
		name         string
		nodeID       roachpb.NodeID
		filename     string
		expectedSize int64
		err          string
	}{
		{
			"stat-remote-file",
			remoteNodeID,
			"test/remote.csv",
			int64(len(remoteFileContent)),
			"",
		},
		{
			"stat-local-file",
			localNodeID,
			"test/local.csv",
			int64(len(localFileContent)),
			"",
		},
		{
			"stat-remote-file-does-not-exist",
			remoteNodeID,
			"test/doesnotexist",
			0,
			"no such file",
		},
		{
			"stat-directory",
			remoteNodeID,
			"test",
			0,
			"is a directory",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			blobClient, err := blobClientFactory(ctx, tc.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := blobClient.Stat(ctx, tc.filename)
			if err != nil {
				if testutils.IsError(err, tc.err) {
					// the correct error was returned
					return
				}
				t.Fatal(err)
			}
			if resp.Filesize != tc.expectedSize {
				t.Fatalf("expected size: %d got: %d", tc.expectedSize, resp)
			}
		})
	}
}
