package commands

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"syscall"
	"time"

	"github.com/c2h5oh/datasize"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	proto_core "github.com/ledgerwatch/turbo-geth/cmd/headers/core"
	proto_sentry "github.com/ledgerwatch/turbo-geth/cmd/headers/sentry"
	"github.com/ledgerwatch/turbo-geth/cmd/mempool/control"
	"github.com/ledgerwatch/turbo-geth/core"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/metrics"
	"github.com/ledgerwatch/turbo-geth/params"
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

	log.Info("Starting mempool", "on", coreAddr, "sentry", sentryAddr)

	txCacher := core.NewTxSenderCacher(runtime.NumCPU())
	pool := core.NewTxPool(core.DefaultTxPoolConfig, params.MainnetChainConfig, nil, txCacher)

	sentryClient, _ := startSentryClient(ctx, sentryAddr)
	controlServer, _ := startControlServer(ctx, sentryClient, pool, coreAddr)

	controlServer.Loop(ctx)

	return nil
}

func startControlServer(ctx context.Context, sentryClient proto_sentry.SentryClient, pool *core.TxPool, coreAddr string) (*control.ControlServer, error) {
	listenConfig := net.ListenConfig{
		Control: func(network, address string, _ syscall.RawConn) error {
			log.Info("mempool received connection", "via", network, "from", address)
			return nil
		},
	}

	lis, err := listenConfig.Listen(ctx, "tcp", coreAddr)
	if err != nil {
		return nil, fmt.Errorf("could not create Core P2P listener: %w, addr=%s", err, coreAddr)
	}

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
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(getStreamInterceptors()...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(getUnaryInterceptors()...)),
	}

	grpcServer = grpc.NewServer(opts...)

	var controlServer *control.ControlServer
	if controlServer, err = control.NewControlServer(sentryClient, pool); err != nil {
		return nil, fmt.Errorf("create core P2P server: %w", err)
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

	return controlServer, nil
}

func startSentryClient(ctx context.Context, sentryAddr string) (proto_sentry.SentryClient, error) {
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
		return nil, fmt.Errorf("creating client connection to sentry P2P: %w", err)
	}

	return proto_sentry.NewSentryClient(conn), nil
}

func getStreamInterceptors() []grpc.StreamServerInterceptor {
	var streamInterceptors []grpc.StreamServerInterceptor
	if metrics.Enabled {
		streamInterceptors = append(streamInterceptors, grpc_prometheus.StreamServerInterceptor)
	}
	streamInterceptors = append(streamInterceptors, grpc_recovery.StreamServerInterceptor())
	return streamInterceptors

}

func getUnaryInterceptors() []grpc.UnaryServerInterceptor {
	var unaryInterceptors []grpc.UnaryServerInterceptor
	if metrics.Enabled {
		unaryInterceptors = append(unaryInterceptors, grpc_prometheus.UnaryServerInterceptor)
	}
	unaryInterceptors = append(unaryInterceptors, grpc_recovery.UnaryServerInterceptor())
	return unaryInterceptors
}
