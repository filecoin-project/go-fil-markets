package storageimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
)

func TestGeneratePieceCommitment(t *testing.T) {
	carV1Path := filepath.Join("storagemarket", "fixtures", "test.car")
	_, carv2 := shared_testutil.GenCARV2(t, carV1Path)
	defer os.Remove(carv2)

	env := &providerDealEnvironment{}

	pieceCid, _, err := env.GeneratePieceCommitment(cid.Cid{}, carv2)
	require.NoError(t, err)
	require.NotEmpty(t, pieceCid.String())

	// generate CommP again = should get back the same CommP
	pieceCid2, _, err := env.GeneratePieceCommitment(cid.Cid{}, carv2)
	require.NoError(t, err)
	require.NotEmpty(t, pieceCid2.String())
	require.Equal(t, pieceCid, pieceCid2)

	// TODO Generate Commp for a different CAR file -> should get a different commP.

	// test failures
	pieceCid, _, err = env.GeneratePieceCommitment(cid.Cid{}, "carv2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")
	require.Equal(t, cid.Undef, pieceCid)

}

func TestClientAndProviderCommPMatches(t *testing.T) {
	ctx := context.Background()

	carV1Path := filepath.Join("storagemarket", "fixtures", "test.car")
	root, carv2 := shared_testutil.GenCARV2(t, carV1Path)
	defer os.Remove(carv2)

	// provider CommP
	env := &providerDealEnvironment{}
	providerCommP, _, err := env.GeneratePieceCommitment(cid.Cid{}, carv2)
	require.NoError(t, err)
	require.NotEqual(t, providerCommP, cid.Undef)

	// clientCommP
	ref := &storagemarket.DataRef{
		Root:         root,
		TransferType: storagemarket.TTGraphsync,
	}
	clientCommP, _, err := clientutils.CommP(ctx, carv2, ref)
	require.NotEqual(t, clientCommP, cid.Undef)
	require.NoError(t, err)
	require.Equal(t, clientCommP, providerCommP)

	// TODO should not match if client and provider use different files.
}
