package maps

import (
	"testing"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/offline"
	m "pfeifer.dev/mapd/math"
)

// testTile packs ways (each a list of [lat, lon] nodes, with an id) the way tiles are stored.
func testTile(t *testing.T, ways map[int64][][2]float64, order []int64) Offline {
	t.Helper()
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatal(err)
	}
	root, err := offline.NewRootOffline(seg)
	if err != nil {
		t.Fatal(err)
	}
	list, err := root.NewWays(int32(len(order)))
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range order {
		way := list.At(i)
		way.SetId(id)
		nodes := ways[id]
		coords, err := way.NewNodes(int32(len(nodes)))
		if err != nil {
			t.Fatal(err)
		}
		minLat, minLon, maxLat, maxLon := 90.0, 180.0, -90.0, -180.0
		for j, node := range nodes {
			coords.At(j).SetLatitude(node[0])
			coords.At(j).SetLongitude(node[1])
			minLat, maxLat = min(minLat, node[0]), max(maxLat, node[0])
			minLon, maxLon = min(minLon, node[1]), max(maxLon, node[1])
		}
		way.SetMinLat(minLat)
		way.SetMinLon(minLon)
		way.SetMaxLat(maxLat)
		way.SetMaxLon(maxLon)
	}
	data, err := msg.MarshalPacked()
	if err != nil {
		t.Fatal(err)
	}
	o := ReadOffline(data)
	if !o.Loaded {
		t.Fatal("test tile did not load")
	}
	return o
}

func ids(ways []Way) []int64 {
	out := []int64{}
	for _, way := range ways {
		out = append(out, way.Id())
	}
	return out
}

// The endpoint index must return exactly what scanning every way returns, in the same order:
// NextWay breaks ties by order, so a different order would pick a different next way.
func TestMatchingWaysIndexMatchesFullScan(t *testing.T) {
	a, b, c, d, e := [2]float64{36.10, -115.10}, [2]float64{36.11, -115.10}, [2]float64{36.12, -115.10}, [2]float64{36.12, -115.11}, [2]float64{36.13, -115.11}
	ways := map[int64][][2]float64{
		1: {a, b},                   // chain a-b-c
		2: {b, c},                   //
		3: {c, b},                   // reversed duplicate geometry: same box as way 2
		4: {c, d, e},                // ends at e
		5: {e, {36.14, -115.12}, e}, // loop: first node == last node, indexed once
		6: {b, {36.11, -115.09}, c}, // starts at b, passes nothing, ends at c
		7: {b},                      // single node: never matches
		8: {},                       // no nodes: never matches
		9: {d, b},                   // d is a middle node of way 4, an endpoint here
	}
	order := []int64{4, 1, 9, 6, 2, 8, 3, 7, 5}
	indexed := testTile(t, ways, order)
	scanned := indexed
	scanned.endpoints = nil // forces the original full scan

	points := [][2]float64{a, b, c, d, e, {36.14, -115.12}, {36.0, -115.0}}
	for _, way := range order {
		for i := range indexed.Ways.Len() {
			from := indexed.Ways.At(i)
			if from.Id() != way {
				continue
			}
			for _, p := range points {
				pos := m.NewPosition(p[0], p[1])
				got, err := from.MatchingWays(&indexed, pos)
				if err != nil {
					t.Fatal(err)
				}
				want, err := from.MatchingWays(&scanned, pos)
				if err != nil {
					t.Fatal(err)
				}
				if g, w := ids(got), ids(want); !equal(g, w) {
					t.Errorf("way %d at %v: indexed %v, full scan %v", way, p, g, w)
				}
			}
		}
	}

	// hand-checked expectations, in tile order
	way1, way2 := indexed.Ways.At(1), indexed.Ways.At(4)
	if got, _ := way1.MatchingWays(&indexed, m.NewPosition(b[0], b[1])); !equal(ids(got), []int64{9, 6, 2, 3}) {
		t.Errorf("ways meeting way 1 at b = %v, expected [9 6 2 3]", ids(got))
	}
	// way 3 has the same box as way 2, so it is skipped like way 2 itself
	if got, _ := way2.MatchingWays(&indexed, m.NewPosition(c[0], c[1])); !equal(ids(got), []int64{4, 6}) {
		t.Errorf("ways meeting way 2 at c = %v, expected [4 6]", ids(got))
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
