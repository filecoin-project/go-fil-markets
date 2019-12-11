package cario

import (
    "context"
    "fmt"
    "github.com/filecoin-project/go-fil-components/pieceio"
    "github.com/ipfs/go-car"
    "github.com/ipfs/go-cid"
    "github.com/ipld/go-ipld-prime"
    "github.com/ipld/go-ipld-prime/traversal/selector"
    "io"
)

type carIO struct {
}

func NewCarIO() pieceio.CarIO {
    return &carIO{}
}

func (c carIO) WriteCar(ctx context.Context, bs pieceio.ReadStore, payloadCid cid.Cid, node ipld.Node, w io.Writer) error {
    selector, err := selector.ParseSelector(node)
    if err != nil {
        return err
    }
    return car.WriteSelectiveCar(ctx, bs, []car.CarDag{{Root: payloadCid, Selector: selector}}, w)
}

func (c carIO) LoadCar(bs pieceio.WriteStore, r io.Reader) (cid.Cid, error) {
    header, err := car.LoadCar(bs, r)
    if err != nil {
        return cid.Undef, err
    }
    l := len(header.Roots)
    if l == 0 {
        return cid.Undef, fmt.Errorf("invalid header: missing root")
    }
    if l > 1 {
        return cid.Undef, fmt.Errorf("invalid header: contains %d roots (expecting 1)", l)
    }
    return header.Roots[0], nil
}
