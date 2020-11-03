package commands

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/golang/protobuf/ptypes/empty"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	proto_core "github.com/ledgerwatch/turbo-geth/cmd/headers/core"
	proto_sentry "github.com/ledgerwatch/turbo-geth/cmd/headers/sentry"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/metrics"
	"github.com/ledgerwatch/turbo-geth/rlp"
	"github.com/ledgerwatch/turbo-geth/turbo/stages/headerdownload"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/keepalive"
)

var (
	bufferSizeStr string // Size of buffer
)

func init() {
	startCmd.Flags().StringVar(&sentryAddr, "sentryAddr", "localhost:9091", "sentry address <host>:<port>")
	startCmd.Flags().StringVar(&coreAddr, "coreAddr", "localhost:9092", "core address <host>:<port>")
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Begin running the mempool",
	RunE: func(cmd *cobra.Command, args []string) error {
		return start(sentryAddr, coreAddr)
	},
}

func start(sentryAddr string, coreAddr string) error {
	ctx := rootContext()

	log.Info("Starting mempool")
	log.Info("Starting Core P2P server", "on", coreAddr, "connecting to sentry", coreAddr)

	listenConfig := net.ListenConfig{
		Control: func(network, address string, _ syscall.RawConn) error {
			log.Info("Core P2P received connection", "via", network, "from", address)
			return nil
		},
	}
	lis, err := listenConfig.Listen(ctx, "tcp", coreAddr)
	if err != nil {
		return fmt.Errorf("could not create Core P2P listener: %w, addr=%s", err, coreAddr)
	}
	var (
		streamInterceptors []grpc.StreamServerInterceptor
		unaryInterceptors  []grpc.UnaryServerInterceptor
	)
	if metrics.Enabled {
		streamInterceptors = append(streamInterceptors, grpc_prometheus.StreamServerInterceptor)
		unaryInterceptors = append(unaryInterceptors, grpc_prometheus.UnaryServerInterceptor)
	}
	streamInterceptors = append(streamInterceptors, grpc_recovery.StreamServerInterceptor())
	unaryInterceptors = append(unaryInterceptors, grpc_recovery.UnaryServerInterceptor())
	var grpcServer *grpc.Server
	cpus := uint32(runtime.GOMAXPROCS(-1))
	opts := []grpc.ServerOption{
		grpc.NumStreamWorkers(cpus), // reduce amount of goroutines
		grpc.WriteBufferSize(1024),  // reduce buffers to save mem
		grpc.ReadBufferSize(1024),
		grpc.MaxConcurrentStreams(100), // to force clients reduce concurrency level
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: 10 * time.Minute,
		}),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
	}
	grpcServer = grpc.NewServer(opts...)

	// CREATING GRPC CLIENT CONNECTION
	var dialOpts []grpc.DialOption
	dialOpts = []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.DefaultConfig, MinConnectTimeout: 10 * time.Minute}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(5 * datasize.MB))),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Timeout: 10 * time.Minute,
		}),
	}

	dialOpts = append(dialOpts, grpc.WithInsecure())

	conn, err := grpc.DialContext(ctx, sentryAddr, dialOpts...)
	if err != nil {
		return fmt.Errorf("creating client connection to sentry P2P: %w", err)
	}
	sentryClient := proto_sentry.NewSentryClient(conn)

	var bufferSize datasize.ByteSize
	if err = bufferSize.UnmarshalText([]byte(bufferSizeStr)); err != nil {
		return fmt.Errorf("parsing bufferSize %s: %w", bufferSizeStr, err)
	}
	var controlServer *ControlServerImpl

	if controlServer, err = NewControlServer(filesDir, int(bufferSize), sentryClient); err != nil {
		return fmt.Errorf("create core P2P server: %w", err)
	}
	proto_core.RegisterControlServer(grpcServer, controlServer)
	if metrics.Enabled {
		grpc_prometheus.Register(grpcServer)
	}

	go func() {
		if err1 := grpcServer.Serve(lis); err1 != nil {
			log.Error("Core P2P server fail", "err", err1)
		}
	}()

	controlServer.loop(ctx)

	return nil
}

type ControlServerImpl struct {
	proto_core.UnimplementedControlServer
	lock          sync.Mutex
	sentryClient  proto_sentry.SentryClient
	requestWakeUp chan struct{}
}

func NewControlServer(filesDir string, bufferSize int, sentryClient proto_sentry.SentryClient) (*ControlServerImpl, error) {
	return &ControlServerImpl{sentryClient: sentryClient, requestWakeUp: make(chan struct{})}, nil
}

func (cs *ControlServerImpl) ForwardInboundMessage(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	defer func() {
		select {
		case cs.requestWakeUp <- struct{}{}:
		default:
		}
	}()
	log.Info("new msg", "id", inreq.Id)
	switch inreq.Id {
	default:
		return nil, fmt.Errorf("not implemented for message Id: %s", inreq.Id)
	}
}

func (cs *ControlServerImpl) GetStatus(context.Context, *empty.Empty) (*proto_core.StatusData, error) {
	return nil, nil
}

func (cs *ControlServerImpl) sendRequests(ctx context.Context, reqs []*headerdownload.HeaderRequest) {
	for _, req := range reqs {
		log.Debug(fmt.Sprintf("Sending header request {hash: %x, height: %d, length: %d}", req.Hash, req.Number, req.Length))
		bytes, err := rlp.EncodeToBytes(&eth.GetBlockHeadersData{
			Amount:  uint64(req.Length),
			Reverse: true,
			Skip:    0,
			Origin:  eth.HashOrNumber{Hash: req.Hash},
		})
		if err != nil {
			log.Error("Could not encode header request", "err", err)
			continue
		}
		outreq := proto_sentry.SendMessageByMinBlockRequest{
			MinBlock: req.Number,
			Data: &proto_sentry.OutboundMessageData{
				Id:   proto_sentry.OutboundMessageId_GetBlockHeaders,
				Data: bytes,
			},
		}
		_, err = cs.sentryClient.SendMessageByMinBlock(ctx, &outreq, &grpc.EmptyCallOption{})
		if err != nil {
			log.Error("Could not send header request", "err", err)
			continue
		}
	}
}

func (cs *ControlServerImpl) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cs.requestWakeUp:
			log.Info("Woken up by the incoming request")
		}
	}
}
