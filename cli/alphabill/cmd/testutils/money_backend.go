package testutils

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/net"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	moneybackend "github.com/alphabill-org/alphabill-wallet/wallet/money/backend"
	moneyclient "github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
)

var (
	defaultMoneySDR = &genesis.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}
	defaultTokenSDR = &genesis.SystemDescriptionRecord{
		SystemIdentifier: tokens.DefaultSystemIdentifier,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.NewBillID(nil, []byte{3}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}
)

func StartMoneyBackend(t *testing.T, moneyPart *testpartition.NodePartition, genesisConfig *testutil.MoneyGenesisConfig) (string, *moneyclient.MoneyBackendClient) {
	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%v", port)
	observe := observability.Default(t)
	restClient, err := moneyclient.New(serverAddr, observe)
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	go func() {
		err := moneybackend.Run(ctx,
			&moneybackend.Config{
				ABMoneySystemIdentifier: money.DefaultSystemIdentifier,
				AlphabillUrl:            moneyPart.Nodes[0].AddrGRPC,
				ServerAddr:              serverAddr,
				DbFile:                  filepath.Join(t.TempDir(), moneybackend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: moneybackend.InitialBill{
					Id:        genesisConfig.InitialBillID,
					Value:     genesisConfig.InitialBillValue,
					Predicate: genesisConfig.InitialBillOwner,
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR, defaultTokenSDR},
				Logger:                   observe.Logger(),
				Observe:                  observe,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// wait for backend to start
	require.Eventually(t, func() bool {
		rnr, err := restClient.GetRoundNumber(ctx)
		return err == nil && rnr.RoundNumber > 0
	}, test.WaitDuration, test.WaitTick)

	return serverAddr, restClient
}
