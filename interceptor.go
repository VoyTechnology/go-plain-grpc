package plaingrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// ClientInterceptor hijacks the gRPC
type Hijacker struct {
	ep         string
	httpClient *http.Client
	wsClient   *websocket.Dialer
}

func NewHijacker(ep string) *Hijacker {
	return &Hijacker{
		ep:         ep,
		httpClient: http.DefaultClient, // TODO: Add as option
		wsClient:   websocket.DefaultDialer,
	}
}

func (h *Hijacker) UnaryInterceptor(ctx context.Context, method string, req, res interface{}, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
	in := req.(proto.Message)
	b, err := proto.Marshal(in)
	if err != nil {
		return err
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, h.ep+method, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	resp, err := h.httpClient.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	out := res.(proto.Message)
	return proto.Unmarshal(body, out)
}

func (h *Hijacker) StreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, _ *grpc.ClientConn, method string, _ grpc.Streamer, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	u, err := url.Parse(h.ep + method)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	conn, resp, err := h.wsClient.DialContext(ctx, u.String(), addStreamDescToHeader(desc, http.Header{
		"Content-Type": {"application/protobuf"},
	}))
	if err != nil {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unable to create websocket: %w: %s", err, string(data))
	}

	return &ClientStream{conn: conn}, nil
}

// ClientStream implements the grpc.ClientStream
// TODO: Add final implementation to a lot of them. This is so basic for now.
type ClientStream struct {
	conn *websocket.Conn
}

func (cs *ClientStream) Header() (metadata.MD, error) {
	return nil, nil
}

func (cs *ClientStream) Trailer() metadata.MD {
	return nil
}

func (cs *ClientStream) CloseSend() error {
	return nil
}

func (cs *ClientStream) Context() context.Context {
	return context.Background()
}

func (cs *ClientStream) SendMsg(m interface{}) error {
	v := m.(proto.Message)
	data, err := proto.Marshal(v)
	if err != nil {
		return err
	}
	return cs.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (cs *ClientStream) RecvMsg(m interface{}) error {
	v := m.(proto.Message)

	_, data, err := cs.conn.ReadMessage()
	if err != nil {
		return err
	}
	return proto.Unmarshal(data, v)
}

func addStreamDescToHeader(desc *grpc.StreamDesc, h http.Header) http.Header {
	h.Add("Client-Streams", fmt.Sprint(desc.ClientStreams))
	h.Add("Server-Streams", fmt.Sprint(desc.ServerStreams))
	h.Add("Stream-Name", desc.StreamName)

	return h
}
