package fsm

import (
	"context"
	"reflect"

	"github.com/filecoin-project/go-statemachine"
	"golang.org/x/xerrors"
)

type fsmHandler struct {
	stateType          reflect.Type
	stateField         StateKeyField
	transitionsByEvent map[EventName]eventDestination
	transitions        map[eKey]eventDestination
	stateHandlers      StateHandlers
	world              World
}

// NewFSMHandler defines an StateHandler for go-statemachine that implements
// a traditional Finite State Machine model -- transitions, start states,
// end states, and callbacks
func NewFSMHandler(world World, state StateType, stateField StateKeyField, events []EventDesc, stateHandlers StateHandlers) (statemachine.StateHandler, error) {
	worldType := reflect.TypeOf(world)
	stateType := reflect.TypeOf(state)
	stateFieldType, ok := stateType.FieldByName(string(stateField))
	if !ok {
		return nil, xerrors.Errorf("state type has no field `%s`", stateField)
	}
	if !stateFieldType.Type.Comparable() {
		return nil, xerrors.Errorf("state field `%s` is not comparable", stateField)
	}

	d := fsmHandler{
		world:              world,
		stateType:          stateType,
		stateField:         stateField,
		transitionsByEvent: make(map[EventName]eventDestination),
		transitions:        make(map[eKey]eventDestination),
		stateHandlers:      make(StateHandlers),
	}

	// Build transition map and store sets of all events and states.
	for _, e := range events {
		if !reflect.TypeOf(e.Dst).AssignableTo(stateFieldType.Type) {
			return nil, xerrors.Errorf("event `%s` destination type is not assignable to: %s", e.Name, stateFieldType.Type.Name())
		}
		argumentTypes, err := inspectApplyTransitionFunc(e, stateType)
		if err != nil {
			return nil, err
		}
		destination := eventDestination{
			dst:             e.Dst,
			argumentTypes:   argumentTypes,
			applyTransition: e.ApplyTransition,
		}
		d.stateHandlers[e.Dst] = nil
		d.transitionsByEvent[e.Name] = destination
		for _, src := range e.Src {
			if !reflect.TypeOf(src).AssignableTo(stateFieldType.Type) {
				return nil, xerrors.Errorf("event `%s` source type is not assignable to: %s", e.Name, stateFieldType.Type.Name())
			}
			d.transitions[eKey{e.Name, src}] = destination
			d.stateHandlers[src] = nil
		}
	}

	// type check state handlers
	for state, stateHandler := range stateHandlers {
		if !reflect.TypeOf(state).AssignableTo(stateFieldType.Type) {
			return nil, xerrors.Errorf("state key is not assignable to: %s", stateFieldType.Type.Name())
		}
		expectedHandlerType := reflect.FuncOf([]reflect.Type{reflect.TypeOf((*Context)(nil)).Elem(), worldType, d.stateType}, []reflect.Type{reflect.TypeOf(new(error)).Elem()}, false)
		validHandler := expectedHandlerType.AssignableTo(reflect.TypeOf(stateHandler))
		if !validHandler {
			return nil, xerrors.Errorf("handler for state does not match expected type")
		}
		d.stateHandlers[state] = stateHandler
	}

	return d, nil
}

// Plan executes events according to finite state machine logic
// It checks to see if the events can applied based on the current state,
// then applies the transition, updating the keyed state in the process
// At the end it executes the specified handler for the final state,
// if specified
func (d fsmHandler) Plan(events []statemachine.Event, user interface{}) (interface{}, error) {
	userValue := reflect.ValueOf(user)
	currentState := userValue.Elem().FieldByName(string(d.stateField)).Interface()
	for _, event := range events {
		e := event.User.(fsmEvent)
		destination, ok := d.transitions[eKey{e.name, currentState}]
		if !ok {
			return nil, xerrors.Errorf("Invalid event in queue, state `%s`, event `%s`", currentState, event)
		}
		err := d.applyTransition(userValue, e, destination)
		if err != nil {
			return nil, err
		}

		userValue.Elem().FieldByName(string(d.stateField)).Set(reflect.ValueOf(destination.dst))
		currentState = userValue.Elem().FieldByName(string(d.stateField)).Interface()
	}

	internalHandler := d.stateHandlers[currentState]

	return d.handler(internalHandler), nil
}

func (d fsmHandler) applyTransition(userValue reflect.Value, e fsmEvent, destination eventDestination) error {
	if destination.applyTransition == nil {
		return nil
	}
	values := make([]reflect.Value, 0, len(e.args)+1)
	values = append(values, userValue)
	for _, arg := range e.args {
		values = append(values, reflect.ValueOf(arg))
	}
	res := reflect.ValueOf(destination.applyTransition).Call(values)

	if res[0].Interface() != nil {
		return xerrors.Errorf("Error applying event transition `%s`: %w", e.name, res[0].Interface().(error))
	}
	return nil
}
func (d fsmHandler) handler(cb interface{}) interface{} {
	handlerType := reflect.FuncOf([]reflect.Type{reflect.TypeOf(statemachine.Context{}), d.stateType}, []reflect.Type{reflect.TypeOf(new(error)).Elem()}, false)

	if cb == nil {
		return reflect.MakeFunc(handlerType, func(args []reflect.Value) (results []reflect.Value) {
			return []reflect.Value{reflect.ValueOf(error(nil))}
		}).Interface()
	}
	return reflect.MakeFunc(handlerType, func(args []reflect.Value) (results []reflect.Value) {
		ctx := args[0].Interface().(statemachine.Context)
		state := args[1].Interface()
		dContext := fsmContext{state, ctx, d}
		return reflect.ValueOf(cb).Call([]reflect.Value{reflect.ValueOf(dContext), reflect.ValueOf(d.world), args[1]})
	}).Interface()
}

func (d fsmHandler) event(event EventName, args ...interface{}) (fsmEvent, error) {
	destination, ok := d.transitionsByEvent[event]
	if !ok {
		return fsmEvent{}, xerrors.Errorf("Unknown event `%s`", event)
	}
	if len(args) != len(destination.argumentTypes) {
		return fsmEvent{}, xerrors.Errorf("Wrong number of arguments for event `%s`", event)
	}
	for i, arg := range args {
		if !reflect.TypeOf(arg).AssignableTo(destination.argumentTypes[i]) {
			return fsmEvent{}, xerrors.Errorf("Incorrect argument type at index `%d` for event `%s`", i, event)
		}
	}
	return fsmEvent{event, args}, nil
}

// eKey is a struct key used for storing the transition map.
type eKey struct {
	// event is the name of the event that the keys refers to.
	event EventName

	// src is the source from where the event can transition.
	src interface{}
}

type eventDestination struct {
	dst             interface{}
	argumentTypes   []reflect.Type
	applyTransition interface{}
}

type fsmContext struct {
	state interface{}
	ctx   statemachine.Context
	d     fsmHandler
}

func (dc fsmContext) Context() context.Context {
	return dc.ctx.Context()
}

func (dc fsmContext) Event(event EventName, args ...interface{}) error {
	evt, err := dc.d.event(event, args...)
	if err != nil {
		return err
	}
	return dc.ctx.Send(evt)
}

var _ Context = fsmContext{}

type fsmEvent struct {
	name EventName
	args []interface{}
}

func inspectApplyTransitionFunc(e EventDesc, stateType reflect.Type) ([]reflect.Type, error) {
	if e.ApplyTransition == nil {
		return nil, nil
	}

	atType := reflect.TypeOf(e.ApplyTransition)
	if atType.Kind() != reflect.Func {
		return nil, xerrors.Errorf("event `%s` has a callback that is not a function", e.Name)
	}
	if atType.NumIn() < 1 {
		return nil, xerrors.Errorf("event `%s` has a callback that does not take the state", e.Name)
	}
	if !reflect.PtrTo(stateType).AssignableTo(atType.In(0)) {
		return nil, xerrors.Errorf("event `%s` has a callback that does not take the state", e.Name)
	}
	if atType.NumOut() != 1 || atType.Out(0).AssignableTo(reflect.TypeOf(new(error))) {
		return nil, xerrors.Errorf("event `%s` callback should return exactly one param that is an error", e.Name)
	}
	argumentTypes := make([]reflect.Type, atType.NumIn()-1)
	for i := range argumentTypes {
		argumentTypes[i] = atType.In(i + 1)
	}
	return argumentTypes, nil
}
