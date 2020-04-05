package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ledgerwatch/turbo-geth/cmd/utils"
	"github.com/ledgerwatch/turbo-geth/core"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/spf13/cobra"

	"net/http"
	_ "net/http/pprof"
)

var (
	profile     bool
	genesisPath string
	genesis     *core.Genesis
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&profile, "profile", false, "start pprof on port 6060")
	rootCmd.PersistentFlags().StringVar(&genesisPath, "genesis", "", "path to genesis.json file")
}

func rootContext() context.Context {
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

var rootCmd = &cobra.Command{
	Use:   "state",
	Short: "state is a utility for Stateless ethereum clients",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		genesis = core.DefaultGenesisBlock()
		if genesisPath != "" {
			genesis = genesisFromFile(genesisPath)
		}

		startProfilingIfNeeded()
	},
}

func genesisFromFile(genesisPath string) *core.Genesis {
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}
	return genesis
}

func Execute() {
	if err := rootCmd.ExecuteContext(rootContext()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func startProfilingIfNeeded() {
	if profile {
		go func() {
			fmt.Println("starting profiling at localhost:6060")
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}
}
