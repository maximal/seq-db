package search

import (
	"github.com/ozontech/seq-db/pkg/storeapi"
	"github.com/ozontech/seq-db/seq"
	"github.com/ozontech/seq-db/util"
)

type AggQuery struct {
	Field     string
	GroupBy   string
	Func      seq.AggFunc
	Quantiles []float64
}

//nolint:revive // TODO: ***REMOVED***
type SearchRequest struct {
	Explain     bool
	Q           []byte
	Offset      int
	Size        int
	Interval    seq.MID
	AggQ        []AggQuery
	From        seq.MID
	To          seq.MID
	WithTotal   bool
	ShouldFetch bool
	Order       seq.DocsOrder
}

func (sr *SearchRequest) GetAPISearchRequest() *storeapi.SearchRequest {
	return &storeapi.SearchRequest{
		Query:     util.ByteToStringUnsafe(sr.Q),
		From:      int64(sr.From),
		To:        int64(sr.To),
		Size:      int64(sr.Size),
		Offset:    int64(sr.Offset),
		Interval:  int64(sr.Interval),
		Aggs:      convertToAggsQuery(sr.AggQ),
		Explain:   sr.Explain,
		WithTotal: sr.WithTotal,
		Order:     storeapi.MustProtoOrder(sr.Order),
	}
}

func convertToAggsQuery(aggs []AggQuery) []*storeapi.AggQuery {
	buf := make([]storeapi.AggQuery, len(aggs))
	aggQ := make([]*storeapi.AggQuery, len(aggs))
	for i, query := range aggs {
		groupBy := query.GroupBy
		field := query.Field
		// Support legacy format in which field means groupBy.
		if query.Func == seq.AggFuncCount && query.Field != "" {
			groupBy = query.Field
			field = ""
		}
		buf[i].Field = field
		buf[i].GroupBy = groupBy
		buf[i].Func = storeapi.AggFunc(query.Func)
		buf[i].Quantiles = query.Quantiles
		aggQ[i] = &buf[i]
	}
	return aggQ
}
