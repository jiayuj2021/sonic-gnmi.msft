package gnmi

// Tests SHOW interface neighbor expected (JSON output)

import (
	"fmt"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	pb "github.com/openconfig/gnmi/proto/gnmi"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	show_client "github.com/sonic-net/sonic-gnmi/show_client"
)

// getInterfaceNeighborExpected returns JSON like:
// {
//   "Ethernet2": {
//     "neighbor":"DEVICE01T1",
//     "neighbor_port":"Ethernet1",
//     "neighbor_loopback":"10.1.1.1",
//     "neighbor_mgmt":"192.0.2.10",
//     "neighbor_type":"BackEndLeafRouter"
//   }
// }

func TestShowInterfaceNeighborExpected(t *testing.T) {
	s := createServer(t, ServerPort)
	go runServer(t, s)
	defer s.ForceStop()
	defer ResetDataSetsAndMappings(t)

	conn, err := grpc.Dial(TargetAddr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	gClient := pb.NewGNMIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout*time.Second)
	defer cancel()

	neighborFile := "../testdata/DEVICE_NEIGHBOR_EXPECTED.txt"
	neighborMetaFile := "../testdata/DEVICE_NEIGHBOR_METADATA_EXPECTED.txt"
	neighborOnlyFile := "../testdata/DEVICE_NEIGHBOR_EXPECTED_NO_META.txt"

	const (
		expectedEmpty = `{}`

		expectedSingle = `{"Ethernet2":{"neighbor":"DEVICE01T1","neighbor_port":"Ethernet1","neighbor_loopback":"10.1.1.1","neighbor_mgmt":"192.0.2.10","neighbor_type":"BackEndLeafRouter"}}`

		expectedMissingMeta = `{"Ethernet4":{"neighbor":"DEVICE02T1","neighbor_port":"Ethernet9","neighbor_loopback":"None","neighbor_mgmt":"None","neighbor_type":"None"}}`
	)

	tests := []struct {
		desc       string
		textPbPath string
		wantCode   codes.Code
		wantResp   string
		valTest    bool
		testInit   func()
		mockPatch  func() *gomonkey.Patches
	}{
		{
			desc: "no data",
			textPbPath: `
              elem: <name: "interface">
              elem: <name: "neighbor">
              elem: <name: "expected">
            `,
			wantCode: codes.OK,
			wantResp: expectedEmpty,
			valTest:  true,
			testInit: func() { FlushDataSet(t, ConfigDbNum) },
		},
		{
			desc: "single neighbor (datasets)",
			textPbPath: `
              elem: <name: "interface">
              elem: <name: "neighbor">
              elem: <name: "expected">
            `,
			wantCode: codes.OK,
			wantResp: expectedSingle,
			valTest:  true,
			testInit: func() {
				FlushDataSet(t, ConfigDbNum)
				AddDataSet(t, ConfigDbNum, neighborFile)
				AddDataSet(t, ConfigDbNum, neighborMetaFile)
			},
		},
		{
			desc: "missing metadata defaults (datasets)",
			textPbPath: `
              elem: <name: "interface">
              elem: <name: "neighbor">
              elem: <name: "expected">
            `,
			wantCode: codes.OK,
			wantResp: expectedMissingMeta,
			valTest:  true,
			testInit: func() {
				FlushDataSet(t, ConfigDbNum)
				AddDataSet(t, ConfigDbNum, neighborOnlyFile)
			},
		},
		{
			desc: "GetMapFromQueries error (neighbor)",
			textPbPath: `
              elem: <name: "interface">
              elem: <name: "neighbor">
              elem: <name: "expected">
            `,
			wantCode: codes.NotFound,
			valTest:  false,
			testInit: func() { FlushDataSet(t, ConfigDbNum) },
			mockPatch: func() *gomonkey.Patches {
				return gomonkey.ApplyFunc(show_client.GetMapFromQueries,
					func(q [][]string) (map[string]interface{}, error) {
						return nil, fmt.Errorf("injected neighbor error")
					})
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			if tc.testInit != nil {
				tc.testInit()
			}
			var patches *gomonkey.Patches
			if tc.mockPatch != nil {
				patches = tc.mockPatch()
			}
			if patches != nil {
				defer patches.Reset()
			}
			wantVal := []byte(nil)
			if tc.valTest {
				wantVal = []byte(tc.wantResp)
			}
			runTestGet(t, ctx, gClient, "SHOW", tc.textPbPath, tc.wantCode, wantVal, tc.valTest)
		})
	}
}
