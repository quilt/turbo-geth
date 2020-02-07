package main

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/ledgerwatch/turbo-geth/ethdb/remote"
)

func MustConnectRemoteDB(remoteDbAddress string) *remote.DB {
	var remoteDB *remote.DB

	dial := func(ctx context.Context) (in io.Reader, out io.Writer, closer io.Closer, err error) {
		dialer := net.Dialer{}
		conn, err := dialer.DialContext(ctx, "tcp", remoteDbAddress)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("could not connect to remoteDb. addr: %s. err: %w", remoteDbAddress, err)
		}
		return conn, conn, conn, err
	}

	remoteDB, err := remote.NewDB(context.TODO(), dial)
	if err != nil {
		panic(err)
	}
	return remoteDB
}