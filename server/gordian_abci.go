package server

import (
	"context"
	"encoding/json"

	gordianabci "github.com/rollchains/gordian/abci"
	tmengine "github.com/rollchains/gordian/tm/tmengine"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

var _ gordianabci.Application = (*gordianABCIWrapper)(nil)

type gordianABCIWrapper struct {
	app servertypes.ABCI

	e tmengine.Engine
}

func NewGordianABCIWrapper(app servertypes.ABCI) gordianabci.Application {
	return gordianABCIWrapper{app: app}
}

// InitChain implements abci.Application.
func (g gordianABCIWrapper) InitChain(context.Context, json.RawMessage) (json.RawMessage, error) {
	// TODO convert between the rawMsg -> the engine for init chain resp channel
	panic("unimplemented")
}
