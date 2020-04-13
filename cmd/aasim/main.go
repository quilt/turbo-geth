package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ledgerwatch/turbo-geth/cmd/utils"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/p2p/enode"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli"
)

var (
	app = utils.NewApp("", "", "aa simulator")
)

func init() {
	app.Action = entry

	app.Before = func(ctx *cli.Context) error {
		setupLogger(ctx)
		return nil
	}

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "test p2p connection with host",
		},
	}
}

func main() {
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func entry(c *cli.Context) error {
	log.Info("Beginning simulator . . .")

	// set program to shut down on SIGTERM interrupt
	ctx := getRootContext()

	// get local enode address from file
	nodeToConnect, err := getTargetAddr()
	if err != nil {
		return err
	}

	// need to run debug first to send genesis (but why?)
	runDebug(c, ctx, nodeToConnect)

	return runSimulation(c, ctx, nodeToConnect)
}

func runDebug(c *cli.Context, ctx context.Context, enode *enode.Node) error {
	server := makeP2PServer(ctx, NewSimulatorProtocol(), []string{eth.DebugName})

	err := server.Start()
	if err != nil {
		panic(fmt.Errorf("could not start server: %w", err))
	}

	server.AddPeer(enode)
	time.Sleep(2 * time.Second)
	server.Stop()

	return nil
}

func runSimulation(c *cli.Context, ctx context.Context, enode *enode.Node) error {
	blockGen, err := NewBlockGenerator(ctx, 10)
	if err != nil {
		panic(fmt.Sprintf("Failed to create block generator: %v", err))
	}

	sp := NewSimulatorProtocol()
	sp.feeder = blockGen
	sp.protocolVersion = uint32(eth.ProtocolVersions[0])
	sp.networkID = 1
	sp.genesis = sp.feeder.Genesis().Hash()

	server := makeP2PServer(ctx, sp, []string{eth.ProtocolName})
	if err := server.Start(); err != nil {
		panic(fmt.Errorf("could not start server: %w", err))
	}

	server.AddPeer(enode)

	<-ctx.Done()

	return nil

}

func getRootContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(ch)

		select {
		case <-ch:
			log.Info("Got interrupt, shutting down...")
		case <-ctx.Done():
		}

		cancel()
	}()

	return ctx
}

func setupLogger(cliCtx *cli.Context) {
	var (
		ostream log.Handler
		glogger *log.GlogHandler
	)

	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	output := io.Writer(os.Stderr)

	if usecolor {
		output = colorable.NewColorableStderr()
	}

	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))
	glogger = log.NewGlogHandler(ostream)
	log.Root().SetHandler(glogger)
	glogger.Verbosity(log.Lvl(5))
}
