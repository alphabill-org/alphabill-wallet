package testutils

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/net"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	tokenbackend "github.com/alphabill-org/alphabill-wallet/wallet/tokens/backend"
	tokenclient "github.com/alphabill-org/alphabill-wallet/wallet/tokens/client"
)

func StartTokensBackend(t *testing.T, nodeAddr string) (srvUri string, restApi *tokenclient.TokenBackend) {
	port, err := net.GetFreePort()
	require.NoError(t, err)
	host := fmt.Sprintf("localhost:%v", port)
	srvUri = "http://" + host
	addr, err := url.Parse(srvUri)
	require.NoError(t, err)
	observe := observability.Default(t)
	restApi = tokenclient.New(*addr, observe)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		cfg := tokenbackend.NewConfig(tokens.DefaultSystemIdentifier, host, nodeAddr, filepath.Join(t.TempDir(), "backend.db"), observe)
		require.ErrorIs(t, tokenbackend.Run(ctx, cfg), context.Canceled)
	}()

	require.Eventually(t, func() bool {
		rnr, err := restApi.GetRoundNumber(ctx)
		return err == nil && rnr.RoundNumber > 0
	}, test.WaitDuration, test.WaitTick)

	return
}
