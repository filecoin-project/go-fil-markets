package storageimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestGeneratePieceCommitment(t *testing.T) {
	_, carV2File1 := shared_testutil.GenFullCARv2FromNormalFile(t, filepath.Join("storagemarket", "fixtures", "payload.txt"))
	defer os.Remove(carV2File1)
	_, carV2File2 := shared_testutil.GenFullCARv2FromNormalFile(t, filepath.Join("storagemarket", "fixtures", "payload2.txt"))
	defer os.Remove(carV2File2)

	commP1 := genProviderCommP(t, carV2File1)
	commP2 := genProviderCommP(t, carV2File2)

	commP3 := genProviderCommP(t, carV2File1)
	commP4 := genProviderCommP(t, carV2File2)

	require.Equal(t, commP1, commP3)
	require.Equal(t, commP2, commP4)

	require.NotEqual(t, commP1, commP4)
	require.NotEqual(t, commP2, commP3)

	// fails when CARv2 file path isn't a valid one.
	env := &providerDealEnvironment{}
	pieceCid, _, err := env.GeneratePieceCommitment(cid.Cid{}, "randpath")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")
	require.Equal(t, cid.Undef, pieceCid)
}

func genProviderCommP(t *testing.T, carv2 string) cid.Cid {
	env := &providerDealEnvironment{}
	pieceCid, _, err := env.GeneratePieceCommitment(cid.Cid{}, carv2)
	require.NoError(t, err)
	require.NotEqual(t, pieceCid, cid.Undef)
	return pieceCid
}
