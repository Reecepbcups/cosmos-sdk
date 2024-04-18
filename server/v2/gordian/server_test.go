package gordian

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/core/transaction"
	"cosmossdk.io/log"
)

func TestServerBase(t *testing.T) {
	ctx := context.Background()
	l := log.NewTestLogger(t)

	gs := GordianServer[transaction.Tx]{
		logger: l,
		config: NewDefaultConfig(),
		cleanupFn: func() {
			fmt.Println("cleanup")
		},
	}

	fmt.Println(gs)

	err := gs.Start(ctx)
	fmt.Println(err)
}
