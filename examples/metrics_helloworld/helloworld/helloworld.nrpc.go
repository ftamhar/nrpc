// Code generated by protoc-gen-nrpc, DO NOT EDIT.
// source: helloworld.proto

package helloworld

import (
	"context"
	"log"
	"time"

	"github.com/ftamhar/nrpc"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
)

// GreeterServer is the interface that providers of the service
// Greeter should implement.
type GreeterServer interface {
	SayHello(ctx context.Context, req *HelloRequest) (resp *HelloReply, err error)
}

var (
	// The request completion time, measured at client-side.
	clientRCTForGreeter = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "nrpc_client_request_completion_time_seconds",
			Help:       "The request completion time for calls, measured client-side.",
			Objectives: map[float64]float64{0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
			ConstLabels: map[string]string{
				"service": "Greeter",
			},
		},
		[]string{"method"})

	// The handler execution time, measured at server-side.
	serverHETForGreeter = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "nrpc_server_handler_execution_time_seconds",
			Help:       "The handler execution time for calls, measured server-side.",
			Objectives: map[float64]float64{0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
			ConstLabels: map[string]string{
				"service": "Greeter",
			},
		},
		[]string{"method"})

	// The counts of calls made by the client, classified by result type.
	clientCallsForGreeter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nrpc_client_calls_count",
			Help: "The count of calls made by the client.",
			ConstLabels: map[string]string{
				"service": "Greeter",
			},
		},
		[]string{"method", "encoding", "result_type"})

	// The counts of requests handled by the server, classified by result type.
	serverRequestsForGreeter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nrpc_server_requests_count",
			Help: "The count of requests handled by the server.",
			ConstLabels: map[string]string{
				"service": "Greeter",
			},
		},
		[]string{"method", "encoding", "result_type"})
)

// GreeterHandler provides a NATS subscription handler that can serve a
// subscription using a given GreeterServer implementation.
type GreeterHandler struct {
	ctx       context.Context
	workers   *nrpc.WorkerPool
	nc        nrpc.NatsConn
	server    GreeterServer
	encodings []string
}

func NewGreeterHandler(ctx context.Context, nc nrpc.NatsConn, s GreeterServer) *GreeterHandler {
	return &GreeterHandler{
		ctx:       ctx,
		nc:        nc,
		server:    s,
		encodings: []string{"protobuf"},
	}
}

func NewGreeterConcurrentHandler(workers *nrpc.WorkerPool, nc nrpc.NatsConn, s GreeterServer) *GreeterHandler {
	return &GreeterHandler{
		workers: workers,
		nc:      nc,
		server:  s,
	}
}

// SetEncodings sets the output encodings when using a '*Publish' function
func (h *GreeterHandler) SetEncodings(encodings []string) {
	h.encodings = encodings
}

func (h *GreeterHandler) Subject() string {
	return "Greeter.>"
}

func (h *GreeterHandler) Handler(msg *nats.Msg) {
	var ctx context.Context
	if h.workers != nil {
		ctx = h.workers.Context
	} else {
		ctx = h.ctx
	}
	request := nrpc.NewRequest(ctx, h.nc, msg.Subject, msg.Reply)
	// extract method name & encoding from subject
	_, _, name, tail, err := nrpc.ParseSubject(
		"", 0, "Greeter", 0, msg.Subject)
	if err != nil {
		log.Printf("GreeterHanlder: Greeter subject parsing failed: %v", err)
		return
	}

	request.MethodName = name
	request.SubjectTail = tail

	// call handler and form response
	var immediateError *nrpc.Error
	switch name {
	case "SayHello":
		_, request.Encoding, err = nrpc.ParseSubjectTail(0, request.SubjectTail)
		if err != nil {
			log.Printf("SayHelloHanlder: SayHello subject parsing failed: %v", err)
			break
		}
		req := new(HelloRequest)
		if err := nrpc.Unmarshal(request.Encoding, msg.Data, req); err != nil {
			log.Printf("SayHelloHandler: SayHello request unmarshal failed: %v", err)
			immediateError = &nrpc.Error{
				Type:    nrpc.Error_CLIENT,
				Message: "bad request received: " + err.Error(),
			}
			serverRequestsForGreeter.WithLabelValues(
				"SayHello", request.Encoding, "unmarshal_fail").Inc()
		} else {
			request.Handler = func(ctx context.Context) (proto.Message, error) {
				innerResp, err := h.server.SayHello(ctx, req)
				if err != nil {
					return nil, err
				}
				return innerResp, err
			}
		}
	default:
		log.Printf("GreeterHandler: unknown name %q", name)
		immediateError = &nrpc.Error{
			Type:    nrpc.Error_CLIENT,
			Message: "unknown name: " + name,
		}
		serverRequestsForGreeter.WithLabelValues(
			"Greeter", request.Encoding, "name_fail").Inc()
	}
	request.AfterReply = func(request *nrpc.Request, success, replySuccess bool) {
		if !replySuccess {
			serverRequestsForGreeter.WithLabelValues(
				request.MethodName, request.Encoding, "sendreply_fail").Inc()
		}
		if success {
			serverRequestsForGreeter.WithLabelValues(
				request.MethodName, request.Encoding, "success").Inc()
		} else {
			serverRequestsForGreeter.WithLabelValues(
				request.MethodName, request.Encoding, "handler_fail").Inc()
		}
		// report metric to Prometheus
		serverHETForGreeter.WithLabelValues(request.MethodName).Observe(
			request.Elapsed().Seconds())
	}
	if immediateError == nil {
		if h.workers != nil {
			// Try queuing the request
			if err := h.workers.QueueRequest(request); err != nil {
				log.Printf("nrpc: Error queuing the request: %s", err)
			}
		} else {
			// Run the handler synchronously
			request.RunAndReply()
		}
	}

	if immediateError != nil {
		if err := request.SendReply(nil, immediateError); err != nil {
			log.Printf("GreeterHandler: Greeter handler failed to publish the response: %s", err)
			serverRequestsForGreeter.WithLabelValues(
				request.MethodName, request.Encoding, "handler_fail").Inc()
		}
		serverHETForGreeter.WithLabelValues(request.MethodName).Observe(
			request.Elapsed().Seconds())
	}
}

type GreeterClient struct {
	nc       nrpc.NatsConn
	Subject  string
	Encoding string
	Timeout  time.Duration
}

func NewGreeterClient(nc nrpc.NatsConn) *GreeterClient {
	return &GreeterClient{
		nc:       nc,
		Subject:  "Greeter",
		Encoding: "protobuf",
		Timeout:  5 * time.Second,
	}
}

func (c *GreeterClient) SayHello(ctx context.Context, req *HelloRequest) (resp *HelloReply, err error) {
	start := time.Now()

	subject := c.Subject + "." + "SayHello"

	// call
	resp = new(HelloReply)
	err = nrpc.Call(ctx, req, resp, c.nc, subject, c.Encoding)
	if err != nil {
		clientCallsForGreeter.WithLabelValues(
			"SayHello", c.Encoding, "call_fail").Inc()
		return // already logged
	}

	// report total time taken to Prometheus
	elapsed := time.Since(start).Seconds()
	clientRCTForGreeter.WithLabelValues("SayHello").Observe(elapsed)
	clientCallsForGreeter.WithLabelValues(
		"SayHello", c.Encoding, "success").Inc()

	return
}

type Client struct {
	nc              nrpc.NatsConn
	defaultEncoding string
	defaultTimeout  time.Duration
	Greeter         *GreeterClient
}

func NewClient(nc nrpc.NatsConn) *Client {
	c := Client{
		nc:              nc,
		defaultEncoding: "protobuf",
		defaultTimeout:  5 * time.Second,
	}
	c.Greeter = NewGreeterClient(nc)
	return &c
}

func (c *Client) SetEncoding(encoding string) {
	c.defaultEncoding = encoding
	if c.Greeter != nil {
		c.Greeter.Encoding = encoding
	}
}

func (c *Client) SetTimeout(t time.Duration) {
	c.defaultTimeout = t
	if c.Greeter != nil {
		c.Greeter.Timeout = t
	}
}

func init() {
	// register metrics for service Greeter
	prometheus.MustRegister(clientRCTForGreeter)
	prometheus.MustRegister(serverHETForGreeter)
	prometheus.MustRegister(clientCallsForGreeter)
	prometheus.MustRegister(serverRequestsForGreeter)
}
