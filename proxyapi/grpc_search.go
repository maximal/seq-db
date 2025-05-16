package proxyapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ozontech/seq-db/pkg/seqproxyapi/v1"
)

func (g *grpcV1) Search(
	ctx context.Context, req *seqproxyapi.SearchRequest,
) (*seqproxyapi.SearchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, g.config.SearchTimeout)
	defer cancel()

	if req.Size <= 0 {
		return nil, status.Error(codes.InvalidArgument, `"size" must be greater than 0`)
	}

	proxyReq := &seqproxyapi.ComplexSearchRequest{
		Query:     req.Query,
		Size:      req.Size,
		Offset:    req.Offset,
		WithTotal: req.WithTotal,
		Order:     req.Order,
	}
	sResp, err := g.doSearch(ctx, proxyReq, true, nil)
	if err != nil {
		return nil, err
	}
	if sResp.err != nil && !shouldHaveResponse(sResp.err.Code) {
		return &seqproxyapi.SearchResponse{Error: sResp.err}, nil
	}

	resp := &seqproxyapi.SearchResponse{
		Docs:  makeProtoDocs(sResp.qpr, sResp.docsStream),
		Total: int64(sResp.qpr.Total),
		Error: &seqproxyapi.Error{
			Code: seqproxyapi.ErrorCode_ERROR_CODE_NO,
		},
	}
	if sResp.err != nil {
		resp.Error = sResp.err
		resp.PartialResponse = sResp.err.Code == seqproxyapi.ErrorCode_ERROR_CODE_PARTIAL_RESPONSE
	}

	return resp, nil
}
