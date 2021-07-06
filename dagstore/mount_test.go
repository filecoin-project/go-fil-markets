package dagstore

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"

	mock_dagstore "github.com/filecoin-project/go-fil-markets/dagstore/mocks"
	"github.com/golang/mock/gomock"
	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/stretchr/testify/require"
)

func TestLotusMount(t *testing.T) {
	ctx := context.Background()
	bgen := blocksutil.NewBlockGenerator()
	cid := bgen.Next().Cid()

	mockCtrl := gomock.NewController(t)
	// when test is done, assert expectations on all mock objects.
	defer mockCtrl.Finish()

	// create a mock lotus api that returns the reader we want
	mockLotusMountAPI := mock_dagstore.NewMockLotusMountAPI(mockCtrl)
	mockLotusMountAPI.EXPECT().FetchUnsealedPiece(gomock.Any(), cid).Return(&readCloser{ioutil.NopCloser(strings.NewReader("testing"))}, nil).Times(1)
	mockLotusMountAPI.EXPECT().FetchUnsealedPiece(gomock.Any(), cid).Return(&readCloser{ioutil.NopCloser(strings.NewReader("testing"))}, nil).Times(1)
	mockLotusMountAPI.EXPECT().GetUnpaddedCARSize(cid).Return(uint64(100), nil).Times(1)

	mnt, err := NewLotusMount(cid, mockLotusMountAPI)
	require.NoError(t, err)
	info := mnt.Info()
	require.Equal(t, lotusScheme, info.URL.Scheme)
	require.Equal(t, cid.String(), info.URL.Host)

	// fetch and assert success
	rd, err := mnt.Fetch(context.Background())
	require.NoError(t, err)

	bz, err := ioutil.ReadAll(rd)
	require.NoError(t, err)
	require.NoError(t, rd.Close())
	require.Equal(t, []byte("testing"), bz)

	stat, err := mnt.Stat(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 100, stat.Size)

	// give url to factory -> should get back the same mount
	fct, err := NewLotusMountFactory(mockLotusMountAPI)
	require.NoError(t, err)

	// fetching on this mount should get us back the same data.
	mnt2, err := fct.Parse(mnt.URL)
	require.NoError(t, err)
	require.NotNil(t, mnt2)
	rd, err = mnt2.Fetch(context.Background())
	require.NoError(t, err)
	bz, err = ioutil.ReadAll(rd)
	require.NoError(t, err)
	require.NoError(t, rd.Close())
	require.Equal(t, []byte("testing"), bz)
}
