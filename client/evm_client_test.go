package client

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestGetFeeCreditRecordByOwnerID(t *testing.T) {
	tests := []struct {
		name         string
		ownerID      []byte
		setupMock    func() *mocksrv.StateServiceMock
		expectError  bool
		assertResult func(result *sdktypes.FeeCreditRecord)
	}{
		{
			name:    "WithExistingOwnerID",
			ownerID: []byte{1},
			setupMock: func() *mocksrv.StateServiceMock {
				service := mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit([]byte{1}, &sdktypes.Unit[any]{
					UnitID: []byte{11},
					Data: &stateObject{
						Account: &account{
							Balance: uint256.NewInt(100 * 1e8),
						},
						AlphaBill: &alphaBillLink{
							Counter:        5,
							MinLifetime:    42,
							OwnerPredicate: []byte{1},
						},
					},
					StateProof: nil,
				}))
				return service
			},
			expectError: false,
			assertResult: func(fcr *sdktypes.FeeCreditRecord) {
				require.NotNil(t, fcr)
				require.EqualValues(t, []byte{11}, fcr.ID)
				require.NotNil(t, fcr.Counter)
				require.EqualValues(t, 1, fcr.Balance)
				require.EqualValues(t, 5, *fcr.Counter)
				require.EqualValues(t, 42, fcr.MinLifetime)
			},
		},
		{
			name:        "WithNonExistingOwnerID",
			ownerID:     []byte{2},
			setupMock:   func() *mocksrv.StateServiceMock { return mocksrv.NewStateServiceMock() },
			expectError: false,
			assertResult: func(fcr *sdktypes.FeeCreditRecord) {
				require.Nil(t, fcr)
			},
		},
		{
			name:    "WithError",
			ownerID: []byte{1},
			setupMock: func() *mocksrv.StateServiceMock {
				service := mocksrv.NewStateServiceMock()
				service.Err = errors.New("some error")
				return service
			},
			expectError: true,
		},
	}

	pdr := types.PartitionDescriptionRecord{
		NetworkID:       3,
		PartitionID:     3,
		PartitionTypeID: 3,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.setupMock()
			client := startServerAndEvmClient(t, &pdr, service)

			fcr, err := client.GetFeeCreditRecordByOwnerID(context.Background(), tt.ownerID)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.assertResult(fcr)
			}
		})
	}
}

func startServerAndEvmClient(t *testing.T, pdr *types.PartitionDescriptionRecord, service *mocksrv.StateServiceMock) sdktypes.PartitionClient {
	srv := mocksrv.StartStateApiServer(t, pdr, service)

	evmClient, err := NewEvmPartitionClient(context.Background(), "http://"+srv)
	require.NoError(t, err)
	t.Cleanup(evmClient.Close)

	return evmClient
}
