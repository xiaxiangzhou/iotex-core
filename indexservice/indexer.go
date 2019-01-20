// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package indexservice

import (
	"database/sql"
	"encoding/hex"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/iotexproject/iotex-core/action"
	"github.com/iotexproject/iotex-core/blockchain/block"
	"github.com/iotexproject/iotex-core/config"
	s "github.com/iotexproject/iotex-core/db/sql"
	"github.com/iotexproject/iotex-core/pkg/hash"
	"github.com/iotexproject/iotex-core/proto"
)

type (
	// TransferHistory defines the schema of "transfer history" table
	TransferHistory struct {
		NodeAddress string
		UserAddress string
		TrasferHash string
	}
	// TransferToBlock defines the schema of "transfer hash to block hash" table
	TransferToBlock struct {
		NodeAddress string
		TrasferHash string
		BlockHash   string
	}
	// VoteHistory defines the schema of "vote history" table
	VoteHistory struct {
		NodeAddress string
		UserAddress string
		VoteHash    string
	}
	// VoteToBlock defines the schema of "vote hash to block hash" table
	VoteToBlock struct {
		NodeAddress string
		VoteHash    string
		BlockHash   string
	}
	// ExecutionHistory defines the schema of "execution history" table
	ExecutionHistory struct {
		NodeAddress   string
		UserAddress   string
		ExecutionHash string
	}
	// ExecutionToBlock defines the schema of "execution hash to block hash" table
	ExecutionToBlock struct {
		NodeAddress   string
		ExecutionHash string
		BlockHash     string
	}
	// ActionHistory defines the schema of "action history" table
	ActionHistory struct {
		NodeAddress string
		UserAddress string
		ActionHash  string
	}
	// ActionToBlock defines the schema of "action hash to block hash" table
	ActionToBlock struct {
		NodeAddress string
		ActionHash  string
		BlockHash   string
	}
	// HashToReceipt defines the schema of "hash to receipt" table
	HashToReceipt struct {
		NodeAddress  string
		ReceiptHash  []byte
		ReceiptBytes []byte
	}
)

// Indexer handles the index build for blocks
type Indexer struct {
	cfg                config.Indexer
	store              s.Store
	hexEncodedNodeAddr string
}

var (
	// ErrNotExist indicates certain item does not exist in Blockchain database
	ErrNotExist = errors.New("not exist in DB")
	// ErrAlreadyExist indicates certain item already exists in Blockchain database
	ErrAlreadyExist = errors.New("already exist in DB")
)

// HandleBlock is an implementation of interface BlockCreationSubscriber
func (idx *Indexer) HandleBlock(blk *block.Block) error {
	return idx.BuildIndex(blk)
}

// BuildIndex builds the index for a block
func (idx *Indexer) BuildIndex(blk *block.Block) error {
	idx.store.Transact(func(tx *sql.Tx) error {
		// log transfer to transfer history table
		if err := idx.UpdateTransferHistory(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update transfer to transfer history table")
		}
		// map transfer to block
		if err := idx.UpdateTransferToBlock(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update transfer to block")
		}

		// log vote to vote history table
		if err := idx.UpdateVoteHistory(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update vote to vote history table")
		}
		// map vote to block
		if err := idx.UpdateVoteToBlock(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update vote to block")
		}

		// log execution to execution history table
		if err := idx.UpdateExecutionHistory(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update execution to execution history table")
		}
		// map execution to block
		if err := idx.UpdateExecutionToBlock(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update execution to block")
		}

		// log action to action history table
		if err := idx.UpdateActionHistory(blk, tx); err != nil {
			return errors.Wrapf(err, "failed to update action to action history table")
		}
		// map action to block
		if err := idx.UpdateActionToBlock(blk, tx); err != nil {
			return errors.Wrap(err, "failed to update action to block")
		}

		// map hash to receipt
		if err := idx.UpdateHashToReceipt(blk, tx); err != nil {
			return errors.Wrap(err, "failed to update hash to receipt")
		}

		return nil
	})
	return nil
}

// UpdateTransferHistory stores transfer information into transfer history table
func (idx *Indexer) UpdateTransferHistory(blk *block.Block, tx *sql.Tx) error {
	insertQuery := "INSERT INTO transfer_history (node_address,user_address,transfer_hash) VALUES (?, ?, ?)"
	transfers, _, _ := action.ClassifyActions(blk.Actions)
	for _, transfer := range transfers {
		transferHash := transfer.Hash()

		// put new transfer for sender
		senderAddr := transfer.Sender()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, senderAddr, transferHash[:]); err != nil {
			return err
		}

		// put new transfer for recipient
		receiverAddr := transfer.Recipient()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, receiverAddr, transferHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetTransferHistory gets transfer history
func (idx *Indexer) GetTransferHistory(userAddr string) ([]hash.Hash32B, error) {
	getQuery := "SELECT * FROM transfer_history WHERE node_address=? AND user_address=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, userAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute get query")
	}

	var transferHistory TransferHistory
	parsedRows, err := s.ParseSQLRows(rows, &transferHistory)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse results")
	}

	var transferHashes []hash.Hash32B
	for _, parsedRow := range parsedRows {
		var hash hash.Hash32B
		copy(hash[:], parsedRow.(*TransferHistory).TrasferHash)
		transferHashes = append(transferHashes, hash)
	}
	return transferHashes, nil
}

// UpdateTransferToBlock maps transfer hash to block hash
func (idx *Indexer) UpdateTransferToBlock(blk *block.Block, tx *sql.Tx) error {
	blockHash := blk.HashBlock()
	insertQuery := "INSERT INTO transfer_to_block (node_address,transfer_hash,block_hash) VALUES (?, ?, ?)"
	transfers, _, _ := action.ClassifyActions(blk.Actions)
	for _, transfer := range transfers {
		transferHash := transfer.Hash()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, hex.EncodeToString(transferHash[:]), blockHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetBlockByTransfer returns block hash by transfer hash
func (idx *Indexer) GetBlockByTransfer(transferHash hash.Hash32B) (hash.Hash32B, error) {
	getQuery := "SELECT * FROM transfer_to_block WHERE node_address=? AND transfer_hash=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, hex.EncodeToString(transferHash[:]))
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to execute get query")
	}

	var transferToBlock TransferToBlock
	parsedRows, err := s.ParseSQLRows(rows, &transferToBlock)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to parse results")
	}

	if len(parsedRows) == 0 {
		return hash.ZeroHash32B, ErrNotExist
	}

	var hash hash.Hash32B
	copy(hash[:], parsedRows[0].(*TransferToBlock).BlockHash)
	return hash, nil
}

// UpdateVoteHistory stores vote information into vote history table
func (idx *Indexer) UpdateVoteHistory(blk *block.Block, tx *sql.Tx) error {
	insertQuery := "INSERT INTO vote_history (node_address,user_address,vote_hash) VALUES (?, ?, ?)"
	_, votes, _ := action.ClassifyActions(blk.Actions)
	for _, vote := range votes {
		voteHash := vote.Hash()

		// put new vote for sender
		senderAddr := vote.Voter()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, senderAddr, voteHash[:]); err != nil {
			return err
		}

		// put new vote for recipient
		recipientAddr := vote.Votee()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, recipientAddr, voteHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetVoteHistory gets vote history
func (idx *Indexer) GetVoteHistory(userAddr string) ([]hash.Hash32B, error) {
	getQuery := "SELECT * FROM vote_history WHERE node_address=? AND user_address=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, userAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute get query")
	}

	var voteHistory VoteHistory
	parsedRows, err := s.ParseSQLRows(rows, &voteHistory)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse results")
	}

	var voteHashes []hash.Hash32B
	for _, parsedRow := range parsedRows {
		var hash hash.Hash32B
		copy(hash[:], parsedRow.(*VoteHistory).VoteHash)
		voteHashes = append(voteHashes, hash)
	}
	return voteHashes, nil
}

// UpdateVoteToBlock maps vote hash to block hash
func (idx *Indexer) UpdateVoteToBlock(blk *block.Block, tx *sql.Tx) error {
	blockHash := blk.HashBlock()
	insertQuery := "INSERT INTO vote_to_block (node_address,vote_hash,block_hash) VALUES (?, ?, ?)"
	_, votes, _ := action.ClassifyActions(blk.Actions)
	for _, vote := range votes {
		voteHash := vote.Hash()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, hex.EncodeToString(voteHash[:]), blockHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetBlockByVote returns block hash by vote hash
func (idx *Indexer) GetBlockByVote(voteHash hash.Hash32B) (hash.Hash32B, error) {
	getQuery := "SELECT * FROM vote_to_block WHERE node_address=? AND vote_hash=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, hex.EncodeToString(voteHash[:]))
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to execute get query")
	}

	var voteToBlock VoteToBlock
	parsedRows, err := s.ParseSQLRows(rows, &voteToBlock)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to parse results")
	}

	if len(parsedRows) == 0 {
		return hash.ZeroHash32B, ErrNotExist
	}

	var hash hash.Hash32B
	copy(hash[:], parsedRows[0].(*VoteToBlock).BlockHash)
	return hash, nil
}

// UpdateExecutionHistory stores execution information into execution history table
func (idx *Indexer) UpdateExecutionHistory(blk *block.Block, tx *sql.Tx) error {
	insertQuery := "INSERT INTO execution_history (node_address,user_address,execution_hash) VALUES (?, ?, ?)"
	_, _, executions := action.ClassifyActions(blk.Actions)
	for _, execution := range executions {
		executionHash := execution.Hash()

		// put new execution for executor
		executorAddr := execution.Executor()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, executorAddr, executionHash[:]); err != nil {
			return err
		}

		// put new execution for contract
		contractAddr := execution.Contract()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, contractAddr, executionHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetExecutionHistory gets execution history
func (idx *Indexer) GetExecutionHistory(userAddr string) ([]hash.Hash32B, error) {
	getQuery := "SELECT * FROM execution_history WHERE node_address=? AND user_address=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, userAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute get query")
	}

	var executionHistory ExecutionHistory
	parsedRows, err := s.ParseSQLRows(rows, &executionHistory)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse results")
	}

	var executionHashes []hash.Hash32B
	for _, parsedRow := range parsedRows {
		var hash hash.Hash32B
		copy(hash[:], parsedRow.(*ExecutionHistory).ExecutionHash)
		executionHashes = append(executionHashes, hash)
	}
	return executionHashes, nil
}

// UpdateExecutionToBlock maps execution hash to block hash
func (idx *Indexer) UpdateExecutionToBlock(blk *block.Block, tx *sql.Tx) error {
	blockHash := blk.HashBlock()
	insertQuery := "INSERT INTO execution_to_block (node_address,execution_hash,block_hash) VALUES (?, ?, ?)"
	_, _, executions := action.ClassifyActions(blk.Actions)
	for _, execution := range executions {
		executionHash := execution.Hash()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, hex.EncodeToString(executionHash[:]), blockHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetBlockByExecution returns block hash by execution hash
func (idx *Indexer) GetBlockByExecution(executionHash hash.Hash32B) (hash.Hash32B, error) {
	getQuery := "SELECT * FROM execution_to_block WHERE node_address=? AND execution_hash=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, hex.EncodeToString(executionHash[:]))
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to execute get query")
	}

	var executionToBlock ExecutionToBlock
	parsedRows, err := s.ParseSQLRows(rows, &executionToBlock)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to parse results")
	}

	if len(parsedRows) == 0 {
		return hash.ZeroHash32B, ErrNotExist
	}

	var hash hash.Hash32B
	copy(hash[:], parsedRows[0].(*ExecutionToBlock).BlockHash)
	return hash, nil
}

// UpdateActionHistory stores action information into action history table
func (idx *Indexer) UpdateActionHistory(blk *block.Block, tx *sql.Tx) error {
	insertQuery := "INSERT INTO action_history (node_address,user_address,action_hash) VALUES (?, ?, ?)"
	for _, selp := range blk.Actions {
		actionHash := selp.Hash()

		// put new action for sender
		senderAddr := selp.SrcAddr()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, senderAddr, actionHash[:]); err != nil {
			return err
		}

		// put new transfer for recipient
		receiverAddr := selp.DstAddr()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, receiverAddr, actionHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetActionHistory gets action history
func (idx *Indexer) GetActionHistory(userAddr string) ([]hash.Hash32B, error) {
	getQuery := "SELECT * FROM action_history WHERE node_address=? AND user_address=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, userAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute get query")
	}

	var actionHistory ActionHistory
	parsedRows, err := s.ParseSQLRows(rows, &actionHistory)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse results")
	}

	var actionHashes []hash.Hash32B
	for _, parsedRow := range parsedRows {
		var hash hash.Hash32B
		copy(hash[:], parsedRow.(*ActionHistory).ActionHash)
		actionHashes = append(actionHashes, hash)
	}
	return actionHashes, nil
}

// UpdateActionToBlock maps action hash to block hash
func (idx *Indexer) UpdateActionToBlock(blk *block.Block, tx *sql.Tx) error {
	blockHash := blk.HashBlock()
	insertQuery := "INSERT INTO action_to_block (node_address,action_hash,block_hash) VALUES (?, ?, ?)"
	for _, selp := range blk.Actions {
		actionHash := selp.Hash()
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, hex.EncodeToString(actionHash[:]), blockHash[:]); err != nil {
			return err
		}
	}
	return nil
}

// GetBlockByAction returns block hash by action hash
func (idx *Indexer) GetBlockByAction(actionHash hash.Hash32B) (hash.Hash32B, error) {
	getQuery := "SELECT * FROM action_to_block WHERE node_address=? AND action_hash=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, hex.EncodeToString(actionHash[:]))
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to execute get query")
	}

	var actionToBlock ActionToBlock
	parsedRows, err := s.ParseSQLRows(rows, &actionToBlock)
	if err != nil {
		return hash.ZeroHash32B, errors.Wrapf(err, "failed to parse results")
	}

	if len(parsedRows) == 0 {
		return hash.ZeroHash32B, ErrNotExist
	}

	var hash hash.Hash32B
	copy(hash[:], parsedRows[0].(*ActionToBlock).BlockHash)
	return hash, nil
}

// UpdateHashToReceipt maps action hash to receipt
func (idx *Indexer) UpdateHashToReceipt(blk *block.Block, tx *sql.Tx) error {
	insertQuery := "INSERT INTO hash_to_receipt (node_address,receipt_hash,receipt_bytes) VALUES (?, ?, ?)"
	for hash, receipt := range blk.Receipts {
		receiptBytes, err := proto.Marshal(receipt.ConvertToReceiptPb())
		if err != nil {
			return err
		}
		if _, err := tx.Exec(insertQuery, idx.hexEncodedNodeAddr, hex.EncodeToString(hash[:]), receiptBytes); err != nil {
			return err
		}
	}
	return nil
}

// GetReceiptByHash returns receipt by receipt hash
func (idx *Indexer) GetReceiptByHash(receiptHash hash.Hash32B) (*action.Receipt, error) {
	getQuery := "SELECT * FROM hash_to_receipt WHERE node_address=? AND receipt_hash=?"
	db := idx.store.GetDB()

	stmt, err := db.Prepare(getQuery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare get query")
	}

	rows, err := stmt.Query(idx.hexEncodedNodeAddr, hex.EncodeToString(receiptHash[:]))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute get query")
	}

	var hashToReceipt HashToReceipt
	parsedRows, err := s.ParseSQLRows(rows, &hashToReceipt)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse results")
	}

	if len(parsedRows) == 0 {
		return nil, ErrNotExist
	}
	receiptPb := iproto.ReceiptPb{}
	if err := proto.Unmarshal(parsedRows[0].(*HashToReceipt).ReceiptBytes, &receiptPb); err != nil {
		return nil, err
	}
	receipt := action.Receipt{}
	receipt.ConvertFromReceiptPb(&receiptPb)
	return &receipt, nil
}

// CreateTablesIfNotExist creates tables in local database
func (idx *Indexer) CreateTablesIfNotExist() error {
	// create action tables
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS action_history ([node_address] TEXT NOT NULL, [user_address] " +
		"TEXT NOT NULL, [action_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS action_to_block ([node_address] TEXT NOT NULL, [action_hash] " +
		"BLOB(32) NOT NULL, [block_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}

	// create transfer tables
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS transfer_history ([node_address] TEXT NOT NULL, [user_address] " +
		"TEXT NOT NULL, [transfer_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS transfer_to_block ([node_address] TEXT NOT NULL, [transfer_hash] " +
		"BLOB(32) NOT NULL, [block_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}

	// create vote tables
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS vote_history ([node_address] TEXT NOT NULL, [user_address] " +
		"TEXT NOT NULL, [vote_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS vote_to_block ([node_address] TEXT NOT NULL, [vote_hash] " +
		"BLOB(32) NOT NULL, [block_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}

	// create execution tables
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS execution_history ([node_address] TEXT NOT NULL, [user_address] " +
		"TEXT NOT NULL, [execution_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS execution_to_block ([node_address] TEXT NOT NULL, [execution_hash] " +
		"BLOB(32) NOT NULL, [block_hash] BLOB(32) NOT NULL)"); err != nil {
		return err
	}

	// create receipt index
	if _, err := idx.store.GetDB().Exec("CREATE TABLE IF NOT EXISTS hash_to_receipt ([node_address] TEXT NOT NULL, [receipt_hash] " +
		"BLOB(32) NOT NULL, [receipt_bytes] BLOB NOT NULL)"); err != nil {
		return err
	}

	return nil
}
