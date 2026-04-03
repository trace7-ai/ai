package sdk

import (
	"fmt"

	"mira/pkg/contract"
	"mira/pkg/service"
	"mira/pkg/session"
	"mira/pkg/transport"
)

type Options struct {
	Transport        transport.Transport
	TransportFactory func() (transport.Transport, error)
	Store            *session.Store
	OnChunk          func([]byte)
}

func Ask(request contract.Request, options *Options) (contract.Response, int, error) {
	client, err := resolveTransport(options)
	if err != nil {
		return contract.Response{}, 0, err
	}
	store, err := resolveStore(options)
	if err != nil {
		return contract.Response{}, 0, err
	}
	return service.Service{Client: client, Store: store, OnChunk: resolveOnChunk(options)}.Run(request)
}

func AskWithBuilder(builder *RequestBuilder, options *Options) (contract.Response, int, error) {
	if builder == nil {
		return contract.Response{}, 0, fmt.Errorf("request builder must not be nil")
	}
	request, err := builder.Build()
	if err != nil {
		return contract.Response{}, 0, err
	}
	return Ask(request, options)
}

func DefaultTransport() (transport.Transport, error) {
	config, err := transport.LoadConfig()
	if err != nil {
		return nil, err
	}
	return transport.NewMiraClient(config), nil
}

func resolveTransport(options *Options) (transport.Transport, error) {
	if options != nil {
		if options.Transport != nil {
			return options.Transport, nil
		}
		if options.TransportFactory != nil {
			return options.TransportFactory()
		}
	}
	return DefaultTransport()
}

func resolveStore(options *Options) (*session.Store, error) {
	if options != nil && options.Store != nil {
		return options.Store, nil
	}
	return session.New("")
}

func resolveOnChunk(options *Options) func([]byte) {
	if options == nil {
		return nil
	}
	return options.OnChunk
}
