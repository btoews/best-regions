package regions

import (
	"context"
	"net"
)

type resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

var dns resolver = net.DefaultResolver

type staticResolver struct {
	TXTs map[string]any
}

func (r *staticResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if r.TXTs == nil {
		return nil, &net.DNSError{Err: "nil MXs", IsNotFound: true}
	}
	ret, ok := r.TXTs[name]
	if !ok {
		return nil, &net.DNSError{Err: "no record", IsNotFound: true}
	}

	switch tret := ret.(type) {
	case error:
		return nil, tret
	case []string:
		return tret, nil
	default:
		panic("bad MX")
	}
}
