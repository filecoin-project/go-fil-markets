package fsm_test

import (
	"context"
	"testing"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/fsm"
)

func init() {
	logging.SetLogLevel("*", "INFO") // nolint: errcheck
}

type testWorld struct {
	t       *testing.T
	proceed chan struct{}
	done    chan struct{}
}

type TestState struct {
	A uint64
	B uint64
	C []uint64
}

type TestEvent struct {
	A   string
	Val uint64
}

var events = []fsm.EventDesc{
	{
		Name: "start",
		Dst:  uint64(1),
		Src:  []fsm.StateKey{uint64(0)},
	},
	{
		Name: "restart",
		Dst:  uint64(1),
		Src:  []fsm.StateKey{uint64(1), uint64(2)},
	},
	{
		Name: "b",
		Dst:  uint64(2),
		Src:  []fsm.StateKey{uint64(1)},
		ApplyTransition: func(state *TestState, val uint64) error {
			state.B = val
			return nil
		},
	},
}

var stateHandlers = fsm.StateHandlers{
	uint64(1): func(ctx fsm.Context, tw *testWorld, ts TestState) error {
		err := ctx.Event("b", uint64(55))
		assert.NoError(tw.t, err)
		<-tw.proceed
		return nil
	},
	uint64(2): func(ctx fsm.Context, tw *testWorld, ts TestState) error {
		assert.Equal(tw.t, uint64(2), ts.A)
		close(tw.done)
		return nil
	},
}

func TestTypeCheckingOnSetup(t *testing.T) {
	ds := datastore.NewMapDatastore()
	tw := &testWorld{t: t, done: make(chan struct{}), proceed: make(chan struct{})}

	t.Run("Bad state field", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "Jesus", events, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "state type has no field `Jesus`")
	})
	t.Run("State field not comparable", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "C", events, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "state field `C` is not comparable")
	})
	t.Run("Event description has bad source type", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name: "start",
				Dst:  uint64(1),
				Src:  []fsm.StateKey{"happy"},
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `start` source type is not assignable to: uint64")
	})
	t.Run("Event description has bad destination type", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name: "start",
				Dst:  "happy",
				Src:  []fsm.StateKey{uint64(0)},
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `start` destination type is not assignable to: uint64")
	})
	t.Run("Event description has callback that is not a function", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name:            "b",
				Dst:             uint64(2),
				Src:             []fsm.StateKey{uint64(1)},
				ApplyTransition: "applesuace",
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `b` has a callback that is not a function")
	})
	t.Run("Event description has callback with no parameters", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name:            "b",
				Dst:             uint64(2),
				Src:             []fsm.StateKey{uint64(1)},
				ApplyTransition: func() {},
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `b` has a callback that does not take the state")
	})
	t.Run("Event description has callback with wrong first parameter", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name:            "b",
				Dst:             uint64(2),
				Src:             []fsm.StateKey{uint64(1)},
				ApplyTransition: func(uint64) error { return nil },
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `b` has a callback that does not take the state")
	})
	t.Run("Event description has callback that doesn't return an error", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", []fsm.EventDesc{
			{
				Name:            "b",
				Dst:             uint64(2),
				Src:             []fsm.StateKey{uint64(1)},
				ApplyTransition: func(*TestState) {},
			},
		}, stateHandlers)
		require.Nil(t, smm)
		require.EqualError(t, err, "event `b` callback should return exactly one param that is an error")
	})
	t.Run("State Handler with bad stateKey", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", events, fsm.StateHandlers{
			"apples": func(ctx fsm.Context, tw *testWorld, ts TestState) error {
				err := ctx.Event("b", uint64(55))
				assert.NoError(tw.t, err)
				<-tw.proceed
				return nil
			},
		})
		require.Nil(t, smm)
		require.EqualError(t, err, "state key is not assignable to: uint64")
	})
	t.Run("State Handler with bad statehandler", func(t *testing.T) {
		smm, err := fsm.New(ds, tw, TestState{}, "A", events, fsm.StateHandlers{
			uint64(1): func(ctx fsm.Context, tw *testWorld, u uint64) error {
				return nil
			},
		})
		require.Nil(t, smm)
		require.EqualError(t, err, "handler for state does not match expected type")
	})
}
func TestArgumentChecks(t *testing.T) {
	ds := datastore.NewMapDatastore()

	tw := &testWorld{t: t, done: make(chan struct{}), proceed: make(chan struct{})}
	smm, err := fsm.New(ds, tw, TestState{}, "A", events, stateHandlers)
	close(tw.proceed)
	require.NoError(t, err)

	// should take B with correct arguments
	err = smm.Send(uint64(2), "b", uint64(55))
	require.NoError(t, err)

	// should not take b with incorrect argument count
	err = smm.Send(uint64(2), "b", uint64(55), "applesuace")
	require.Regexp(t, "^Wrong number of arguments for event `b`", err.Error())

	// should not take b with incorrect argument type
	err = smm.Send(uint64(2), "b", "applesuace")
	require.Regexp(t, "^Incorrect argument type at index `0`", err.Error())

}

func TestBasic(t *testing.T) {
	for i := 0; i < 1000; i++ { // run a few times to expose any races
		ds := datastore.NewMapDatastore()

		tw := &testWorld{t: t, done: make(chan struct{}), proceed: make(chan struct{})}
		close(tw.proceed)
		smm, err := fsm.New(ds, tw, TestState{}, "A", events, stateHandlers)
		require.NoError(t, err)

		err = smm.Send(uint64(2), "start")
		require.NoError(t, err)
		<-tw.done
	}
}

func TestPersist(t *testing.T) {
	for i := 0; i < 1000; i++ { // run a few times to expose any races
		ds := datastore.NewMapDatastore()

		tw := &testWorld{t: t, done: make(chan struct{}), proceed: make(chan struct{})}
		smm, err := fsm.New(ds, tw, TestState{}, "A", events, stateHandlers)
		require.NoError(t, err)

		err = smm.Send(uint64(2), "start")
		require.NoError(t, err)

		if err := smm.Stop(context.Background()); err != nil {
			t.Fatal(err)
			return
		}

		smm, err = fsm.New(ds, tw, TestState{}, "A", events, stateHandlers)
		require.NoError(t, err)
		err = smm.Send(uint64(2), "restart")
		require.NoError(t, err)

		close(tw.proceed)

		<-tw.done
	}
}
