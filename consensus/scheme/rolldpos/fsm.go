// Copyright (c) 2018 IoTeX
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package rolldpos

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/facebookgo/clock"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zjshen14/go-fsm"
	"golang.org/x/crypto/blake2b"

	"github.com/iotexproject/iotex-core/blockchain"
	"github.com/iotexproject/iotex-core/crypto"
	"github.com/iotexproject/iotex-core/iotxaddress"
	"github.com/iotexproject/iotex-core/logger"
	"github.com/iotexproject/iotex-core/pkg/enc"
	"github.com/iotexproject/iotex-core/pkg/hash"
	"github.com/iotexproject/iotex-core/pkg/keypair"
	"github.com/iotexproject/iotex-core/proto"
)

/**
 * TODO:
 *  1. Store endorse decisions of follow up status
 *  2. For the nodes received correct proposal, add proposer's proposal endorse without signature, which could be replaced with real signature
 */

var (
	consensusMtc = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iotex_consensus",
			Help: "Consensus result",
		},
		[]string{"result"},
	)
)

func init() {
	prometheus.MustRegister(consensusMtc)
}

const (
	// consensusEvt states
	sEpochStart            fsm.State = "S_EPOCH_START"
	sDKGGeneration         fsm.State = "S_DKG_GENERATION"
	sRoundStart            fsm.State = "S_ROUND_START"
	sInitPropose           fsm.State = "S_INIT_PROPOSE"
	sAcceptPropose         fsm.State = "S_ACCEPT_PROPOSE"
	sAcceptProposalEndorse fsm.State = "S_ACCEPT_PROPOSAL_ENDROSE"
	sAcceptCommitEndorse   fsm.State = "S_ACCEPT_COMMIT_ENDORSE"

	// sInvalid indicates an invalid state. It doesn't matter what dst state to return when there's an error. Transition
	// to dst state will not happen. However, we should always return to this state to be consistent.
	sInvalid fsm.State = "S_INVALID"

	// consensusEvt event types
	eRollDelegates          fsm.EventType = "E_ROLL_DELEGATES"
	eGenerateDKG            fsm.EventType = "E_GENERATE_DKG"
	eStartRound             fsm.EventType = "E_START_ROUND"
	eInitBlock              fsm.EventType = "E_INIT_BLOCK"
	eProposeBlock           fsm.EventType = "E_PROPOSE_BLOCK"
	eProposeBlockTimeout    fsm.EventType = "E_PROPOSE_BLOCK_TIMEOUT"
	eEndorseProposal        fsm.EventType = "E_ENDORSE_PROPOSAL"
	eEndorseProposalTimeout fsm.EventType = "E_ENDORSE_PROPOSAL_TIMEOUT"
	eEndorseCommit          fsm.EventType = "E_ENDORSE_COMMIT"
	eEndorseCommitTimeout   fsm.EventType = "E_ENDORSE_COMMIT_TIMEOUT"
	eFinishEpoch            fsm.EventType = "E_FINISH_EPOCH"

	// eBackdoor indicates an backdoor event type
	eBackdoor fsm.EventType = "E_BACKDOOR"
)

var (
	// ErrEvtCast indicates the error of casting the event
	ErrEvtCast = errors.New("error when casting the event")
	// ErrEvtConvert indicates the error of converting the event from/to the proto message
	ErrEvtConvert = errors.New("error when converting the event from/to the proto message")

	// consensusStates is a slice consisting of all consensusEvt states
	consensusStates = []fsm.State{
		sEpochStart,
		sDKGGeneration,
		sRoundStart,
		sInitPropose,
		sAcceptPropose,
		sAcceptProposalEndorse,
		sAcceptCommitEndorse,
	}
)

// iConsensusEvt is the interface of all events for the consensusEvt FSM
type iConsensusEvt interface {
	fsm.Event
	timestamp() time.Time
	// TODO: we need to add height or some other ctx to identify which consensus round the event is associated to
}

type consensusEvt struct {
	t  fsm.EventType
	ts time.Time
}

func newCEvt(t fsm.EventType, c clock.Clock) *consensusEvt {
	return &consensusEvt{
		t:  t,
		ts: c.Now(),
	}
}

func (e *consensusEvt) Type() fsm.EventType { return e.t }

func (e *consensusEvt) timestamp() time.Time { return e.ts }

type proposeBlkEvt struct {
	consensusEvt
	block *blockchain.Block
}

func newProposeBlkEvt(block *blockchain.Block, c clock.Clock) *proposeBlkEvt {
	return &proposeBlkEvt{
		consensusEvt: *newCEvt(eProposeBlock, c),
		block:        block,
	}
}

func (e *proposeBlkEvt) toProtoMsg() *iproto.ProposePb {
	return &iproto.ProposePb{
		Block:    e.block.ConvertToBlockPb(),
		Proposer: e.block.ProducerAddress(),
	}
}

func (e *proposeBlkEvt) fromProtoMsg(pMsg *iproto.ProposePb) error {
	if pMsg.Block != nil {
		e.block = &blockchain.Block{}
		e.block.ConvertFromBlockPb(pMsg.Block)
	}
	return nil
}

const (
	endorseProposal = false
	endorseCommit   = true
)

type endorse struct {
	topic          bool
	height         uint64
	blkHash        hash.Hash32B
	decision       bool
	endorser       string
	endorserPubkey keypair.PublicKey
	signature      []byte
}

// ByteStream returns a raw byte stream
func (en *endorse) ByteStream() []byte {
	stream := make([]byte, 8)
	enc.MachineEndian.PutUint64(stream, en.height)
	if en.topic {
		stream = append(stream, 1)
	} else {
		stream = append(stream, 0)
	}
	stream = append(stream, en.blkHash[:]...)
	if en.decision {
		stream = append(stream, 1)
	} else {
		stream = append(stream, 0)
	}
	return stream
}

// Hash returns the hash of the endorse for signature
func (en *endorse) Hash() hash.Hash32B {
	return blake2b.Sum256(en.ByteStream())
}

// Sign signs with endorser's private key
func (en *endorse) Sign(endorser *iotxaddress.Address) error {
	if endorser.PrivateKey == keypair.ZeroPrivateKey {
		return errors.New("The endorser's private key is empty")
	}
	hash := en.Hash()
	en.endorser = endorser.RawAddress
	en.endorserPubkey = endorser.PublicKey
	en.signature = crypto.EC283.Sign(endorser.PrivateKey, hash[:])
	return nil
}

// VerifySignature verifies that the endorse with pubkey
func (en *endorse) VerifySignature(pubkey keypair.PublicKey) bool {
	pubkeyHash := keypair.HashPubKey(pubkey)
	endorserPubkeyHash, err := iotxaddress.GetPubkeyHash(en.endorser)
	if err != nil {
		return false
	}
	if !bytes.Equal(pubkeyHash[:], endorserPubkeyHash) {
		return false
	}
	hash := en.Hash()
	return crypto.EC283.Verify(pubkey, hash[:], en.signature)
}

func (en *endorse) toProtoMsg() *iproto.EndorsePb {
	var topic iproto.EndorsePb_EndorsementTopic
	switch en.topic {
	case endorseProposal:
		topic = iproto.EndorsePb_PROPOSAL
	case endorseCommit:
		topic = iproto.EndorsePb_COMMIT
	}
	return &iproto.EndorsePb{
		Height:         en.height,
		BlockHash:      en.blkHash[:],
		Topic:          topic,
		Endorser:       en.endorser,
		EndorserPubKey: en.endorserPubkey[:],
		Decision:       en.decision,
		Signature:      en.signature[:],
	}
}

func (en *endorse) fromProtoMsg(endorsePb *iproto.EndorsePb) error {
	copy(en.blkHash[:], endorsePb.BlockHash)
	switch endorsePb.Topic {
	case iproto.EndorsePb_PROPOSAL:
		en.topic = endorseProposal
	case iproto.EndorsePb_COMMIT:
		en.topic = endorseCommit
	}
	pubKey, err := keypair.BytesToPublicKey(endorsePb.EndorserPubKey)
	if err != nil {
		logger.Error().
			Err(err).
			Bytes("endorserPubKey", endorsePb.EndorserPubKey).
			Msg("error when constructing endorse from proto message")
		return err
	}
	en.endorserPubkey = pubKey
	en.height = endorsePb.Height
	en.endorser = endorsePb.Endorser
	en.decision = endorsePb.Decision
	copy(en.signature, endorsePb.Signature)
	return nil
}

type endorseEvt struct {
	consensusEvt
	endorse *endorse
}

func newEndorseEvt(topic bool, blkHash hash.Hash32B, decision bool, height uint64, endorser *iotxaddress.Address, c clock.Clock) (*endorseEvt, error) {
	endorse := &endorse{
		height:   height,
		topic:    topic,
		blkHash:  blkHash,
		decision: decision,
	}
	if err := endorse.Sign(endorser); err != nil {
		logger.Error().Err(err).Bytes("Block Hash", blkHash[:]).Str("endorser", endorser.RawAddress).Msg("failed to sign endorse for block")
		return nil, err
	}

	return newEndorseEvtWithEndorse(endorse, c), nil
}

func newEndorseEvtWithEndorse(endorse *endorse, c clock.Clock) *endorseEvt {
	var eventType fsm.EventType
	if endorse.topic == endorseProposal {
		eventType = eEndorseProposal
	} else {
		eventType = eEndorseCommit
	}
	return &endorseEvt{
		consensusEvt: *newCEvt(eventType, c),
		endorse:      endorse,
	}
}

func (e *endorseEvt) toProtoMsg() *iproto.EndorsePb {
	return e.endorse.toProtoMsg()
}

type timeoutEvt struct {
	consensusEvt
}

func newTimeoutEvt(t fsm.EventType, c clock.Clock) *timeoutEvt {
	return &timeoutEvt{
		consensusEvt: *newCEvt(t, c),
	}
}

// backdoorEvt is used for testing purpose to set the consensusEvt FSM to any particular state
type backdoorEvt struct {
	consensusEvt
	dst fsm.State
}

func newBackdoorEvt(dst fsm.State, c clock.Clock) *backdoorEvt {
	return &backdoorEvt{
		consensusEvt: *newCEvt(eBackdoor, c),
		dst:          dst,
	}
}

// cFSM wraps over the general purpose FSM and implements the consensusEvt logic
type cFSM struct {
	fsm   fsm.FSM
	evtq  chan iConsensusEvt
	close chan interface{}
	ctx   *rollDPoSCtx
	wg    sync.WaitGroup
}

func newConsensusFSM(ctx *rollDPoSCtx) (*cFSM, error) {
	cm := &cFSM{
		evtq:  make(chan iConsensusEvt, ctx.cfg.EventChanSize),
		close: make(chan interface{}),
		ctx:   ctx,
	}
	b := fsm.NewBuilder().
		AddInitialState(sEpochStart).
		AddStates(sDKGGeneration, sRoundStart, sInitPropose, sAcceptPropose, sAcceptProposalEndorse, sAcceptCommitEndorse).
		AddTransition(sEpochStart, eRollDelegates, cm.handleRollDelegatesEvt, []fsm.State{sEpochStart, sDKGGeneration}).
		AddTransition(sDKGGeneration, eGenerateDKG, cm.handleGenerateDKGEvt, []fsm.State{sRoundStart}).
		AddTransition(sRoundStart, eStartRound, cm.handleStartRoundEvt, []fsm.State{sInitPropose, sAcceptPropose}).
		AddTransition(sRoundStart, eFinishEpoch, cm.handleFinishEpochEvt, []fsm.State{sEpochStart, sRoundStart}).
		AddTransition(sInitPropose, eInitBlock, cm.handleInitBlockEvt, []fsm.State{sAcceptPropose}).
		AddTransition(
			sAcceptPropose,
			eProposeBlock,
			cm.handleProposeBlockEvt,
			[]fsm.State{
				sAcceptPropose,         // proposed block invalid
				sAcceptProposalEndorse, // proposed block valid
			}).
		AddTransition(
			sAcceptPropose,
			eProposeBlockTimeout,
			cm.handleProposeBlockTimeout,
			[]fsm.State{
				sAcceptProposalEndorse, // no valid block, jump to next step
			}).
		AddTransition(
			sAcceptProposalEndorse,
			eEndorseProposal,
			cm.handleEndorseProposalEvt,
			[]fsm.State{
				sAcceptProposalEndorse, // haven't reach agreement yet
				sAcceptCommitEndorse,   // reach agreement
			}).
		AddTransition(
			sAcceptProposalEndorse,
			eEndorseProposalTimeout,
			cm.handleEndorseProposalTimeout,
			[]fsm.State{
				sAcceptCommitEndorse, // timeout, jump to next step
			}).
		AddTransition(
			sAcceptCommitEndorse,
			eEndorseCommit,
			cm.handleEndorseCommitEvt,
			[]fsm.State{
				sAcceptCommitEndorse, // haven't reach agreement yet
				sRoundStart,          // reach commit agreement, jump to next round
			}).
		AddTransition(
			sAcceptCommitEndorse,
			eEndorseCommitTimeout,
			cm.handleEndorseCommitTimeout,
			[]fsm.State{
				sRoundStart, // timeout, jump to next round
			})
	// Add the backdoor transition so that we could unit test the transition from any given state
	for _, state := range consensusStates {
		b = b.AddTransition(state, eBackdoor, cm.handleBackdoorEvt, consensusStates)
	}
	m, err := b.Build()
	if err != nil {
		return nil, errors.Wrap(err, "error when building the FSM")
	}
	cm.fsm = m
	return cm, nil
}

func (m *cFSM) Start(c context.Context) error {
	m.wg.Add(1)
	go func() {
		running := true
		for running {
			select {
			case <-m.close:
				running = false
			case evt := <-m.evtq:
				timeoutEvt, ok := evt.(*timeoutEvt)
				if ok && timeoutEvt.timestamp().Before(m.ctx.round.timestamp) {
					logger.Debug().Msg("timeoutEvt is stale")
					continue
				}
				src := m.fsm.CurrentState()
				if err := m.fsm.Handle(evt); err != nil {
					if errors.Cause(err) == fsm.ErrTransitionNotFound {
						if m.ctx.clock.Now().Sub(evt.timestamp()) <= m.ctx.cfg.UnmatchedEventTTL {
							m.produce(evt, m.ctx.cfg.UnmatchedEventInterval)
							logger.Debug().
								Str("src", string(src)).
								Str("evt", string(evt.Type())).
								Err(err).
								Msg("consensusEvt state transition could find the match")
						}
					} else {
						logger.Error().
							Str("src", string(src)).
							Str("evt", string(evt.Type())).
							Err(err).
							Msg("consensusEvt state transition fails")
					}
				} else {
					dst := m.fsm.CurrentState()
					logger.Debug().
						Str("src", string(src)).
						Str("dst", string(dst)).
						Str("evt", string(evt.Type())).
						Msg("consensusEvt state transition happens")
				}
			}
		}
		m.wg.Done()
	}()
	return nil
}

func (m *cFSM) Stop(_ context.Context) error {
	close(m.close)
	m.wg.Wait()
	return nil
}

func (m *cFSM) currentState() fsm.State {
	return m.fsm.CurrentState()
}

// produce adds an event into the queue for the consensus FSM to process
func (m *cFSM) produce(evt iConsensusEvt, delay time.Duration) {
	if delay > 0 {
		m.wg.Add(1)
		go func() {
			select {
			case <-m.close:
			case <-m.ctx.clock.After(delay):
				m.evtq <- evt
			}
			m.wg.Done()
		}()
	} else {
		m.evtq <- evt
	}
}

func (m *cFSM) handleRollDelegatesEvt(_ fsm.Event) (fsm.State, error) {
	epochNum, epochHeight, err := m.ctx.calcEpochNumAndHeight()
	if err != nil {
		// Even if error happens, we still need to schedule next check of delegate to tolerate transit error
		m.produce(m.newCEvt(eRollDelegates), m.ctx.cfg.DelegateInterval)
		return sInvalid, errors.Wrap(
			err,
			"error when determining the epoch ordinal number and start height offset",
		)
	}
	delegates, err := m.ctx.rollingDelegates(epochNum)
	if err != nil {
		// Even if error happens, we still need to schedule next check of delegate to tolerate transit error
		m.produce(m.newCEvt(eRollDelegates), m.ctx.cfg.DelegateInterval)
		return sInvalid, errors.Wrap(
			err,
			"error when determining if the node will participate into next epoch",
		)
	}
	// If the current node is the delegate, move to the next state
	if m.isDelegate(delegates) {
		// Get the sub-epoch number
		numSubEpochs := uint(1)
		if m.ctx.cfg.NumSubEpochs > 0 {
			numSubEpochs = m.ctx.cfg.NumSubEpochs
		}

		// The epochStart start height is going to be the next block to generate
		m.ctx.epoch = epochCtx{
			num:          epochNum,
			height:       epochHeight,
			delegates:    delegates,
			numSubEpochs: numSubEpochs,
		}

		// Trigger the event to generate DKG
		m.produce(m.newCEvt(eGenerateDKG), 0)

		logger.Info().
			Uint64("epoch", epochNum).
			Msg("current node is the delegate")
		return sDKGGeneration, nil
	}
	// Else, stay at the current state and check again later
	m.produce(m.newCEvt(eRollDelegates), m.ctx.cfg.DelegateInterval)
	logger.Info().
		Uint64("epoch", epochNum).
		Msg("current node is not the delegate")
	return sEpochStart, nil
}

func (m *cFSM) handleGenerateDKGEvt(_ fsm.Event) (fsm.State, error) {
	dkg, err := m.ctx.generateDKG()
	if err != nil {
		return sInvalid, err
	}
	m.ctx.epoch.dkg = dkg
	if err := m.produceStartRoundEvt(); err != nil {
		return sInvalid, errors.Wrapf(err, "error when producing %s", eStartRound)
	}
	return sRoundStart, nil
}

func (m *cFSM) handleStartRoundEvt(_ fsm.Event) (fsm.State, error) {
	proposer, height, err := m.ctx.rotatedProposer()
	if err != nil {
		logger.Error().
			Err(err).
			Msg("error when getting the proposer")
		return sInvalid, err
	}
	m.ctx.round = roundCtx{
		height:           height,
		timestamp:        m.ctx.clock.Now(),
		proposalEndorses: make(map[hash.Hash32B]map[string]bool),
		commitEndorses:   make(map[hash.Hash32B]map[string]bool),
		proposer:         proposer,
	}
	if proposer == m.ctx.addr.RawAddress {
		logger.Info().
			Str("proposer", proposer).
			Uint64("height", height).
			Msg("current node is the proposer")
		m.produce(m.newCEvt(eInitBlock), 0)
		// TODO: we may need timeout event for block producer too
		return sInitPropose, nil
	}
	logger.Info().
		Str("proposer", proposer).
		Uint64("height", height).
		Msg("current node is not the proposer")
	// Setup timeout for waiting for proposed block
	m.produce(m.newTimeoutEvt(eProposeBlockTimeout, m.ctx.round.height), m.ctx.cfg.AcceptProposeTTL)
	return sAcceptPropose, nil
}

func (m *cFSM) handleInitBlockEvt(evt fsm.Event) (fsm.State, error) {
	blk, err := m.ctx.mintBlock()
	if err != nil {
		return sInvalid, errors.Wrap(err, "error when minting a block")
	}
	proposeBlkEvt := m.newProposeBlkEvt(blk)
	proposeBlkEvtProto := proposeBlkEvt.toProtoMsg()
	// Notify itself
	m.produce(proposeBlkEvt, 0)
	// Notify other delegates
	if err := m.ctx.p2p.Broadcast(m.ctx.chain.ChainID(), proposeBlkEvtProto); err != nil {
		logger.Error().
			Err(err).
			Msg("error when broadcasting proposeBlkEvt")
	}
	return sAcceptPropose, nil
}

func (m *cFSM) validateProposeBlock(blk *blockchain.Block, expectedProposer string) bool {
	blkHash := blk.HashBlock()
	errorLog := logger.Error().
		Uint64("expectedHeight", m.ctx.round.height).
		Str("expectedProposer", expectedProposer).
		Str("hash", hex.EncodeToString(blkHash[:]))
	if blk.Height() != m.ctx.round.height {
		errorLog.Uint64("blockHeight", blk.Height()).
			Msg("error when validating the block height")
		return false
	}
	producer := blk.ProducerAddress()

	if producer == "" || producer != expectedProposer {
		errorLog.Str("proposer", producer).
			Msg("error when validating the block proposer")
		return false
	}
	if !blk.VerifySignature() {
		errorLog.Msg("error when validating the block signature")
		return false
	}
	if producer == m.ctx.addr.RawAddress {
		// If the block is self proposed, skip validation
		return true
	}
	if err := m.ctx.chain.ValidateBlock(blk, true); err != nil {
		errorLog.Err(err).Msg("error when validating the proposed block")
		return false
	}
	if len(blk.Header.DKGPubkey) > 0 && len(blk.Header.DKGBlockSig) > 0 {
		// TODO failed if no dkg
		if err := verifyDKGSignature(blk, m.ctx.epoch.seed); err != nil {
			// Verify dkg signature failed
			errorLog.Err(err).Msg("Failed to verify the DKG signature")
			return false
		}
	}

	return true
}

func (m *cFSM) moveToAcceptProposalEndorse() (fsm.State, error) {
	// Setup timeout for waiting for endorse
	m.produce(m.newTimeoutEvt(eEndorseProposalTimeout, m.ctx.round.height), m.ctx.cfg.AcceptProposalEndorseTTL)
	return sAcceptProposalEndorse, nil
}

func (m *cFSM) handleProposeBlockEvt(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eProposeBlock {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	m.ctx.round.block = nil
	proposeBlkEvt, ok := evt.(*proposeBlkEvt)
	if !ok {
		return sInvalid, errors.Wrap(ErrEvtCast, "the event is not a proposeBlkEvt")
	}
	proposer, err := m.ctx.calcProposer(proposeBlkEvt.block.Height(), m.ctx.epoch.delegates)
	if err != nil {
		return sInvalid, errors.Wrap(err, "error when calculating the proposer")
	}
	if !m.validateProposeBlock(proposeBlkEvt.block, proposer) {
		return sAcceptPropose, nil
	}
	m.ctx.round.block = proposeBlkEvt.block
	endorseEvt, err := m.newEndorseProposalEvt(m.ctx.round.block.HashBlock(), true)
	if err != nil {
		return sInvalid, errors.Wrap(err, "error when generating new endorse proposal event")
	}
	endorseEvtProto := endorseEvt.toProtoMsg()
	// Notify itself
	m.produce(endorseEvt, 0)
	// Notify other delegates
	if err := m.ctx.p2p.Broadcast(m.ctx.chain.ChainID(), endorseEvtProto); err != nil {
		logger.Error().
			Err(err).
			Msg("error when broadcasting endorseEvtProto")
	}

	return m.moveToAcceptProposalEndorse()
}

func (m *cFSM) handleProposeBlockTimeout(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eProposeBlockTimeout {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	logger.Warn().
		Str("proposer", m.ctx.round.proposer).
		Uint64("height", m.ctx.round.height).
		Msg("didn't receive the proposed block before timeout")

	return m.moveToAcceptProposalEndorse()
}

func (m *cFSM) validateEndorse(en *endorse, expectedEndorseTopic bool) bool {
	errorLog := logger.Error().
		Uint64("expectedHeight", m.ctx.round.height).
		Bool("expectedEndorseTopic", expectedEndorseTopic)
	if en.topic != expectedEndorseTopic {
		errorLog.Bool("endorseTopic", en.topic).
			Msg("error when validating the endorse topic")
		return false
	}
	if en.height != m.ctx.round.height {
		errorLog.Uint64("height", en.height).
			Msg("error when validating the endorse height")
		return false
	}
	// TODO verify that the endorser is one delegate, and verify signature via endorse.VerifySignature() with pub key
	return true
}

func (m *cFSM) moveToAcceptCommitEndorse() (fsm.State, error) {
	// Setup timeout for waiting for commit
	m.produce(m.newTimeoutEvt(eEndorseCommitTimeout, m.ctx.round.height), m.ctx.cfg.AcceptCommitEndorseTTL)
	return sAcceptCommitEndorse, nil
}

func (m *cFSM) handleEndorseProposalEvt(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eEndorseProposal {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	endorseEvt, ok := evt.(*endorseEvt)
	if !ok {
		return sInvalid, errors.Wrap(ErrEvtCast, "the event is not an endorseEvt")
	}
	endorse := endorseEvt.endorse
	if !m.validateEndorse(endorse, endorseProposal) {
		return sAcceptProposalEndorse, nil
	}
	blkHash := endorse.blkHash
	endorses := m.ctx.round.proposalEndorses[blkHash]
	if endorses == nil {
		endorses = map[string]bool{}
		m.ctx.round.proposalEndorses[blkHash] = endorses
	}
	endorses[endorse.endorser] = endorse.decision
	// if ether yes or no is true, block must exists and blkHash must be a valid one
	yes, no := m.ctx.calcQuorum(m.ctx.round.proposalEndorses[blkHash])
	if !yes && !no {
		// Wait for more preCommits to come
		return sAcceptProposalEndorse, nil
	}
	// Reached the agreement
	cEvt, err := m.newEndorseCommitEvt(blkHash, yes && !no)
	if err != nil {
		return sInvalid, errors.Wrap(err, "failed to generate endorse commit event")
	}
	cEvtProto := cEvt.toProtoMsg()
	// Notify itself
	m.produce(cEvt, 0)
	// Notify other delegates
	if err := m.ctx.p2p.Broadcast(m.ctx.chain.ChainID(), cEvtProto); err != nil {
		logger.Error().
			Err(err).
			Msg("error when broadcasting commitEvtProto")
	}

	return m.moveToAcceptCommitEndorse()
}

func (m *cFSM) handleEndorseProposalTimeout(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eEndorseProposalTimeout {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	logger.Warn().
		Uint64("height", m.ctx.round.height).
		Int("numberOfEndorses", len(m.ctx.round.proposalEndorses)).
		Msg("didn't collect enough proposal endorses before timeout")

	return m.moveToAcceptCommitEndorse()
}

func (m *cFSM) handleEndorseCommitEvt(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eEndorseCommit {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	endorseEvt, ok := evt.(*endorseEvt)
	if !ok {
		return sInvalid, errors.Wrap(ErrEvtCast, "the event is not an endorseEvt")
	}
	endorse := endorseEvt.endorse
	if endorse.topic != endorseCommit {
		return sAcceptCommitEndorse, nil
	}
	// TODO verify that the endorse is one delegate, and verify signature via endorse.VerifySignature() with pub key
	blkHash := endorse.blkHash
	endorses := m.ctx.round.commitEndorses[blkHash]
	if endorses == nil {
		endorses = map[string]bool{}
		m.ctx.round.commitEndorses[blkHash] = endorses
	}
	endorses[endorse.endorser] = endorse.decision
	// if either yes or no is true, block must exists and blkHash must be a valid one
	yes, no := m.ctx.calcQuorum(endorses)
	if !yes && !no {
		// Wait for more votes to come
		return sAcceptCommitEndorse, nil
	}

	return m.processEndorseCommit(yes && !no)
}

func (m *cFSM) handleEndorseCommitTimeout(evt fsm.Event) (fsm.State, error) {
	if evt.Type() != eEndorseCommitTimeout {
		return sInvalid, errors.Errorf("invalid event type %s", evt.Type())
	}
	logger.Warn().
		Uint64("height", m.ctx.round.height).
		Int("numOfCommitEndorses", len(m.ctx.round.commitEndorses)).
		Msg("didn't collect enough commit endorse before timeout")

	return m.processEndorseCommit(false)
}

func (m *cFSM) processEndorseCommit(consensus bool) (fsm.State, error) {
	var pendingBlock *blockchain.Block
	height := m.ctx.round.height
	if consensus {
		pendingBlock = m.ctx.round.block
		logger.Info().
			Uint64("block", height).
			Msg("consensus reached")
		consensusMtc.WithLabelValues("true").Inc()
	} else {
		logger.Warn().
			Uint64("block", height).
			Bool("consensus", consensus).
			Msg("consensus did not reach")
		consensusMtc.WithLabelValues("false").Inc()
		if m.ctx.cfg.EnableDummyBlock {
			pendingBlock = m.ctx.chain.MintNewDummyBlock()
			logger.Warn().
				Uint64("block", pendingBlock.Height()).
				Msg("dummy block is generated")
		}
	}
	if pendingBlock != nil {
		// Commit and broadcast the pending block
		if err := m.ctx.chain.CommitBlock(pendingBlock); err != nil {
			logger.Error().
				Err(err).
				Uint64("block", pendingBlock.Height()).
				Bool("dummy", pendingBlock.IsDummyBlock()).
				Msg("error when committing a block")
		}
		// Remove transfers in this block from ActPool and reset ActPool state
		m.ctx.actPool.Reset()
		// Broadcast the committed block to the network
		if blkProto := pendingBlock.ConvertToBlockPb(); blkProto != nil {
			if err := m.ctx.p2p.Broadcast(m.ctx.chain.ChainID(), blkProto); err != nil {
				logger.Error().
					Err(err).
					Uint64("block", pendingBlock.Height()).
					Bool("dummy", pendingBlock.IsDummyBlock()).
					Msg("error when broadcasting blkProto")
			}
		} else {
			logger.Error().
				Uint64("block", pendingBlock.Height()).
				Bool("dummy", pendingBlock.IsDummyBlock()).
				Msg("error when converting a block into a proto msg")
		}
	}
	m.produce(m.newCEvt(eFinishEpoch), 0)
	return sRoundStart, nil
}

func (m *cFSM) handleFinishEpochEvt(evt fsm.Event) (fsm.State, error) {
	finished, err := m.ctx.isEpochFinished()
	if err != nil {
		return sInvalid, errors.Wrap(err, "error when checking if the epoch is finished")
	}
	if finished {
		m.produce(m.newCEvt(eRollDelegates), 0)
		return sEpochStart, nil
	}
	if err := m.produceStartRoundEvt(); err != nil {
		return sInvalid, errors.Wrapf(err, "error when producing %s", eStartRound)
	}
	return sRoundStart, nil

}

func (m *cFSM) isDelegate(delegates []string) bool {
	for _, d := range delegates {
		if m.ctx.addr.RawAddress == d {
			return true
		}
	}
	return false
}

func (m *cFSM) produceStartRoundEvt() error {
	var (
		duration time.Duration
		err      error
	)
	// If we have the cached last block, we get the timestamp from it
	if m.ctx.round.block != nil {
		duration = m.ctx.clock.Now().Sub(m.ctx.round.block.Header.Timestamp())
	} else if duration, err = m.ctx.calcDurationSinceLastBlock(); err != nil {
		// Otherwise, we read it from blockchain
		return errors.Wrap(err, "error when computing the duration since last block time")

	}
	// If the proposal interval is not set (not zero), the next round will only be started after the configured duration
	// after last block's creation time, so that we could keep the constant
	if duration >= m.ctx.cfg.ProposerInterval {
		m.produce(m.newCEvt(eStartRound), 0)
	} else {
		m.produce(m.newCEvt(eStartRound), m.ctx.cfg.ProposerInterval-duration)
	}
	return nil
}

// handleBackdoorEvt takes the dst state from the event and move the FSM into it
func (m *cFSM) handleBackdoorEvt(evt fsm.Event) (fsm.State, error) {
	bEvt, ok := evt.(*backdoorEvt)
	if !ok {
		return sInvalid, errors.Wrap(ErrEvtCast, "the event is not a backdoorEvt")
	}
	return bEvt.dst, nil
}

func (m *cFSM) newCEvt(t fsm.EventType) *consensusEvt {
	return newCEvt(t, m.ctx.clock)
}

func (m *cFSM) newProposeBlkEvt(blk *blockchain.Block) *proposeBlkEvt {
	return newProposeBlkEvt(blk, m.ctx.clock)
}

func (m *cFSM) newProposeBlkEvtFromProposePb(pb *iproto.ProposePb) (*proposeBlkEvt, error) {
	pbEvt := m.newProposeBlkEvt(nil)
	if err := pbEvt.fromProtoMsg(pb); err != nil {
		return nil, errors.Wrap(err, "error when casting a proto msg to proposeBlkEvt")
	}
	return pbEvt, nil
}

func (m *cFSM) newEndorseEvtWithEndorsePb(ePb *iproto.EndorsePb) (*endorseEvt, error) {
	var en endorse
	if err := en.fromProtoMsg(ePb); err != nil {
		return nil, errors.Wrap(err, "error when casting a proto msg to endorse")
	}
	return newEndorseEvtWithEndorse(&en, m.ctx.clock), nil
}

func (m *cFSM) newEndorseProposalEvt(blkHash hash.Hash32B, decision bool) (*endorseEvt, error) {
	return newEndorseEvt(endorseProposal, blkHash, decision, m.ctx.round.height, m.ctx.addr, m.ctx.clock)
}

func (m *cFSM) newEndorseCommitEvt(blkHash hash.Hash32B, decision bool) (*endorseEvt, error) {
	return newEndorseEvt(endorseCommit, blkHash, decision, m.ctx.round.height, m.ctx.addr, m.ctx.clock)
}

func (m *cFSM) newTimeoutEvt(t fsm.EventType, height uint64) *timeoutEvt {
	return newTimeoutEvt(t, m.ctx.clock)
}

func (m *cFSM) newBackdoorEvt(dst fsm.State) *backdoorEvt {
	return newBackdoorEvt(dst, m.ctx.clock)
}

func (m *cFSM) updateSeed() ([]byte, error) {
	numDlgs := m.ctx.cfg.NumDelegates
	epochNum, epochHeight, err := m.ctx.calcEpochNumAndHeight()
	if err != nil {
		return []byte{}, errors.Wrap(err, "Failed to do decode seed")
	}
	if epochNum <= 1 {
		return []byte{}, nil
	}
	selectedID := make([][]uint8, 0)
	selectedSig := make([][]byte, 0)
	selectedPK := make([][]byte, 0)
	endHeight := epochHeight - 1
	startHeight := uint64(numDlgs)*uint64(m.ctx.cfg.NumSubEpochs)*(epochNum-2) + 1
	for i := startHeight; i <= endHeight && len(selectedID) < crypto.Degree+1; i++ {
		blk, err := m.ctx.chain.GetBlockByHeight(i)
		if err != nil {
			continue
		}
		if len(blk.Header.DKGID) > 0 && len(blk.Header.DKGPubkey) > 0 && len(blk.Header.DKGBlockSig) > 0 {
			selectedID = append(selectedID, blk.Header.DKGID)
			selectedSig = append(selectedSig, blk.Header.DKGBlockSig)
			selectedPK = append(selectedPK, blk.Header.DKGPubkey)
		}
	}

	if len(selectedID) < crypto.Degree+1 {
		return []byte{}, errors.New("DKG signature/pubic key is not enough to aggregate")
	}

	aggregateSig, err := crypto.BLS.SignAggregate(selectedID, selectedSig)
	if err != nil {
		return []byte{}, errors.Wrap(err, "Failed to generate aggregate signature to update Seed")
	}
	if err = crypto.BLS.VerifyAggregate(selectedID, selectedPK, m.ctx.epoch.seed, aggregateSig); err != nil {
		return []byte{}, errors.Wrap(err, "Failed to verify aggregate signature to update Seed")
	}
	return aggregateSig, nil
}

func verifyDKGSignature(blk *blockchain.Block, seedByte []byte) error {
	return crypto.BLS.Verify(blk.Header.DKGPubkey, seedByte, blk.Header.DKGBlockSig)
}
