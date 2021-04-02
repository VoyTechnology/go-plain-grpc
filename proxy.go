package plaingrpc

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
)

type Proxy struct {
	cc       *grpc.ClientConn
	mux      *http.ServeMux
	upgrader websocket.Upgrader
}

func NewProxy(s *grpc.Server, addr net.Addr) (*Proxy, error) {
	cc, err := grpc.Dial(
		addr.String(),
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&codec{})),
	)
	if err != nil {
		return nil, err
	}

	p := &Proxy{
		cc:  cc,
		mux: http.NewServeMux(),
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 0,
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
		},
	}

	for service, info := range s.GetServiceInfo() {
		for _, method := range info.Methods {
			path := fmt.Sprintf("/%s/%s", service, method.Name)
			if method.IsClientStream || method.IsServerStream {
				p.mux.HandleFunc(path, p.streamHandler)
			} else {
				p.mux.HandleFunc(path, p.unaryHandler)
			}
		}
	}

	return p, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mux.ServeHTTP(w, r)
}

func (p *Proxy) unaryHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var res []byte
	if err := p.cc.Invoke(r.Context(), r.URL.String(), body, &res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err = w.Write(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (p *Proxy) streamHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := p.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusSwitchingProtocols)
	}
	defer ws.Close()

	s, err := p.cc.NewStream(r.Context(), streamDescFromHeader(r.Header), r.URL.String())
	if err != nil {
		_ = ws.WriteMessage(websocket.CloseMessage, nil)
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)

		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			if err := s.SendMsg(data); err != nil {
				errCh <- err
				return
			}
		}
	}()

	defer ws.WriteMessage(websocket.CloseMessage, nil)

	for {
		select {
		case <-errCh:
			return
		default:
		}

		data := new([]byte)
		if err := s.RecvMsg(data); err != nil {
			return
		}

		if err := ws.WriteMessage(websocket.BinaryMessage, *data); err != nil {
			return
		}
	}
}

type codec struct{}

func (*codec) Marshal(value interface{}) ([]byte, error) {
	v := value.([]byte)
	return v, nil
}

func (*codec) Unmarshal(data []byte, value interface{}) error {
	v := value.(*[]byte)
	*v = data
	return nil
}

func (*codec) Name() string {
	return "bytes"
}

func streamDescFromHeader(h http.Header) *grpc.StreamDesc {
	return &grpc.StreamDesc{
		StreamName:    h.Get("Stream-Name"),
		ClientStreams: h.Get("Client-Streams") == "true",
		ServerStreams: h.Get("Server-Streams") == "true",
	}
}
