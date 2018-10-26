// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

// This is a testing tool to inject fake actions to the blockchain
// To use, run "make build" and " ./bin/indexbuilder"

package main

import (
	"flag"
	"fmt"
	_ "go.uber.org/automaxprocs"
	"github.com/iotexproject/iotex-core/explorer"
	"github.com/iotexproject/iotex-core/logger"
)

func main() {
	// start block id of the index build
	var fromBlockID int64
	// end block id of the index build
	var toBlockID int64
	// end point of rds
	var batchSize int64
	// retry limit
	var retryLimit int
	// target address for jrpc connection. Default is "127.0.0.1:14004"
	var explorerAddr string

	flag.Int64Var(&fromBlockID, "from-block-id", 0, "sync from which block id")
	flag.Int64Var(&toBlockID, "to-block-id", 0, "sync to which block id")
	flag.Int64Var(&batchSize, "batch-size", 1, "batch size")
	flag.IntVar(&retryLimit, "retry-number", 3, "retry number")
	flag.StringVar(&explorerAddr, "explorer-addr", "127.0.0.1:14004", "target ip:port for jrpc connection")
	flag.Parse()

	proxy := explorer.NewExplorerProxy("http://" + explorerAddr)
	for i := fromBlockID; i <= toBlockID; i += batchSize {
		startBlock := i
		endBlock := startBlock + batchSize - 1
		if endBlock > toBlockID {
			endBlock = toBlockID
		}

		retry := 0
		for retry < retryLimit {
			failedBlock, err := proxy.BuildIndexByRange(startBlock, endBlock)
			if err == nil {
				break
			}

			startBlock = failedBlock
			retry++

			if retry == retryLimit {
				logger.Fatal().Err(err).Msg(fmt.Sprintf("error when build index for block height <%d>", failedBlock))
				return
			}
		}
		logger.Info().Msgf("finished build index for range <%d, %d>", i, endBlock)
	}
}
