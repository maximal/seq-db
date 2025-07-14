package processor

import (
	"math"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ozontech/seq-db/node"
	"github.com/ozontech/seq-db/seq"
)

func TestSingleSourceCountAggregator(t *testing.T) {
	searchDocs := []uint32{2, 3, 5, 8, 10, 12, 15}
	sources := [][]uint32{
		{2, 3, 5, 8, 10, 12},
		{1, 4, 6, 9, 11, 13},
		{1, 2, 4, 5, 8, 11, 12},
	}

	source := node.BuildORTreeAgg(node.MakeStaticNodes(sources))
	iter := NewSourcedNodeIterator(source, nil, nil, 0, false)
	agg := NewSingleSourceCountAggregator(iter, provideExtractTimeFunc(nil, nil, 0))
	for _, id := range searchDocs {
		if err := agg.Next(id); err != nil {
			t.Fatal(err)
		}
	}
	assert.Equal(t, map[AggBin[uint32]]int64{{Source: 0}: 6}, agg.countBySource)
	assert.Equal(t, int64(1), agg.notExists)
}

func TestSingleSourceCountAggregatorWithInterval(t *testing.T) {
	searchDocs := []uint32{2, 3, 5, 8, 10, 12, 15}
	sources := [][]uint32{
		{2, 3, 5, 8, 10, 12},
		{1, 4, 6, 9, 11, 13},
		{1, 2, 4, 5, 8, 11, 12},
	}

	source := node.BuildORTreeAgg(node.MakeStaticNodes(sources))
	iter := NewSourcedNodeIterator(source, nil, nil, 0, false)

	agg := NewSingleSourceCountAggregator(iter, func(l seq.LID) seq.MID {
		return seq.MID(l) % 3
	})

	for _, id := range searchDocs {
		if err := agg.Next(id); err != nil {
			t.Fatal(err)
		}
	}

	assert.Equal(t, map[AggBin[uint32]]int64{
		{Source: 0, MID: 0}: 2,
		{Source: 0, MID: 1}: 1,
		{Source: 0, MID: 2}: 3,
	}, agg.countBySource)

	assert.Equal(t, int64(1), agg.notExists)
}

func Generate(n int) ([]uint32, uint32) {
	v := make([]uint32, n)
	last := uint32(1)
	for i := range v {
		v[i] = last
		last += uint32(1 + rand.Intn(5))
	}
	return v, last
}

func BenchmarkAggDeep(b *testing.B) {
	v, _ := Generate(b.N)
	src := node.NewSourcedNodeWrapper(node.NewStatic(v, false), 0)
	iter := NewSourcedNodeIterator(src, nil, make([]uint32, 1), 0, false)
	n := NewSingleSourceCountAggregator(iter, provideExtractTimeFunc(nil, nil, 0))
	vals, _ := Generate(b.N)
	b.ResetTimer()
	for _, v := range vals {
		if err := n.Next(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAggWide(b *testing.B) {
	v, _ := Generate(b.N)

	factor := int(math.Sqrt(float64(b.N)))
	wide := make([][]uint32, b.N/factor)
	for i := range wide {
		for range factor {
			wide[i] = append(wide[i], v[rand.Intn(b.N)])
		}
		slices.Sort(wide[i])
	}

	source := node.BuildORTreeAgg(node.MakeStaticNodes(wide))

	iter := NewSourcedNodeIterator(source, nil, make([]uint32, len(wide)), 0, false)
	n := NewSingleSourceCountAggregator(iter, provideExtractTimeFunc(nil, nil, 0))
	vals, _ := Generate(b.N)
	b.ResetTimer()
	for _, v := range vals {
		if err := n.Next(v); err != nil {
			b.Fatal(err)
		}
	}
}

type MockTokenIndex struct {
	tokenIndex // embed to implement TokenIndex interface and override only needed methods
}

func (m *MockTokenIndex) GetValByTID(tid uint32) []byte {
	return []byte(strconv.Itoa(int(tid)))
}

type IDSourcePair struct {
	LID    uint32
	Source uint32
}

type MockNode struct {
	Pairs []IDSourcePair
}

// String implements node.Sourced
func (m *MockNode) String() string {
	return reflect.TypeOf(m).String()
}

func (m *MockNode) NextSourced() (uint32, uint32, bool) {
	if len(m.Pairs) == 0 {
		return 0, 0, false
	}
	first := m.Pairs[0]
	m.Pairs = m.Pairs[1:]
	return first.LID, first.Source, true
}

func TestTwoSourceAggregator(t *testing.T) {
	r := require.New(t)

	// Mock data provider and sources.
	dp := &MockTokenIndex{}
	field := &MockNode{
		Pairs: []IDSourcePair{
			{LID: 1, Source: 0},
			{LID: 2, Source: 1},
		},
	}
	groupBy := &MockNode{
		Pairs: []IDSourcePair{
			{LID: 1, Source: 0},
			{LID: 2, Source: 1},
		},
	}

	fieldTIDs := []uint32{42, 73}
	groupByTIDs := []uint32{1, 2}
	groupIterator := NewSourcedNodeIterator(groupBy, dp, groupByTIDs, 0, false)
	fieldIterator := NewSourcedNodeIterator(field, dp, fieldTIDs, 0, false)
	aggregator := NewGroupAndFieldAggregator(
		fieldIterator, groupIterator, provideExtractTimeFunc(nil, nil, 0), true,
	)

	// Call Next for two data points.
	r.NoError(aggregator.Next(1))
	r.NoError(aggregator.Next(2))

	// Verify countBySource map.
	expectedCountBySource := map[twoSources]int64{
		{GroupBySource: 0, FieldSource: 0}: 1,
		{GroupBySource: 1, FieldSource: 1}: 1,
	}

	for source, count := range expectedCountBySource {
		r.Equal(count, aggregator.countBySource[AggBin[twoSources]{
			Source: source,
		}])
	}

	agg, err := aggregator.Aggregate()
	r.NoError(err)

	wantBuckets := []seq.AggregationBucket{
		{
			Name:  "2",
			Value: 73,
		},
		{
			Name:  "1",
			Value: 42,
		},
	}

	got := agg.Aggregate(seq.AggregateArgs{
		Func: seq.AggFuncMax,
	})

	r.Equal(wantBuckets, got.Buckets)
}

func TestSingleTreeCountAggregator(t *testing.T) {
	r := require.New(t)
	dp := &MockTokenIndex{}
	field := &MockNode{
		Pairs: []IDSourcePair{
			{LID: 1, Source: 0},
		},
	}

	iter := NewSourcedNodeIterator(field, dp, []uint32{0}, 0, false)
	aggregator := NewSingleSourceCountAggregator(iter, provideExtractTimeFunc(nil, nil, 0))

	r.NoError(aggregator.Next(1))

	result, err := aggregator.Aggregate()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expectedResult := seq.AggregatableSamples{
		SamplesByBin: map[seq.AggBin]*seq.SamplesContainer{
			// "0" because DataProvider converts TID source (tid index) to string
			{Token: "0"}: {Total: 1},
		},
	}

	r.Equal(len(expectedResult.SamplesByBin), len(result.SamplesByBin))
	for token, hist := range expectedResult.SamplesByBin {
		r.Equal(hist.Total, result.SamplesByBin[token].Total)
	}
}
