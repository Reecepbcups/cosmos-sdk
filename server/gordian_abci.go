package server

import (
	"context"

	abci "github.com/cometbft/cometbft/abci/types"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

type gordianABCIWrapper struct {
	app servertypes.ABCI
}

func NewGordianABCIWrapper(app servertypes.ABCI) abci.Application {
	return gordianABCIWrapper{app: app}
}

func (w gordianABCIWrapper) Info(_ context.Context, req *abci.RequestInfo) (*abci.ResponseInfo, error) {
	return w.app.Info(req)
}

func (w gordianABCIWrapper) Query(ctx context.Context, req *abci.RequestQuery) (*abci.ResponseQuery, error) {
	return w.app.Query(ctx, req)
}

func (w gordianABCIWrapper) CheckTx(_ context.Context, req *abci.RequestCheckTx) (*abci.ResponseCheckTx, error) {
	return w.app.CheckTx(req)
}

func (w gordianABCIWrapper) InitChain(_ context.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	// TODO: idea here is that we take in the cometbft type and convert it to gordian tmapp.InitChainRequest stuff?
	return w.app.InitChain(req)
}

func (w gordianABCIWrapper) PrepareProposal(_ context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	return w.app.PrepareProposal(req)
}

func (w gordianABCIWrapper) ProcessProposal(_ context.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	return w.app.ProcessProposal(req)
}

func (w gordianABCIWrapper) FinalizeBlock(_ context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	return w.app.FinalizeBlock(req)
}

func (w gordianABCIWrapper) ExtendVote(ctx context.Context, req *abci.RequestExtendVote) (*abci.ResponseExtendVote, error) {
	return w.app.ExtendVote(ctx, req)
}

func (w gordianABCIWrapper) VerifyVoteExtension(_ context.Context, req *abci.RequestVerifyVoteExtension) (*abci.ResponseVerifyVoteExtension, error) {
	return w.app.VerifyVoteExtension(req)
}

func (w gordianABCIWrapper) Commit(_ context.Context, _ *abci.RequestCommit) (*abci.ResponseCommit, error) {
	return w.app.Commit()
}

func (w gordianABCIWrapper) ListSnapshots(_ context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
	return w.app.ListSnapshots(req)
}

func (w gordianABCIWrapper) OfferSnapshot(_ context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
	return w.app.OfferSnapshot(req)
}

func (w gordianABCIWrapper) LoadSnapshotChunk(_ context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
	return w.app.LoadSnapshotChunk(req)
}

func (w gordianABCIWrapper) ApplySnapshotChunk(_ context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
	return w.app.ApplySnapshotChunk(req)
}
