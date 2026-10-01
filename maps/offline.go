package maps

import (
	"log/slog"
	"math"

	"pfeifer.dev/mapd/cereal/offline"
	m "pfeifer.dev/mapd/math"
	u "pfeifer.dev/mapd/utils"

	"capnproto.org/go/capnp/v3"
)

func ReadOffline(data []uint8) Offline {
	msg, err := capnp.UnmarshalPacked(data)
	if err != nil {
		slog.Warn("could not unmarshal offline data", "error", err)
	}
	if err == nil {
		offlineMaps, err := offline.ReadRootOffline(msg)
		if err != nil {
			slog.Warn("could not read offline message", "error", err)
			return Offline{Loaded: false}
		}
		if !offlineMaps.IsValid() {
			slog.Warn("could not read offline message", "reason", "root is not a struct")
			return Offline{Loaded: false}
		}
		// allow us to read as much as we want
		msg.ResetReadLimit(math.MaxUint64)
		ways, err := offlineMaps.Ways()
		if err != nil {
			slog.Warn("Could not read ways from offline maps", "error", err)
		}
		o := Offline{offline: offlineMaps, waysRaw: ways, Loaded: true}
		o.Ways.Init(o._wayAt, ways.Len())
		o.buildEndpointIndex()
		return o
	}
	return Offline{Loaded: false}
}

type Offline struct {
	Loaded     bool
	offline    offline.Offline
	box        u.Curry[m.Box]
	overlapBox u.Curry[m.Box]
	Ways       u.CurryList[Way]
	waysRaw    offline.Way_List
	overlap    u.Curry[float64]
	endpoints  map[m.Position][]int // way indices by first/last node, in tile order
}

func (o *Offline) _box() m.Box {
	return m.Box{
		MinPos: m.NewPosition(float64(o.offline.MinLat()), float64(o.offline.MinLon())),
		MaxPos: m.NewPosition(float64(o.offline.MaxLat()), float64(o.offline.MaxLon())),
	}
}

func (o *Offline) Box() m.Box {
	return o.box.Value(o._box)
}

func (o *Offline) _overlapBox() m.Box {
	box := o.Box()
	return box.Overlap(o.Overlap())
}

func (o *Offline) OverlapBox() m.Box {
	return o.overlapBox.Value(o._overlapBox)
}

func (o *Offline) _overlap() float64 {
	return o.offline.Overlap()
}

func (o *Offline) Overlap() float64 {
	return o.overlap.Value(o._overlap)
}

func (o *Offline) _wayAt(index int) Way {
	return NewWay(o.waysRaw.At(index))
}

// buildEndpointIndex maps each way's first and last node to the ways that start or end there,
// in tile order, so MatchingWays does not scan every way in the tile for each next-way lookup.
// Positions compare exactly, as Position.Equals does.
func (o *Offline) buildEndpointIndex() {
	o.endpoints = make(map[m.Position][]int)
	for i := range o.Ways.Len() {
		way := o.Ways.At(i)
		if way.Nodes.Len() < 2 {
			continue
		}
		first := way.Nodes.At(0)
		last := way.Nodes.At(way.Nodes.Len() - 1)
		o.endpoints[first] = append(o.endpoints[first], i)
		if !last.Equals(first) {
			o.endpoints[last] = append(o.endpoints[last], i)
		}
	}
}

// WaysWithEndpoint returns the indices of ways that start or end at pos, in tile order.
// ok is false when the tile has no index.
func (o *Offline) WaysWithEndpoint(pos m.Position) (indices []int, ok bool) {
	if o.endpoints == nil {
		return nil, false
	}
	return o.endpoints[pos], true
}
